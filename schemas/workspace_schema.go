package schemas

type Workspace struct {
	Task Task          `json:"task"`
	Spec WorkspaceSpec `json:"spec"`
	Data WorkspaceData `json:"data"`
}

type Task struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type WorkspaceSpec struct {
	CpuLimit   string   `json:"cpu_limit"`
	MemLimit   string   `json:"mem_limit"`
	CpuRequest string   `json:"cpu_request"`
	MemRequest string   `json:"mem_request"`
	Language   []string `json:"language"`
}

type WorkspaceData struct {
	Public    []DataDetail `json:"public"`
	Exclusive []DataDetail `json:"exclusive"`
	Private   []DataDetail `json:"private"`
}

type DataDetail struct {
	Bucket   string `json:"bucket"`
	Path     string `json:"path"`
	Name     string `json:"name"`
	RealName string `json:"real_name"`
}

type WorkspaceInfo struct {
	Id     uint   `json:"id"`
	State  string `json:"state"`
	Url    string `json:"url"`
	Token  string `json:"token"`
	TaskId string `json:"task_id"`
}
