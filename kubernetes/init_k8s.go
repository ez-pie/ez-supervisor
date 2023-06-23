package kubernetes

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	ezclientset "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned"
	ezinformers "github.com/ez-pie/ez-supervisor/pkg/generated/informers/externalversions"
	"github.com/ez-pie/ez-supervisor/pkg/signals"
)

const (
	devWorkspaceName = "devworkspaces.stable.ezpie.ai"

	KubeModeProd        = "PRODUCTION"
	_                   = "DEVELOPMENT"
	DevLocal            = "LOCAL"
	DevRemote           = "REMOTE"
	devRemoteKubeConfig = "./cls-7e54yp5e-config"
)

var (
	kubeClient         *kubernetes.Clientset
	apiExtensionClient *apiextensionsclientset.Clientset
	ezClient           *ezclientset.Clientset
)

func init() {
	klog.InitFlags(nil)
	log.Println("INIT Kubernetes...")

	var config *rest.Config
	var err error
	// set up signals so we handle the shutdown signal gracefully
	ctx := signals.SetupSignalHandler()
	logger := klog.FromContext(ctx)

	if inK8s() {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			logger.Error(err, "Error building in-cluster config")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
			panic(err.Error())
		}
	} else {
		// create out-cluster config
		var devKubeConfig string

		switch os.Getenv("DMODE") {
		case DevRemote:
			devKubeConfig = devRemoteKubeConfig
		case DevLocal:
			fallthrough
		default:
			devKubeConfig = fmt.Sprintf("%s/.kube/config", os.Getenv("HOME"))
		}

		config, err = clientcmd.BuildConfigFromFlags("", devKubeConfig)
		if err != nil {
			logger.Error(err, "Error building kubeconfig")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
			panic(err.Error())
		}
	}

	kubeClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Error building kubernetes clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		panic(err.Error())
	}

	apiExtensionClient, err = apiextensionsclientset.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Error building apiextensions clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		panic(err.Error())
	}

	testCrd()

	ezClient, err = ezclientset.NewForConfig(config)
	if err != nil {
		logger.Error(err, "Error building ezpie clientset")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		panic(err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	exampleInformerFactory := ezinformers.NewSharedInformerFactory(ezClient, time.Second*30)

	controller := NewController(ctx, kubeClient, ezClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		exampleInformerFactory.Stable().V1().DevWorkspaces())

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(ctx.done())
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	kubeInformerFactory.Start(ctx.Done())
	exampleInformerFactory.Start(ctx.Done())

	fmt.Println("到这了！！🦅🦅🦅")

	// 以协程运行，避免阻塞 web 服务
	go func() {
		err1 := controller.Run(ctx, 2)
		if err1 != nil {
			logger.Error(err, "Error running controller")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}()

	fmt.Println("在这之后!!!!!🐸🐸🐸")
}

func inK8s() bool {
	mode := os.Getenv("DEV_MODE")
	log.Printf("k8s mode = [%v]", mode)

	return mode == KubeModeProd
}

func testCrd() {
	log.Println("Check if ezpie CRD exists in the K8S...")

	devWorkspaceCrd, err := apiExtensionClient.ApiextensionsV1().CustomResourceDefinitions().Get(
		context.TODO(), devWorkspaceName, metav1.GetOptions{})
	if err != nil {
		log.Println(err.Error())
		return
	}
	log.Printf("Got the ezpie CRD: %v", devWorkspaceCrd.Name)
}
