package kubernetes

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	ezv1 "github.com/ez-pie/ez-supervisor/pkg/apis/stable.ezpie.ai/v1"
	ezclientset "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned"
	ezscheme "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned/scheme"
	ezinformers "github.com/ez-pie/ez-supervisor/pkg/generated/informers/externalversions/stable.ezpie.ai/v1"
	ezlisters "github.com/ez-pie/ez-supervisor/pkg/generated/listers/stable.ezpie.ai/v1"
)

const (
	controllerAgentName = "ezpie-devworkspace-controller"
	devWorkspaceKind    = "DevWorkspace"
)

const (
	// SuccessSynced is used as part of the Event 'reason' when a DevWorkspace is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a DevWorkspace fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by DevWorkspace"
	// MessageResourceSynced is the message used for an Event fired when a DevWorkspace
	// is synced successfully
	MessageResourceSynced = "DevWorkspace synced successfully"
)

// Controller is the controller implementation for DevWorkspace resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// ezclientset is a clientset for our own API group
	ezclientset ezclientset.Interface

	deploymentsLister   appslisters.DeploymentLister
	deploymentsSynced   cache.InformerSynced
	devworkspacesLister ezlisters.DevWorkspaceLister
	devworkspacesSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	ezclientset ezclientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	devworkspaceInformer ezinformers.DevWorkspaceInformer) *Controller {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add devworkspace-controller types to the default Kubernetes Scheme so Events can be
	// logged for devworkspace-controller types.
	utilruntime.Must(ezscheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset: kubeclientset,
		ezclientset:   ezclientset,

		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,

		devworkspacesLister: devworkspaceInformer.Lister(),
		devworkspacesSynced: devworkspaceInformer.Informer().HasSynced,

		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "DevWorkspaces"),
		recorder:  recorder,
	}

	logger.Info("Setting up event handlers")

	// Set up an event handler for when DevWorkspace resources change
	_, _ = devworkspaceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDevWorkspace,

		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDevWorkspace(new)
		},
	})

	// Set up an event handler for when Deployment resources change. This
	// handler will look up the owner of the given Deployment, and if it is
	// owned by a DevWorkspace resource then the handler will enqueue that DevWorkspace resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	_, _ = deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,

		UpdateFunc: func(old, new interface{}) {
			newDeploy := new.(*appsv1.Deployment)
			oldDeploy := old.(*appsv1.Deployment)
			if newDeploy.ResourceVersion == oldDeploy.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},

		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shut down the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting DevWorkspace controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.devworkspacesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process DevWorkspace resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func, so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool

		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		// Run the syncHandler, passing it the namespace/name string of the
		// DevWorkspace resource to be synced.
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		// Finally, if no error occurs we Forget this item, so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the DevWorkspace resource
// with the current status of the resource.
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the DevWorkspace resource with this namespace/name
	devWorkspace, err := c.devworkspacesLister.DevWorkspaces(namespace).Get(name)
	if err != nil {
		// The DevWorkspace resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("DevWorkspace '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	deplName := devWorkspace.Spec.Workspace.DeploymentName
	if deplName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	// Get the deployment with the name specified in DevWorkspace.spec
	deployment, err := c.deploymentsLister.Deployments(devWorkspace.Namespace).Get(deplName)
	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		deployment, err = c.kubeclientset.AppsV1().Deployments(devWorkspace.Namespace).Create(
			context.TODO(),
			newDeployment(devWorkspace),
			metav1.CreateOptions{})
	}

	// If an error occurs during Get/Create, we'll requeue the item, so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// If the Deployment is not controlled by this DevWorkspace resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(deployment, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	logger.V(4).Info("Get/Create deployment resource", "currentReplicas", "🐼")

	// If this number of the replicas on the Foo resource is specified, and the
	// number does not equal the current desired replicas on the Deployment, we
	// should update the Deployment resource.
	//if foo.Spec.Replicas != nil && *foo.Spec.Replicas != *deployment.Spec.Replicas {
	//	logger.V(4).Info("Update deployment resource", "currentReplicas", *foo.Spec.Replicas, "desiredReplicas", *deployment.Spec.Replicas)
	//	deployment, err = c.kubeclientset.AppsV1().Deployments(foo.Namespace).Update(context.TODO(), newDeployment(foo), metav1.UpdateOptions{})
	//}

	// If an error occurs during Update, we'll requeue the item, so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	//if err != nil {
	//	return err
	//}

	// Finally, we update the status block of the DevWorkspace resource to reflect the
	// current state of the world
	err = c.updateDevWorkspaceStatus(devWorkspace, deployment)
	if err != nil {
		return err
	}

	c.recorder.Event(devWorkspace, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateDevWorkspaceStatus(devWorkspace *ezv1.DevWorkspace, deployment *appsv1.Deployment) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	devWorkspaceCopy := devWorkspace.DeepCopy()
	devWorkspaceCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the DevWorkspace resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.
	_, err := c.ezclientset.StableV1().DevWorkspaces(devWorkspace.Namespace).UpdateStatus(
		context.TODO(),
		devWorkspaceCopy,
		metav1.UpdateOptions{})
	return err
}

