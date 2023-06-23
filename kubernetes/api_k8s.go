package kubernetes

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/ez-pie/ez-supervisor/repo"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

func StopWorkspace(workspaceId uint) error {
	deployName1 := deployName(workspaceId)
	deploymentsClient := kubeClient.AppsV1().Deployments("ezpie")

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := deploymentsClient.Get(context.TODO(), deployName1, metav1.GetOptions{})
		if getErr != nil {
			//panic(fmt.Errorf("Failed to get latest version of Deployment: %v", getErr))
			return getErr
		}

		result.Spec.Replicas = int32Ptr(0) // reduce replica count
		_, updateErr := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return retryErr
	}

	log.Println("Updated deployment...")
	return nil
}

func ReopenWorkspace(workspaceId uint) error {
	deployName1 := deployName(workspaceId)
	deploymentsClient := kubeClient.AppsV1().Deployments("ezpie")

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := deploymentsClient.Get(context.TODO(), deployName1, metav1.GetOptions{})
		if getErr != nil {
			panic(fmt.Errorf("Failed to get latest version of Deployment: %v", getErr))
		}

		result.Spec.Replicas = int32Ptr(1) // reduce replica count
		_, updateErr := deploymentsClient.Update(context.TODO(), result, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		return retryErr
	}

	log.Println("Updated deployment...")
	return nil
}

func deployName(workspaceId uint) string {
	taskId := TaskIdByWorkspaceId(workspaceId)
	name := fmt.Sprintf("deployment-%s", taskId)
	return name
}

func TaskIdByWorkspaceId(workspaceId uint) string {
	taskId := repo.GetWorkspace(workspaceId).TaskId
	return taskId
}

func int32Ptr(i int32) *int32 { return &i }
