package admin

import (
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/schedule"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// handleOverview 概览聚合: 运行态 + 计数 + 网关现状。
func handleOverview(deps Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		view := gin.H{
			"runtime": state.Snapshot(),
			"counts": gin.H{
				"plugins":   plugin.RegisteredCount(),
				"commands":  plugin.GetCommandCount(),
				"jobs":      schedule.GetJobCount(),
				"templates": templates.GetMarkdownTemplateCount(),
			},
			"logs": gin.H{
				"total":  logx.Total(),
				"errors": logx.Errors(),
			},
		}
		if deps.Gateway != nil {
			view["gateway"] = deps.Gateway()
		}
		if deps.Assets != nil {
			view["assets"] = assetsSummary(deps.Assets)
		}
		c.JSON(http.StatusOK, view)
	}
}

// handleLogs 环形缓冲快照, 支持级别/来源/关键字过滤。
func handleLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	if limit <= 0 || limit > 2048 {
		limit = 500
	}
	entries := logx.Snapshot(limit, logx.Filter{
		MinLevel: c.Query("min_level"),
		Scope:    c.Query("scope"),
		Text:     c.Query("q"),
	})
	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"scopes":  logx.Scopes(),
		"total":   logx.Total(),
		"errors":  logx.Errors(),
	})
}

// handleJobs 定时任务列表。
func handleJobs(c *gin.Context) {
	c.JSON(http.StatusOK, schedule.Jobs())
}

// handleJobPause 暂停/恢复任务。
func handleJobPause(c *gin.Context) {
	var input struct {
		Paused bool `json:"paused"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
		return
	}
	id := c.Param("id")
	if !schedule.Exists(id) {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if input.Paused {
		schedule.Pause(id)
	} else {
		schedule.Resume(id)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func assetsSummary(mgr *assets.Manager) gin.H {
	cfg := mgr.Config()
	enabled := 0
	for _, item := range cfg.Providers {
		if item.Enabled == nil || *item.Enabled {
			enabled++
		}
	}
	return gin.H{"providers": len(cfg.Providers), "enabled": enabled, "whitelist": len(cfg.Whitelist)}
}
