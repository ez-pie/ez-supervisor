package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ez-pie/ez-supervisor/kubernetes"
	"github.com/ez-pie/ez-supervisor/manage"
	"github.com/ez-pie/ez-supervisor/repo"
	"github.com/ez-pie/ez-supervisor/schemas"
	"github.com/ez-pie/ez-supervisor/timeout"
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
			var workspaceCreate schemas.Workspace

			if err := c.ShouldBindJSON(&workspaceCreate); err != nil {
				log.Println(err.Error())
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			//检查是否已创建
			wsModel1 := repo.GetWorkspaceByTask(workspaceCreate.Task.Id)
			if wsModel1.TaskId != "" {
				//已有则检查状态，非 closed 则直接返回，如果是 closed 状态则需要重启
				if wsModel1.State == "closed" {
					err := timeout.ReopenWorkspace(wsModel1.ID)
					if err != nil {
						return
					}
					wsModel1.State = "creating"
				}
				//更新 workspace 状态
				wsModel1 = *kubernetes.QueryWorkspaceStatus(&wsModel1)
				ret := repo.UpdateWorkspace(wsModel1)

				wsInfo1 := schemas.WorkspaceInfo{
					Id:                 ret.ID,
					State:              ret.State,
					Url:                ret.Url,
					Token:              ret.Token,
					TaskId:             ret.TaskId,
					CurrentMilestoneId: ret.CurrentMilestoneId,
				}
				c.JSON(http.StatusOK, wsInfo1)
				return
			}

			//数据库里新建表项
			wsModel := repo.Workspace{
				TaskId:             workspaceCreate.Task.Id,
				CurrentMilestoneId: workspaceCreate.Task.InitMilestoneId,
			}
			ret := repo.CreateWorkspace(wsModel)
			wsInfo := schemas.WorkspaceInfo{
				Id:                 ret.ID,
				State:              ret.State,
				Url:                ret.Url,
				Token:              ret.Token,
				TaskId:             ret.TaskId,
				CurrentMilestoneId: ret.CurrentMilestoneId,
			}
			//创建k8s资源
			err2 := kubernetes.CreateDevWorkspace(strconv.Itoa(int(ret.ID)), workspaceCreate)
			if err2 != nil {
				log.Println(err2.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": err2.Error()})
				return
			}

			c.JSON(http.StatusCreated, wsInfo)
		})

		w1.GET("/get/:workspace_id", func(c *gin.Context) {
			wid := c.Param("workspace_id")
			u64, _ := strconv.ParseUint(wid, 10, 32)
			wsModel := repo.GetWorkspace(uint(u64))

			var ret repo.Workspace
			if wsModel.State != "closed" {
				wsModel = *kubernetes.QueryWorkspaceStatus(&wsModel)
				ret = repo.UpdateWorkspace(wsModel)
			} else {
				ret = wsModel
			}
			wsInfo := schemas.WorkspaceInfo{
				Id:     ret.ID,
				State:  ret.State,
				Url:    ret.Url,
				Token:  ret.Token,
				TaskId: ret.TaskId,
			}
			c.JSON(http.StatusOK, wsInfo)
		})

		w1.GET("/getbytask/:task_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			wsModel := repo.GetWorkspaceByTask(tid)

			var ret repo.Workspace
			if wsModel.State != "closed" {
				wsModel = *kubernetes.QueryWorkspaceStatus(&wsModel)
				ret = repo.UpdateWorkspace(wsModel)
			} else {
				ret = wsModel
			}
			wsInfo := schemas.WorkspaceInfo{
				Id:     ret.ID,
				State:  ret.State,
				Url:    ret.Url,
				Token:  ret.Token,
				TaskId: ret.TaskId,
			}
			c.JSON(http.StatusOK, wsInfo)
		})

		w1.GET("/list", func(c *gin.Context) {
			wsModels := repo.GetWorkspaceList(0, 100)
			c.JSON(http.StatusOK, wsModels)
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
			totalTime := repo.GetWorkspaceTotalTimeByWorkspaceId(uint(u64))
			c.JSON(http.StatusOK, gin.H{
				"working_time": totalTime,
				"unit":         "second",
			})
		})

		w2.GET("/timebytask/:task_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			totalTime := repo.GetWorkspaceTotalTimeByTaskId(tid)
			c.JSON(http.StatusOK, gin.H{
				"working_time": totalTime,
				"unit":         "second",
			})
		})

		w2.GET("/miletimebytask/:task_id/:milestone_id", func(c *gin.Context) {
			tid := c.Param("task_id")
			mid := c.Param("milestone_id")

			mileTime := repo.GetWorkspaceMileTotalTimeByTaskId(tid, mid)
			c.JSON(http.StatusOK, gin.H{
				"working_time": mileTime,
				"unit":         "second",
			})
		})

		w2.POST("/syncmiletime", func(c *gin.Context) {
			var workMileUpdate schemas.WorkspaceMilestoneUpdate

			if err := c.ShouldBindJSON(&workMileUpdate); err != nil {
				log.Println(err.Error())
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			if workMileUpdate.MilestoneId == "0" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "current milestone id should not be 0"})
				return
			}

			wsModel := repo.GetWorkspaceByTask(workMileUpdate.TaskId)
			wsModel.LastMilestoneId = wsModel.CurrentMilestoneId
			if wsModel.LastMilestoneId != workMileUpdate.MilestoneId {
				c.JSON(http.StatusBadRequest, gin.H{"error": "sync error. last milestone in workstation and param is different"})
				return
			}

			// 先更新当前里程碑的计时，如果要切换到下一个里程碑，本次即当前里程碑的最后一段时间
			timeout.UpdateWorkspaceTimeByTaskId(workMileUpdate.TaskId)

			// 同步里程碑 id（切换到下一个里程碑 id），如果 NextMileId 为0，则表示没有下一个里程碑了，此时不再切换
			// TODO: 如果 ezpie 以本次返回的时间为最后一次里程碑的时间，则会和 workstation 计时不一致，workstation 后续还会继续计时
			wsModel.CurrentMilestoneId = workMileUpdate.NextMilestoneId
			repo.UpdateWorkspace(wsModel)

			// 新建下一个里程碑的初始记录
			wssNew := repo.WorkspaceStats{
				WorkspaceId: wsModel.ID,
				TaskId:      workMileUpdate.TaskId,
				MilestoneId: workMileUpdate.NextMilestoneId,
				StartTime:   time.Now().Unix(),
			}
			repo.GetOrCreateWorkspaceStatsByWorkspaceId(wssNew)

			// 返回上一个里程碑的时间
			lastMileTime := repo.GetWorkspaceMileTotalTimeByTaskId(workMileUpdate.TaskId, workMileUpdate.MilestoneId)
			c.JSON(http.StatusOK, gin.H{
				"working_time": lastMileTime,
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
		c.JSON(http.StatusOK, gin.H{"test_message": "Ezpie Workstation supervisor(2)!"})
	})

	// 监听并在 0.0.0.0:8080 上启动服务
	err1 := r.Run(":8080")
	if err1 != nil {
		panic(err1.Error())
	}
}
