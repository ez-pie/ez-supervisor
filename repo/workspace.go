package repo

import (
	"gorm.io/gorm"
	"log"
)

type Workspace struct {
	gorm.Model
	TaskId string `gorm:"index"`
	State  string
	Url    string
	Token  string
}

func GetWorkspace(id uint) Workspace {
	var ws Workspace

	Db.First(&ws, id)
	log.Println(ws)

	return ws
}

func GetWorkspaceByTask(taskId string) Workspace {
	var ws Workspace

	Db.First(&ws, "task_id = ?", taskId)
	log.Println(ws)

	return ws
}

func GetWorkspaceList(offset, limit int) []Workspace {
	var workspaces []Workspace

	//Db.Limit(limit).Offset(offset).Find(&workspaces)
	Db.Find(&workspaces)
	log.Println(workspaces)

	return workspaces
}

func CreateWorkspace(workspace Workspace) Workspace {
	Db.Create(&workspace)
	log.Println(workspace)

	return workspace
}

func UpdateWorkspace(workspace Workspace) Workspace {
	var ws Workspace
	ws.ID = workspace.ID

	Db.Model(&ws).Updates(workspace)
	log.Println("ws=", ws)
	log.Println("workspace=", workspace)

	return workspace
}

func DeleteWorkspace(id uint) {
	var ws Workspace

	Db.Delete(&ws, id)
}
