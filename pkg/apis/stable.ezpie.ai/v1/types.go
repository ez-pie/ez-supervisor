package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DevWorkspace struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	metav1.TypeMeta `json:",inline"`
	// ObjectMeta contains the metadata for the particular object, including
	// things like...
	//  - name
	//  - namespace
	//  - self link
	//  - labels
	//  - ... etc ...
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevWorkspaceSpec   `json:"spec"`
	Status DevWorkspaceStatus `json:"status"`
}

type DevWorkspaceSpec struct {
	Task      EzTask      `json:"task"`
	Data      EzData      `json:"data"`
	Workspace EzWorkspace `json:"workspace"`
}

type EzTask struct {
	Tid  string `json:"tid"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

type EzData struct {
	Entries []EzDataEntry `json:"entries"`
}

type EzDataEntry struct {
	FileSecurityLevel string `json:"fileSecurityLevel"`
	RealName          string `json:"realName"`
	OssBucket         string `json:"ossBucket"`
	OssPath           string `json:"ossPath"`
	OssName           string `json:"ossName"`
}

type EzWorkspace struct {
	CpuRequest     string           `json:"cpuRequest"`
	CpuLimit       string           `json:"cpuLimit"`
	MemRequest     string           `json:"memRequest"`
	MemLimit       string           `json:"memLimit"`
	DiskSize       int32            `json:"diskSize"`
	Image          string           `json:"image"`
	NamespaceName  string           `json:"namespaceName"`
	DeploymentName string           `json:"deploymentName"`
	ServiceName    string           `json:"serviceName"`
	IngressName    string           `json:"ingressName"`
	Env            []EzWorkspaceEnv `json:"env"`
}

type EzWorkspaceEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DevWorkspaceStatus struct {
	DevWorkspaceStatus string `json:"devWorkspaceStatus"`
	AvailableReplicas  int32  `json:"availableReplicas"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DevWorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DevWorkspace `json:"items"`
}
