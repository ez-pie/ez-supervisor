package manage

import (
	"fmt"

	_ "github.com/ez-pie/ez-supervisor/kubernetes"
	"github.com/ez-pie/ez-supervisor/repo"
)

func Create(tid string) uint {
	ws1 := repo.Workspace{
		TaskId: tid,
	}

	ws := repo.CreateWorkspace(ws1)

	fmt.Println("ws1= ", ws1)
	fmt.Println("ws= ", ws)

	return ws.ID
}

func Get(id uint) {
	ws := repo.GetWorkspace(id)
	fmt.Println("ws= ", ws)
}

func GetByTaskId(tid string) {
	ws := repo.GetWorkspaceByTask(tid)
	fmt.Println("ws= ", ws)
}

func GetList() {
	wss := repo.GetWorkspaceList(0, 0)
	fmt.Println("wss= ", wss)
}

func Update(id uint) {
	ws2 := repo.Workspace{
		TaskId: "test_task_id",
		State:  "ready",
		Url:    "fake_url",
	}
	ws2.ID = id

	ws := repo.UpdateWorkspace(ws2)

	fmt.Println("ws2= ", ws2)
	fmt.Println("ws= ", ws)
}

func Delete(id uint) {
	repo.DeleteWorkspace(id)
}