// enqueueDevWorkspace takes a DevWorkspace resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than DevWorkspace.
func (c *Controller) enqueueDevWorkspace(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the DevWorkspace resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that DevWorkspace resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *Controller) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())

	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}

	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a DevWorkspace, we should not do anything more
		// with it.
		if ownerRef.Kind != devWorkspaceKind {
			return
		}

		devWorkspace, err := c.devworkspacesLister.DevWorkspaces(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "devWorkspace", ownerRef.Name)
			return
		}

		c.enqueueDevWorkspace(devWorkspace)
		return
	}
}

// newDeployment creates a new Deployment for a DevWorkspace resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DevWorkspace resource that 'owns' it.
func newDeployment(devWorkspace *ezv1.DevWorkspace) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "nginx",
		"controller": devWorkspace.Name,
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devWorkspace.Spec.Workspace.DeploymentName,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
}

func newNamespace(devWorkspace *ezv1.DevWorkspace) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: devWorkspace.Spec.Workspace.NamespaceName,
		},
	}
}

func pvName(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) string {
	var type1 string

	switch dataEntry.FileSecurityLevel {
	case "public":
		type1 = "pub"
	case "exclusive":
		type1 = "exc"
	case "private":
		type1 = "pri"
	default:
		type1 = "ukn"
	}

	return fmt.Sprintf("pv-%v-%v-%v", devWorkspace.Spec.Task.Tid, type1, dataSeq)
}
func pvcName(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) string {
	var type1 string

	switch dataEntry.FileSecurityLevel {
	case "public":
		type1 = "pub"
	case "exclusive":
		type1 = "exc"
	case "private":
		type1 = "pri"
	default:
		type1 = "ukn"
	}

	return fmt.Sprintf("pvc-%v-%v-%v", devWorkspace.Spec.Task.Tid, type1, dataSeq)
}

func newCosPv(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) *corev1.PersistentVolume {
	volumeMode := corev1.PersistentVolumeFilesystem
	pvName1 := pvName(devWorkspace, dataEntry, dataSeq)

	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName1,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			VolumeMode:                    &volumeMode,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "com.tencent.cloud.csi.cosfs",
					VolumeHandle: pvName1,
					NodePublishSecretRef: &corev1.SecretReference{
						Name:      "cos-secret",
						Namespace: "kube-system",
					},
					VolumeAttributes: map[string]string{
						"url":             "http://cos.ap-hongkong.myqcloud.com",
						"bucket":          dataEntry.OssBucket,
						"path":            dataEntry.OssPath,
						"additional_args": "-oensure_diskfree=20480 -oallow_other",
					},
				},
			},
		},
	}
}

