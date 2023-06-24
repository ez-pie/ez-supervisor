package timeout

import (
	"time"

	"github.com/ez-pie/ez-supervisor/kubernetes"
	"github.com/ez-pie/ez-supervisor/repo"
)

const (
	stoppedByInactivity = "inactivity"
	stoppedByRunTimeout = "run-timeout"
)

func stopWorkspace(workspaceId uint, reason string) error {
	UpdateWorkspaceTimeByWorkspaceId(workspaceId)

	err := kubernetes.StopWorkspace(workspaceId)
	if err != nil {
		return err
	}
	wsModel := repo.GetWorkspace(workspaceId)
	wsModel.State = "closed"
	repo.UpdateWorkspace(wsModel)

	return nil
}

func ReopenWorkspace(workspaceId uint) error {
	err := kubernetes.ReopenWorkspace(workspaceId)
	if err != nil {
		return err
	}
	return nil
}

func UpdateWorkspaceTimeByWorkspaceId(workspaceId uint) {
	ws := repo.GetWorkspace(workspaceId)
	currentMileId := ws.CurrentMilestoneId

	wss := repo.GetWorkspaceStatsByWorkspaceAndMileId(workspaceId, currentMileId)
	wss.CurrentTime = time.Now().Unix()
	wss.TotalTime = wss.CurrentTime - wss.StartTime + wss.TotalTime
	wss.StartTime = wss.CurrentTime

	repo.UpdateWorkspaceStats(wss)
}

func UpdateWorkspaceTimeByTaskId(taskId string) {
	ws := repo.GetWorkspaceByTask(taskId)
	currentMileId := ws.CurrentMilestoneId

	wss := repo.GetWorkspaceStatsByTaskAndMileId(taskId, currentMileId)
	wss.CurrentTime = time.Now().Unix()
	wss.TotalTime = wss.CurrentTime - wss.StartTime + wss.TotalTime
	wss.StartTime = wss.CurrentTime

	repo.UpdateWorkspaceStats(wss)
}
