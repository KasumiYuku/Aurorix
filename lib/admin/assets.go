package admin

import (
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"net/http"
)

// assetsViewProvider 图床 provider 视图。
type assetsViewProvider struct {
	Name       string               `json:"name"`
	Enabled    bool                 `json:"enabled"`
	Priority   int                  `json:"priority"`
	Configured bool                 `json:"configured"`
	HasSecrets bool                 `json:"has_secrets"`
	Schema     []assets.ConfigField `json:"schema"`
	Config     map[string]any       `json:"config"`
}

// assetsConfigInput PUT /api/assets 请求体。
type assetsConfigInput struct {
	Whitelist []string              `json:"whitelist"`
	Providers []assetsInputProvider `json:"providers"`
}

type assetsInputProvider struct {
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Priority int            `json:"priority"`
	Config   map[string]any `json:"config"`
}

func registerAssetsRoutes(api *http.ServeMux, mgr *assets.Manager) {
	api.HandleFunc("GET /admin/api/assets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, buildAssetsView(mgr))
	})
	api.HandleFunc("PUT /admin/api/assets", func(w http.ResponseWriter, r *http.Request) {
		var input assetsConfigInput
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		cfg, err := resolveAssetsConfig(mgr, input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		if err := mgr.Save(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "保存配置失败"})
			return
		}
		writeJSON(w, http.StatusOK, H{"ok": true})
	})
}

// buildAssetsView 合并注册表与磁盘配置生成管理视图, 密码字段掩码为空。
func buildAssetsView(mgr *assets.Manager) H {
	cfg := mgr.Config()
	stored := make(map[string]assets.ProviderItem, len(cfg.Providers))
	for _, item := range cfg.Providers {
		stored[item.Name] = item
	}

	providers := make([]assetsViewProvider, 0, len(assets.Names()))
	for _, name := range assets.Names() {
		schema := assets.ProviderSchema(name)
		item, ok := stored[name]
		enabled := ok && (item.Enabled == nil || *item.Enabled)
		priority := 0
		var viewConfig map[string]any
		hasSecrets := false
		if ok {
			priority = item.Priority
			viewConfig = maskPasswords(schema, item.Config)
			for _, f := range schema {
				if f.Type == "password" {
					if v, exists := item.Config[f.Key]; exists && v != "" && v != nil {
						hasSecrets = true
						break
					}
				}
			}
		} else {
			viewConfig = make(map[string]any)
		}
		providers = append(providers, assetsViewProvider{
			Name:       name,
			Enabled:    enabled,
			Priority:   priority,
			Configured: ok,
			HasSecrets: hasSecrets,
			Schema:     schema,
			Config:     viewConfig,
		})
	}
	whitelist := cfg.Whitelist
	if whitelist == nil {
		whitelist = []string{}
	}
	return H{"whitelist": whitelist, "providers": providers}
}

// resolveAssetsConfig 请求载荷落成 HostConfig: 密码留空时保留旧值。
func resolveAssetsConfig(mgr *assets.Manager, input assetsConfigInput) (assets.HostConfig, error) {
	oldCfg := mgr.Config()
	old := make(map[string]assets.ProviderItem, len(oldCfg.Providers))
	for _, item := range oldCfg.Providers {
		old[item.Name] = item
	}

	providers := make([]assets.ProviderItem, 0, len(input.Providers))
	for _, item := range input.Providers {
		var oldConfig map[string]any
		if prev, ok := old[item.Name]; ok {
			oldConfig = prev.Config
		}
		enabled := item.Enabled
		providers = append(providers, assets.ProviderItem{
			Name:     item.Name,
			Enabled:  &enabled,
			Priority: item.Priority,
			Config:   mergePasswordFields(assets.ProviderSchema(item.Name), oldConfig, item.Config),
		})
	}
	return assets.HostConfig{
		Whitelist: input.Whitelist,
		Providers: providers,
	}, nil
}

// maskPasswords schema 中 type=password 的字段值置空, 避免密钥回显。
func maskPasswords(schema []assets.ConfigField, cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	result := make(map[string]any, len(cfg))
	for k, v := range cfg {
		result[k] = v
	}
	for _, f := range schema {
		if f.Type == "password" {
			result[f.Key] = ""
		}
	}
	return result
}

// mergePasswordFields 对 type=password 字段: 新值为空时沿用旧值。
func mergePasswordFields(schema []assets.ConfigField, old, next map[string]any) map[string]any {
	if next == nil {
		next = make(map[string]any)
	}
	for _, f := range schema {
		if f.Type != "password" {
			continue
		}
		v, exists := next[f.Key]
		if (exists && (v == nil || v == "")) || !exists {
			if old != nil {
				if oldVal, has := old[f.Key]; has {
					next[f.Key] = oldVal
				}
			}
		}
	}
	return next
}
