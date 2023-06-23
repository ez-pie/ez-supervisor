package repo

import (
	"gorm.io/gorm"
	"log"
	"time"
)

type WorkspaceStats struct {
	gorm.Model
	WorkspaceId uint   `gorm:"index"`
	TaskId      string `gorm:"index"`
	TotalTime   int64
	StartTime   int64
	CurrentTime int64
}

func ExistWorkspaceStats(workspaceId uint) bool {
	var wss WorkspaceStats

	result := Db.First(&wss, "workspace_id = ?", workspaceId)
	if result.RowsAffected == 0 {
		return false
	}
	return true
}

func CreateWorkspaceStats(workspaceStats WorkspaceStats) WorkspaceStats {
	Db.Create(&workspaceStats)
	log.Println(workspaceStats)

	return workspaceStats
}

func GetOrCreateWorkspaceStatsByWorkspaceId(workspaceStats WorkspaceStats) WorkspaceStats {
	var wss WorkspaceStats

	result := Db.First(&wss, "workspace_id = ?", workspaceStats.WorkspaceId)
	if result.RowsAffected == 0 {
		log.Println("==>>> RowsAffected == 0")
		Db.Create(&workspaceStats)
		log.Println(workspaceStats)
		return workspaceStats
	} else {
		wss.StartTime = time.Now().Unix()
		return UpdateWorkspaceStats(wss)
	}
}

func GetWorkspaceStatsByWorkspaceId(workspaceId uint) WorkspaceStats {
	var wss WorkspaceStats

	Db.First(&wss, "workspace_id = ?", workspaceId)
	log.Println(wss)
	return wss
}

func GetWorkspaceStatsByTaskId(taskId string) WorkspaceStats {
	var wss WorkspaceStats

	Db.First(&wss, "task_id = ?", taskId)
	log.Println(wss)
	return wss
}

func UpdateWorkspaceStats(workspaceStats WorkspaceStats) WorkspaceStats {
	var wss WorkspaceStats
	wss.ID = workspaceStats.ID

	Db.Model(&wss).Updates(workspaceStats)
	log.Println("wss=", wss)
	log.Println("workspaceStats=", workspaceStats)

	return wss
}
