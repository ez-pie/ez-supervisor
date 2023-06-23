package kubernetes

import (
	"context"
	"log"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const (
	KubeModeProd    = "PRODUCTION"
	_               = "DEVELOPMENT"
	localKubeConfig = "./cls-7e54yp5e-config"
)

var ClientSet *kubernetes.Clientset

func init() {
	log.Println("INIT Kubernetes...")

	var config *rest.Config
	var err error

	if inK8s() {
		// creates the in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", localKubeConfig)
		if err != nil {
			panic(err.Error())
		}
	}

	// creates the clientSet
	tmpClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	ClientSet = tmpClientSet

	// just for test
	myTestK8s()
}

func inK8s() bool {
	var mode string
	mode = os.Getenv("DEV_MODE")
	log.Printf("k8s mode = [%v]", mode)
	if mode == KubeModeProd {
		return true
	} else {
		return false
	}
}

func myTestK8s() {
	log.Println("TEST Kubernetes...")

	for i := 0; i < 2; i++ {
		pods, err := ClientSet.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		log.Printf("There are %d pods in the cluster\n", len(pods.Items))

		// Examples for error handling:
		// - Use helper functions like e.g. errors.IsNotFound()
		// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
		namespace := "default"
		pod := "ide-746bcbb595-k6pgw"

		_, err = ClientSet.CoreV1().Pods(namespace).Get(context.TODO(), pod, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			log.Printf("Pod %s in namespace %s not found\n", pod, namespace)
		} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
			log.Printf("Error getting pod %s in namespace %s: %v\n",
				pod, namespace, statusError.ErrStatus.Message)
		} else if err != nil {
			panic(err.Error())
		} else {
			log.Printf("Found pod %s in namespace %s\n", pod, namespace)
		}

		time.Sleep(1 * time.Second)
	}
}
