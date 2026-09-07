package admin

import (
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/schedule"
	"github.com/KasumiYuku/Aurorix/lib/state"
	"github.com/KasumiYuku/Aurorix/lib/stats"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"net/http"
	"strconv"
)

// handleOverview 概览聚合: 运行态 + 计数 + 网关现状。
func handleOverview(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		view := H{
			"runtime": state.Snapshot(),
			"counts": H{
				"plugins":   plugin.RegisteredCount(),
				"commands":  plugin.GetCommandCount(),
				"jobs":      schedule.GetJobCount(),
				"templates": templates.GetMarkdownTemplateCount(),
			},
			"logs": H{
				"total":  logx.Total(),
				"errors": logx.Errors(),
			},
			"stats": stats.Snapshot(),
		}
		if deps.Profile != nil {
			view["profile"] = deps.Profile.Get()
		}
		if deps.Gateway != nil {
			view["gateway"] = deps.Gateway()
		}
		if deps.Assets != nil {
			view["assets"] = assetsSummary(deps.Assets)
		}
		writeJSON(w, http.StatusOK, view)
	}
}

// handleLogs 环形缓冲快照, 支持级别/来源/关键字过滤。
func handleLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 2048 {
		limit = 500
	}
	entries := logx.Snapshot(limit, logx.Filter{
		MinLevel: q.Get("min_level"),
		Scope:    q.Get("scope"),
		Text:     q.Get("q"),
	})
	writeJSON(w, http.StatusOK, H{
		"entries": entries,
		"scopes":  logx.Scopes(),
		"total":   logx.Total(),
		"errors":  logx.Errors(),
	})
}

// handleJobs 定时任务列表。
func handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, schedule.Jobs())
}

// handleJobPause 暂停/恢复任务。
func handleJobPause(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Paused bool `json:"paused"`
	}
	if err := readJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
		return
	}
	id := r.PathValue("id")
	if !schedule.Exists(id) {
		writeJSON(w, http.StatusNotFound, H{"error": "任务不存在"})
		return
	}
	if input.Paused {
		schedule.Pause(id)
	} else {
		schedule.Resume(id)
	}
	writeJSON(w, http.StatusOK, H{"ok": true})
}

func assetsSummary(mgr *assets.Manager) H {
	cfg := mgr.Config()
	enabled := 0
	for _, item := range cfg.Providers {
		if item.Enabled == nil || *item.Enabled {
			enabled++
		}
	}
	return H{"providers": len(cfg.Providers), "enabled": enabled, "whitelist": len(cfg.Whitelist)}
}
