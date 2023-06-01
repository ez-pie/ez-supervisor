package main

import (
	"github.com/ez-pie/ez-supervisor/manage"
	"github.com/ez-pie/ez-supervisor/repo"
	"github.com/ez-pie/ez-supervisor/timeout"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"strconv"
)

func main() {
	activityManager, err := timeout.NewInactivityIdleManager()
	if err != nil {
		log.Fatal("Unable to create activity manager. Cause: ", err.Error())
		return
	}

	// web endpoints
	r := gin.Default()

	// router group: workspace
	w1 := r.Group("/workspace")
	{
		w1.POST("/create", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"create": "ok"})
		})

		w1.GET("/get/:workspace_id", func(c *gin.Context) {
			workspaceId := c.Param("workspace_id")
			c.JSON(http.StatusOK, gin.H{"workspace_id": workspaceId})
		})

		w1.GET("/getbytask/:task_id", func(c *gin.Context) {
			taskId := c.Param("task_id")
			c.JSON(http.StatusOK, gin.H{"task_id": taskId})
		})

		w1.GET("/list", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"list": "ok"})
		})

		//w1.POST("/shutdown/:workspace_id", func(c *gin.Context) {
		//	workspaceId := c.Param("workspace_id")
		//	c.JSON(http.StatusFound, gin.H{"workspace_id": workspaceId})
		//})

		//w1.POST("/destroy/:workspace_id", func(c *gin.Context) {
		//	workspaceId := c.Param("workspace_id")
		//	c.JSON(http.StatusFound, gin.H{"workspace_111id": workspaceId})
		//})
	}

	// router group: workspace-stats
	w2 := r.Group("/workspace-stats")
	{
		w2.GET("/time/:workspace_id", func(c *gin.Context) {
			wid := c.Param("workspace_id")
			u64, _ := strconv.ParseUint(wid, 10, 32)
			wss := repo.GetWorkspaceStatsByWorkspaceId(uint(u64))
			c.JSON(http.StatusOK, gin.H{
				"working_time": wss.TotalTime,
				"unit":         "second",
			})
		})

		w2.GET("/timebytask/:task_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			wss := repo.GetWorkspaceStatsByTaskId(tid)
			c.JSON(http.StatusOK, gin.H{
				"working_time": wss.TotalTime,
				"unit":         "second",
			})
		})
	}

	// router group: workspace-activity
	w3 := r.Group("/workspace-activity")
	{
		w3.POST("/create/:workspace_id", func(c *gin.Context) {
			wid := c.Param("workspace_id")
			u64, _ := strconv.ParseUint(wid, 10, 32)
			manage.HandleActivityAdd(c, activityManager, uint(u64))
		})

		w3.POST("/createbytask/:task_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			wid := repo.GetWorkspaceByTask(tid).ID
			manage.HandleActivityAdd(c, activityManager, wid)
		})

		w3.POST("/tick/:workspace_id", func(c *gin.Context) {
			wid := c.Param("workspace_id")
			u64, _ := strconv.ParseUint(wid, 10, 32)
			manage.HandleActivityTick(c, activityManager, uint(u64))
		})

		w3.POST("/tickbytask/:task_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			wid := repo.GetWorkspaceByTask(tid).ID
			manage.HandleActivityTick(c, activityManager, wid)
		})

		w3.GET("/idle-manager", func(c *gin.Context) {
			manage.HandleShow(c, activityManager)
		})
	}

	// test url
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"test_message": "Workstation supervisor!"})
	})

	// 监听并在 0.0.0.0:8080 上启动服务
	err1 := r.Run(":8080")
	if err1 != nil {
		panic(err1.Error())
	}
}
