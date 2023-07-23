package kubernetes

import (
	"context"
	"fmt"
	"log"
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
	coreinformers "k8s.io/client-go/informers/core/v1"
	networkinginformers "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	ezv1 "github.com/ez-pie/ez-supervisor/pkg/apis/stable.ezpie.ai/v1"
	ezclientset "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned"
	ezscheme "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned/scheme"
	ezinformers "github.com/ez-pie/ez-supervisor/pkg/generated/informers/externalversions/stable.ezpie.ai/v1"
	ezlisters "github.com/ez-pie/ez-supervisor/pkg/generated/listers/stable.ezpie.ai/v1"
	"github.com/ez-pie/ez-supervisor/repo"
	"github.com/ez-pie/ez-supervisor/schemas"
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

	pvLister corelisters.PersistentVolumeLister
	pvSynced cache.InformerSynced

	pvcLister corelisters.PersistentVolumeClaimLister
	pvcSynced cache.InformerSynced

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced

	serviceLister corelisters.ServiceLister
	serviceSynced cache.InformerSynced

	ingressLister networkinglisters.IngressLister
	ingressSynced cache.InformerSynced

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

	pvInformer coreinformers.PersistentVolumeInformer,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	ingressInformer networkinginformers.IngressInformer,

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

		pvLister: pvInformer.Lister(),
		pvSynced: pvInformer.Informer().HasSynced,

		pvcLister: pvcInformer.Lister(),
		pvcSynced: pvcInformer.Informer().HasSynced,

		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,

		serviceLister: serviceInformer.Lister(),
		serviceSynced: serviceInformer.Informer().HasSynced,

		ingressLister: ingressInformer.Lister(),
		ingressSynced: ingressInformer.Informer().HasSynced,

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

	// --------------------------------------------------------------------------------
	_, _ = pvInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,

		UpdateFunc: func(old, new interface{}) {
			newPv1 := new.(*corev1.PersistentVolume)
			oldPv1 := old.(*corev1.PersistentVolume)
			if newPv1.ResourceVersion == oldPv1.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},

		DeleteFunc: controller.handleObject,
	})

	_, _ = pvcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,

		UpdateFunc: func(old, new interface{}) {
			newPvc1 := new.(*corev1.PersistentVolumeClaim)
			oldPvc1 := old.(*corev1.PersistentVolumeClaim)
			if newPvc1.ResourceVersion == oldPvc1.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},

		DeleteFunc: controller.handleObject,
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
			newDepl1 := new.(*appsv1.Deployment)
			oldDepl1 := old.(*appsv1.Deployment)
			if newDepl1.ResourceVersion == oldDepl1.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},

		DeleteFunc: controller.handleObject,
	})

	_, _ = serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,

		UpdateFunc: func(old, new interface{}) {
			newSvc := new.(*corev1.Service)
			oldSvc := old.(*corev1.Service)
			if newSvc.ResourceVersion == oldSvc.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},

		DeleteFunc: controller.handleObject,
	})

	_, _ = ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,

		UpdateFunc: func(old, new interface{}) {
			newIng := new.(*networkingv1.Ingress)
			oldIng := old.(*networkingv1.Ingress)
			if newIng.ResourceVersion == oldIng.ResourceVersion {
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

	// data PVC，PV由 StorageClass 自动分配
	dpName := formatDataPvcName(devWorkspace)
	if dpName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: data pvc name must be specified", key))
		return nil
	}
	dataPvc, err := c.pvcLister.PersistentVolumeClaims(devWorkspace.Namespace).Get(dpName)
	if errors.IsNotFound(err) {
		dataPvc, err = c.kubeclientset.CoreV1().PersistentVolumeClaims(devWorkspace.Namespace).Create(
			context.TODO(),
			newDataPvc(devWorkspace),
			metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(dataPvc, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, dataPvc.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// --- PV & PVC ---
	var pubList1 []pvcItem
	var priList1 []pvcItem

	// artifacts out 数据
	//pv out
	pvOutName := formatOutPvName(devWorkspace)
	if pvOutName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: pv out name must be specified", key))
		return nil
	}
	pvOut, err := c.pvLister.Get(pvOutName)
	if errors.IsNotFound(err) {
		pvOut, err = c.kubeclientset.CoreV1().PersistentVolumes().Create(
			context.TODO(),
			newOutPv(devWorkspace),
			metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(pvOut, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, pvOut.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}
	// pvc out
	pvcOutName := formatOutPvcName(devWorkspace)
	if pvcOutName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: pvc out name must be specified", key))
		return nil
	}
	pvcOut, err := c.pvcLister.PersistentVolumeClaims(devWorkspace.Namespace).Get(pvcOutName)
	if errors.IsNotFound(err) {
		pvcOut, err = c.kubeclientset.CoreV1().PersistentVolumeClaims(devWorkspace.Namespace).Create(
			context.TODO(),
			newOutPvc(devWorkspace),
			metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(pvcOut, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, pvcOut.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// dataEntry 对应挂载的数据
	for idx, dataEntry := range devWorkspace.Spec.Data.Entries {
		//pv
		pvName := formatPvName(devWorkspace, &dataEntry, idx)
		if pvName == "" {
			utilruntime.HandleError(fmt.Errorf("%s: pv name must be specified", key))
			return nil
		}
		pv, err := c.pvLister.Get(pvName)
		if errors.IsNotFound(err) {
			pv, err = c.kubeclientset.CoreV1().PersistentVolumes().Create(
				context.TODO(),
				newCosPv(devWorkspace, &dataEntry, idx),
				metav1.CreateOptions{})
		}
		if err != nil {
			return err
		}
		if !metav1.IsControlledBy(pv, devWorkspace) {
			msg := fmt.Sprintf(MessageResourceExists, pv.Name)
			c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}
		//pvc
		pvcName := formatPvcName(devWorkspace, &dataEntry, idx)
		if pvcName == "" {
			utilruntime.HandleError(fmt.Errorf("%s: pvc name must be specified", key))
			return nil
		}
		pvc, err := c.pvcLister.PersistentVolumeClaims(devWorkspace.Namespace).Get(pvcName)
		if errors.IsNotFound(err) {
			pvc, err = c.kubeclientset.CoreV1().PersistentVolumeClaims(devWorkspace.Namespace).Create(
				context.TODO(),
				newPvc(devWorkspace, &dataEntry, idx),
				metav1.CreateOptions{})
		}
		if err != nil {
			return err
		}
		if !metav1.IsControlledBy(pvc, devWorkspace) {
			msg := fmt.Sprintf(MessageResourceExists, pvc.Name)
			c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}

		pvcItem1 := pvcItem{
			pvcName:      pvcName,
			fileName:     dataEntry.OssName,
			fileShowName: dataEntry.RealName,
		}
		if dataEntry.FileSecurityLevel == "public" || dataEntry.FileSecurityLevel == "exclusive" {
			pubList1 = append(pubList1, pvcItem1)
		} else if dataEntry.FileSecurityLevel == "private" {
			priList1 = append(priList1, pvcItem1)
		}
	}
	pvcInfo1 := pvcInfo{
		pubList: pubList1,
		priList: priList1,
	}

	// --- Deploy ---
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
			newDeployment(devWorkspace, pvcInfo1),
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

	// --- Service ---
	svcName := devWorkspace.Spec.Workspace.ServiceName
	if svcName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: service name must be specified", key))
		return nil
	}
	service, err := c.serviceLister.Services(devWorkspace.Namespace).Get(svcName)
	if errors.IsNotFound(err) {
		service, err = c.kubeclientset.CoreV1().Services(devWorkspace.Namespace).Create(
			context.TODO(),
			newService(devWorkspace),
			metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(service, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, service.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	// --- Ingress ---
	ingName := devWorkspace.Spec.Workspace.IngressName
	if ingName == "" {
		utilruntime.HandleError(fmt.Errorf("%s: ingress name must be specified", key))
		return nil
	}
	ingress, err := c.ingressLister.Ingresses(devWorkspace.Namespace).Get(ingName)
	if errors.IsNotFound(err) {
		ingress, err = c.kubeclientset.NetworkingV1().Ingresses(devWorkspace.Namespace).Create(
			context.TODO(),
			newIngress(devWorkspace),
			metav1.CreateOptions{})
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(ingress, devWorkspace) {
		msg := fmt.Sprintf(MessageResourceExists, ingress.Name)
		c.recorder.Event(devWorkspace, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf("%s", msg)
	}

	logger.V(4).Info("create devworkspace resource",
		"deploy", deployment.Name,
		"service", service.Name,
		"ingress", ingress.Name)

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

func newNamespace(devWorkspace *ezv1.DevWorkspace) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: devWorkspace.Spec.Workspace.NamespaceName,
		},
	}
}

func formatOutPvName(devWorkspace *ezv1.DevWorkspace) string {
	return fmt.Sprintf("pv-out-%v", devWorkspace.Spec.Task.Tid)
}

func formatOutPvcName(devWorkspace *ezv1.DevWorkspace) string {
	return fmt.Sprintf("pvc-out-%v", devWorkspace.Spec.Task.Tid)
}

func formatPvName(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) string {
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

func formatPvcName(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) string {
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

func formatDataPvcName(devWorkspace *ezv1.DevWorkspace) string {
	return fmt.Sprintf("pvc-data-%v", devWorkspace.Spec.Task.Tid)
}

func newOutPv(devWorkspace *ezv1.DevWorkspace) *corev1.PersistentVolume {
	volumeMode := corev1.PersistentVolumeFilesystem
	pvName1 := formatOutPvName(devWorkspace)

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
						"bucket":          "workstation-test-1313546141", //TODO: 暂时写死
						"path":            fmt.Sprintf("/artifacts/task-%v", devWorkspace.Spec.Task.Tid),
						"additional_args": "-oensure_diskfree=20480 -oallow_other",
					},
				},
			},
		},
	}
}

func newCosPv(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) *corev1.PersistentVolume {
	volumeMode := corev1.PersistentVolumeFilesystem
	pvName1 := formatPvName(devWorkspace, dataEntry, dataSeq)

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

func newOutPvc(devWorkspace *ezv1.DevWorkspace) *corev1.PersistentVolumeClaim {
	pvcName1 := formatOutPvcName(devWorkspace)
	pvName1 := formatOutPvName(devWorkspace)
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

func newPvc(devWorkspace *ezv1.DevWorkspace, dataEntry *ezv1.EzDataEntry, dataSeq int) *corev1.PersistentVolumeClaim {
	pvcName1 := formatPvcName(devWorkspace, dataEntry, dataSeq)
	pvName1 := formatPvName(devWorkspace, dataEntry, dataSeq)
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

func newDataPvc(devWorkspace *ezv1.DevWorkspace) *corev1.PersistentVolumeClaim {
	storageClassName := "cbs-test" // 根据腾讯云创建的名字决定
	dataName := formatDataPvcName(devWorkspace)
	volumeMode := corev1.PersistentVolumeFilesystem

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataName,
			Namespace: devWorkspace.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(devWorkspace, ezv1.SchemeGroupVersion.WithKind(devWorkspaceKind)),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
			StorageClassName: &storageClassName,
			VolumeMode:       &volumeMode,
		},
	}
}

func formatVolumeName(pvcName1 string) string {
	return fmt.Sprintf("vol-%v", pvcName1)
}

// newDeployment creates a new Deployment for a DevWorkspace resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the DevWorkspace resource that 'owns' it.
func newDeployment(devWorkspace *ezv1.DevWorkspace, pvcinfo pvcInfo) *appsv1.Deployment {
	// 准备 volumes 和 volumeMounts
	// 由于一定会挂载数据盘和 out，所以初始时直接加入 data pvc 和 out pvc
	dataPvcName := formatDataPvcName(devWorkspace)
	volName := formatVolumeName(dataPvcName)

	outPvcName := formatOutPvcName(devWorkspace)
	outVolName := formatVolumeName(outPvcName)
	vols := []corev1.Volume{
		{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: dataPvcName,
				},
			},
		},
		{
			Name: outVolName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: outPvcName,
				},
			},
		},
	}
	volMnts := []corev1.VolumeMount{
		{
			Name:      volName,
			MountPath: "/home/workspace", // TODO: 定义常量
		},
		{
			Name:      outVolName,
			MountPath: "/home/artifacts", //TODO:定义常量
		},
	}

	var vol corev1.Volume
	var volMnt corev1.VolumeMount

	// public 数据
	for _, item := range pvcinfo.pubList {
		volName = formatVolumeName(item.pvcName)

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
		volName = formatVolumeName(item.pvcName)

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
							Image:           devWorkspace.Spec.Workspace.Image,
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

func CreateDevWorkspace(workspaceId string, wsc schemas.Workspace) error {
	dws := newDevWorkspace(wsc)

	ns, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), newNamespace(dws), metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			log.Println("Create namespace error:")
			log.Println(err.Error())
			return err
		}
	}
	log.Printf("namespace: %v", ns.Name)

	devWorkspace, err := ezClient.StableV1().DevWorkspaces(dws.Spec.Workspace.NamespaceName).Create(
		context.TODO(),
		dws,
		metav1.CreateOptions{},
	)

	if err != nil {
		log.Println("CreateDevWorkspace error:")
		log.Println(err.Error())
		return err
	}

	log.Println("CreateDevWorkspace success: devWorkspace->name")
	log.Println(devWorkspace.Name)
	return nil
}

func newDevWorkspace(workspaceCreate schemas.Workspace) *ezv1.DevWorkspace {
	var dataEntries []ezv1.EzDataEntry

	for _, dataItem := range workspaceCreate.Data.Public {
		entry := ezv1.EzDataEntry{
			FileSecurityLevel: "public",
			RealName:          dataItem.RealName,
			OssBucket:         dataItem.Bucket,
			OssPath:           dataItem.Path,
			OssName:           dataItem.Name,
		}
		dataEntries = append(dataEntries, entry)
	}

	for _, dataItem := range workspaceCreate.Data.Exclusive {
		entry := ezv1.EzDataEntry{
			FileSecurityLevel: "exclusive",
			RealName:          dataItem.RealName,
			OssBucket:         dataItem.Bucket,
			OssPath:           dataItem.Path,
			OssName:           dataItem.Name,
		}
		dataEntries = append(dataEntries, entry)
	}

	for _, dataItem := range workspaceCreate.Data.Private {
		entry := ezv1.EzDataEntry{
			FileSecurityLevel: "private",
			RealName:          dataItem.RealName,
			OssBucket:         dataItem.Bucket,
			OssPath:           dataItem.Path,
			OssName:           dataItem.Name,
		}
		dataEntries = append(dataEntries, entry)
	}

	return &ezv1.DevWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      formatDevWorkspaceName(workspaceCreate.Task.Id),
			Namespace: formatNamespaceName(workspaceCreate.Task.Id),
		},
		Spec: ezv1.DevWorkspaceSpec{
			Task: ezv1.EzTask{
				Tid:  workspaceCreate.Task.Id,
				Name: workspaceCreate.Task.Name,
				Desc: "",
			},
			Data: ezv1.EzData{Entries: dataEntries},
			Workspace: ezv1.EzWorkspace{
				CpuRequest:     workspaceCreate.Spec.CpuRequest,
				CpuLimit:       workspaceCreate.Spec.CpuLimit,
				MemRequest:     workspaceCreate.Spec.MemRequest,
				MemLimit:       workspaceCreate.Spec.MemLimit,
				DiskSize:       0,
				Image:          "mirrordust/code:v0.3.0",
				NamespaceName:  formatNamespaceName(workspaceCreate.Task.Id),
				DeploymentName: formatDeployName(workspaceCreate.Task.Id),
				ServiceName:    formatServiceName(workspaceCreate.Task.Id),
				IngressName:    formatIngressName(workspaceCreate.Task.Id),
				Env: []ezv1.EzWorkspaceEnv{ //TODO:暂时没使用到，在deploy里写死
					{
						Name:  "TASKID",
						Value: workspaceCreate.Task.Id,
					},
				},
			},
		},
	}
}

