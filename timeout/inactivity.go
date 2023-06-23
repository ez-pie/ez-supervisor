package timeout

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ez-pie/ez-supervisor/kubernetes"
	"github.com/ez-pie/ez-supervisor/repo"
)

var (
	global_workspaceList []inactivityIdleManagerEntry
)

// InactivityIdleManager manage all workspace
type InactivityIdleManager interface {
	Add(workspaceId uint)
	Tick(workspaceId uint)
	Show() string
}

func NewInactivityIdleManager() (InactivityIdleManager, error) {
	var entryList []inactivityIdleManagerEntry

	return inactivityIdleManagerImpl{
		workspaceList: entryList,
	}, nil
}

type inactivityIdleManagerImpl struct {
	workspaceList []inactivityIdleManagerEntry
}

func (m inactivityIdleManagerImpl) Add(workspaceId uint) {
	w, err := newInactivityIdleManagerEntry(workspaceId, 3*60*time.Second, 10*time.Second)
	if err != nil {
		log.Fatal("Unable to create activity manager. Cause: ", err.Error())
		return
	}

	log.Println("before", m.workspaceList)
	m.workspaceList = append(m.workspaceList, w)
	log.Println("after:", m.workspaceList)

	log.Println("before global", global_workspaceList)
	global_workspaceList = append(global_workspaceList, w)
	log.Println("after global", global_workspaceList)

	//TODO: move the another place
	taskId := kubernetes.TaskIdByWorkspaceId(workspaceId)
	wss := repo.WorkspaceStats{
		WorkspaceId: workspaceId,
		TaskId:      taskId,
		StartTime:   time.Now().Unix(),
	}
	repo.GetOrCreateWorkspaceStatsByWorkspaceId(wss)
	//TODO end

	w.start()
	log.Printf("add activity manager for workspaceId=%d", workspaceId)
}

func (m inactivityIdleManagerImpl) Tick(workspaceId uint) {
	for _, workspace := range global_workspaceList {
		if workspace.id() == workspaceId {
			log.Printf("tick activity manager for workspaceId=%d", workspaceId)
			workspace.tick()
		}
	}
}

func (m inactivityIdleManagerImpl) Show() string {
	log.Println("show:", m.workspaceList)
	log.Println("show global:", global_workspaceList)

	str := ""
	for i, workspace := range global_workspaceList {
		str += fmt.Sprintf("i=%v -> wid=%v", i, workspace.id())
	}
	return str
}

// inactivityIdleManagerEntry is the entry for a single workspace
type inactivityIdleManagerEntry interface {
	id() uint
	start()
	tick()
}

func newInactivityIdleManagerEntry(workspaceId uint, idleTimeout, stopRetryPeriod time.Duration) (inactivityIdleManagerEntry, error) {
	return inactivityIdleManagerEntryImpl{
		workspaceId:     workspaceId,
		idleTimeout:     idleTimeout,
		stopRetryPeriod: stopRetryPeriod,
		activityC:       make(chan bool),
	}, nil
}

type inactivityIdleManagerEntryImpl struct {
	workspaceId uint

	idleTimeout     time.Duration
	stopRetryPeriod time.Duration

	activityC chan bool
}

func (m inactivityIdleManagerEntryImpl) id() uint {
	return m.workspaceId
}

func (m inactivityIdleManagerEntryImpl) tick() {
	select {
	case m.activityC <- true:
	default:
		// activity is already registered, and it will reset timer if workspace won't be stopped
		log.Println("activity manager is temporary busy")
	}
}

func (m inactivityIdleManagerEntryImpl) start() {
	log.Printf("Activity tracker is run and workspace will be stopped in %s if there is no activity\n", m.idleTimeout)
	timer := time.NewTimer(m.idleTimeout)
	var shutdownChan = make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGTERM)

	go func() {
		for {
			select {
			case <-timer.C:
				if err := stopWorkspace(m.workspaceId, stoppedByInactivity); err != nil {
					timer.Reset(m.stopRetryPeriod)
					log.Printf("Failed to stop workspace. Will retry in %s. Cause: %s", m.stopRetryPeriod, err)
				} else {
					log.Println("Workspace is successfully stopped by inactivity. Bye")
					return
				}
			case <-m.activityC:
				log.Println("Activity is reported. Resetting timer")
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(m.idleTimeout)
			case <-shutdownChan:
				log.Println("Received SIGTERM: shutting down activity manager")
				return
			}
		}
	}()
}
