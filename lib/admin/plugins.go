package admin

import (
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"net/http"
)

// registerPluginRoutes 插件目录/详情/配置/访问控制, 契约与旧版一致。
func registerPluginRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /admin/api/plugins", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, plugin.ManagedPlugins())
	})
	api.HandleFunc("GET /admin/api/plugins/{id}", func(w http.ResponseWriter, r *http.Request) {
		managed, ok := plugin.ManagedPluginByID(r.PathValue("id"))
		if !ok {
			writeJSON(w, http.StatusNotFound, H{"error": "插件不存在"})
			return
		}
		writeJSON(w, http.StatusOK, managed)
	})
	api.HandleFunc("PUT /admin/api/plugins/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		prepared, err := plugin.PrepareConfiguration(r.PathValue("id"), input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		if err := config.SavePluginSettings(r.PathValue("id"), prepared); err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "保存配置失败"})
			return
		}
		if err := plugin.ApplyConfiguration(r.PathValue("id"), prepared); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, H{"ok": true})
	})
	api.HandleFunc("PUT /admin/api/plugins/{id}/access", func(w http.ResponseWriter, r *http.Request) {
		var input plugin.AccessConfig
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		prepared, err := plugin.PrepareAccessConfiguration(r.PathValue("id"), input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		persisted := config.AccessConfig{
			Default:  toConfigAccessRule(prepared.Default),
			Commands: make(map[string]config.AccessRule, len(prepared.Commands)),
			Disabled: prepared.Disabled,
		}
		for path, rule := range prepared.Commands {
			persisted.Commands[path] = toConfigAccessRule(rule)
		}
		if err := config.SavePluginAccess(r.PathValue("id"), persisted); err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "保存访问控制失败"})
			return
		}
		plugin.ApplyAccessConfiguration(r.PathValue("id"), prepared)
		writeJSON(w, http.StatusOK, H{"ok": true})
	})
}

func toConfigAccessRule(rule plugin.AccessRule) config.AccessRule {
	return config.AccessRule{Mode: rule.Mode, Users: rule.Users, Groups: rule.Groups}
}