func QueryWorkspaceStatus(in *repo.Workspace) (out *repo.Workspace) {
	namespaceName := formatNamespaceName(in.TaskId)
	ingressName := formatIngressName(in.TaskId)
	deplName := formatDeployName(in.TaskId)

	out = in

	deploy, err := kubeClient.AppsV1().Deployments(deplName).Get(context.TODO(), deplName, metav1.GetOptions{})
	if err != nil {
		log.Println("checkDeploymentReady error:")
		log.Println(err.Error())
		// 出错直接返回，状态不变
		return out
	}

	if deploy.Status.AvailableReplicas < 1 {
		out.Url = ""
		out.State = "creating"
		log.Printf("deploy AvailableReplicas=%v", deploy.Status.AvailableReplicas)
		return out
	}

	ingress, err := kubeClient.NetworkingV1().Ingresses(namespaceName).Get(
		context.TODO(), ingressName, metav1.GetOptions{})
	if err != nil {
		log.Println("checkIngressReady error:")
		log.Println(err.Error())
		// 出错直接返回，状态不变
		return out
	}

	// 检查 Ingress 的状态条件
	for _, ing := range ingress.Status.LoadBalancer.Ingress {
		// 有任意一个ip则表示ingress就绪了
		if ing.IP != "" || ing.Hostname != "" {
			out.Url = ingress.Spec.Rules[0].Host + "/?folder=/home/workspace"
			out.State = "ready"
			log.Printf("ingress state=%v", out.State)
			return out
		}
	}

	// 否则表示ingress未就绪
	out.Url = "none"
	out.State = "creating"
	log.Printf("ingress state=%v", out.State)
	return out
}
