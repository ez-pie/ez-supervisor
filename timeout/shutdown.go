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

	//TODO: move the another place
	wss := repo.GetWorkspaceStatsByWorkspaceId(workspaceId)
	wss.CurrentTime = time.Now().Unix()
	wss.TotalTime = wss.CurrentTime - wss.StartTime + wss.TotalTime
	wss.StartTime = wss.CurrentTime

	repo.UpdateWorkspaceStats(wss)
	//TODO end

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