func newPvc(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) *corev1.PersistentVolumeClaim {
	pvcName1 := pvcName(devWorkspace, dataEntry, dataSeq)
	pvName1 := pvName(devWorkspace, dataEntry, dataSeq)
	storageClassName := ""

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName1,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
			VolumeName:       pvName1,
			StorageClassName: &storageClassName,
		},
	}
}

func volumeName(pvcName1 string) string {
	return fmt.Sprintf("vol-%v", pvcName1)
}

func newDepl(devWorkspace *ezv1.DevWorkspace, pvcinfo pvcInfo) *appsv1.Deployment {
	// 准备 volumes 和 volumeMounts
	var vols []corev1.Volume
	var volMnts []corev1.VolumeMount
	var volName string
	var vol corev1.Volume
	var volMnt corev1.VolumeMount
	// public 数据
	for _, item := range pvcinfo.pubList {
		volName = volumeName(item.pvcName)

		vol = corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: item.pvcName,
					ReadOnly:  true,
				},
			},
		}
		vols = append(vols, vol)

		volMnt = corev1.VolumeMount{
			Name:      volName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("/home/data/public/%v", item.fileShowName),
			SubPath:   item.fileName,
		}
		volMnts = append(volMnts, volMnt)
	}
	// private 数据
	for _, item := range pvcinfo.priList {
		volName = volumeName(item.pvcName)

		vol = corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: item.pvcName,
					ReadOnly:  true,
				},
			},
		}
		vols = append(vols, vol)

		volMnt = corev1.VolumeMount{
			Name:      volName,
			ReadOnly:  true,
			MountPath: fmt.Sprintf("/home/data/private/%v", item.fileShowName),
			SubPath:   item.fileName,
		}
		volMnts = append(volMnts, volMnt)
	}

	labels := map[string]string{
		"ezpie-app": devWorkspace.Spec.Workspace.DeploymentName,
	}

	// 准备 containers
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devWorkspace.Spec.Workspace.DeploymentName,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Volumes: vols,
					Containers: []corev1.Container{
						{
							Name:            fmt.Sprintf("%v-container-1", devWorkspace.Spec.Workspace.DeploymentName),
							Image:           "mirrordust/code:v0.2.0",
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 3000,
								},
							},
							VolumeMounts: volMnts,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(devWorkspace.Spec.Workspace.CpuLimit),
									corev1.ResourceMemory: resource.MustParse(devWorkspace.Spec.Workspace.MemLimit),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(devWorkspace.Spec.Workspace.CpuRequest),
									corev1.ResourceMemory: resource.MustParse(devWorkspace.Spec.Workspace.MemRequest),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "TASKID",
									Value: devWorkspace.Spec.Task.Tid,
								},
							},
						},
					},
				},
			},
		},
	}
}

func newService(devWorkspace *ezv1.DevWorkspace) *corev1.Service {
	labels := map[string]string{
		"ezpie-app": devWorkspace.Spec.Workspace.DeploymentName,
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devWorkspace.Spec.Workspace.ServiceName,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
			Labels: labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       3000,
					TargetPort: intstr.FromInt32(3000),
				},
			},
		},
	}
}

func newIngress(devWorkspace *ezv1.DevWorkspace) *networkingv1.Ingress {
	pathType1 := networkingv1.PathTypePrefix

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      devWorkspace.Spec.Workspace.IngressName,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
			Annotations: map[string]string{
				"kubernetes.io/ingress.class": "nginx",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: fmt.Sprintf("workspace-%v.124.156.125.152.nip.io", devWorkspace.Spec.Task.Tid),
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType1,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: devWorkspace.Spec.Workspace.ServiceName,
											Port: networkingv1.ServiceBackendPort{
												Number: 3000,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

type pvcInfo struct {
	pubList []pvcItem
	priList []pvcItem
}

type pvcItem struct {
	pvcName      string
	fileName     string
	fileShowName string
}
