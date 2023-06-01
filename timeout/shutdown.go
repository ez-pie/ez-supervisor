package timeout

import (
	"github.com/ez-pie/ez-supervisor/kubernetes"
	"github.com/ez-pie/ez-supervisor/repo"
	"time"
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

	return nil
}

func reopenWorkspace(workspaceId uint) error {
	err := kubernetes.ReopenWorkspace(workspaceId)
	if err != nil {
		return err
	}
	return nil
}
