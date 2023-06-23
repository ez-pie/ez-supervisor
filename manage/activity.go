package manage

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ez-pie/ez-supervisor/timeout"
)

func HandleActivityAdd(c *gin.Context, manager timeout.InactivityIdleManager, workspaceId uint) {
	manager.Add(workspaceId)
	c.Writer.WriteHeader(http.StatusCreated)
	return
}

func HandleActivityTick(c *gin.Context, manager timeout.InactivityIdleManager, workspaceId uint) {
	manager.Tick(workspaceId)
	c.Writer.WriteHeader(http.StatusOK)
	return
}

func HandleShow(c *gin.Context, manager timeout.InactivityIdleManager) {
	str := manager.Show()
	c.JSON(http.StatusOK, gin.H{
		"str": str,
	})
	return
}
