package repo

import (
	"log"
	"time"

	"gorm.io/gorm"
)

type WorkspaceStats struct {
	gorm.Model
	WorkspaceId uint   `gorm:"index"`
	TaskId      string `gorm:"index"`
	MilestoneId string
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
	return workspaceStats
}

func GetOrCreateWorkspaceStatsByWorkspaceId(workspaceStats WorkspaceStats) WorkspaceStats {
	var wss WorkspaceStats

	result := Db.First(&wss,
		"workspace_id = ? and milestone_id = ?",
		workspaceStats.WorkspaceId, workspaceStats.MilestoneId)

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

func GetWorkspaceStatsByWorkspaceAndMileId(workspaceId uint, mileId string) WorkspaceStats {
	var wss WorkspaceStats
	Db.First(&wss, "workspace_id = ? and milestone_id = ?", workspaceId, mileId)
	return wss
}

func GetWorkspaceStatsByTaskAndMileId(taskId string, mileId string) WorkspaceStats {
	var wss WorkspaceStats
	Db.First(&wss, "task_id = ? and milestone_id = ?", taskId, mileId)
	return wss
}

func GetWorkspaceTotalTimeByWorkspaceId(workspaceId uint) int64 {
	var wssList []WorkspaceStats
	Db.Where("workspace_id = ?", workspaceId).Find(&wssList)

	var totalTime int64 = 0
	for _, wss := range wssList {
		totalTime += wss.TotalTime
	}

	return totalTime
}

func GetWorkspaceTotalTimeByTaskId(taskId string) int64 {
	var wssList []WorkspaceStats
	Db.Where("task_id = ?", taskId).Find(&wssList)

	var totalTime int64 = 0
	for _, wss := range wssList {
		totalTime += wss.TotalTime
	}

	return totalTime
}

func GetWorkspaceMileTotalTimeByTaskId(taskId string, mileId string) int64 {
	var wss WorkspaceStats
	Db.First(&wss, "task_id = ? and milestone_id = ?", taskId, mileId)

	return wss.TotalTime
}

func UpdateWorkspaceStats(workspaceStats WorkspaceStats) WorkspaceStats {
	var wss WorkspaceStats
	wss.ID = workspaceStats.ID

	Db.Model(&wss).Updates(workspaceStats)

	return wss
}
