// Package admin 管理台: 鉴权、JSON API 与 SPA 静态托管。
package admin

import (
	"embed"
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"io"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

//go:embed dist
var distFS embed.FS

// H JSON 响应便捷别名, 语义同 gin.H。
type H = map[string]any

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

// Deps 管理台依赖。
type Deps struct {
	Assets  *assets.Manager
	Client  *api.BotAPI
	Gateway func() any
	Control Control
	Profile *profileStore
}

// Control 运行控制回调, 由 main 注入。
type Control struct {
	Restart    func()
	Stop       func()
	Supervised bool
}

// Register 挂载全部 /admin 路由。
func Register(mux *http.ServeMux, deps Deps) {
	sess := newSessions()
	bus := newLiveBus(deps)

	mux.HandleFunc("POST /admin/api/login", handleLogin(sess))
	mux.HandleFunc("POST /admin/api/logout", handleLogout(sess))

	api := http.NewServeMux()
	api.HandleFunc("GET /admin/api/me", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, H{"ok": true, "admin": true})
	})
	api.HandleFunc("GET /admin/api/overview", handleOverview(deps))
	api.HandleFunc("GET /admin/api/profile", handleProfileGet(deps))
	api.HandleFunc("POST /admin/api/profile/refresh", handleProfileRefresh(deps))
	api.HandleFunc("GET /admin/api/stream", bus.handleStream)
	api.HandleFunc("GET /admin/api/logs", handleLogs)
	registerPluginRoutes(api)
	if deps.Assets != nil {
		registerAssetsRoutes(api, deps.Assets)
	}
	api.HandleFunc("GET /admin/api/jobs", handleJobs)
	api.HandleFunc("POST /admin/api/jobs/{id}/pause", handleJobPause)
	api.HandleFunc("GET /admin/api/config", handleGetConfig)
	api.HandleFunc("PUT /admin/api/config", handlePutConfig(deps))
	api.HandleFunc("POST /admin/api/system/restart", func(w http.ResponseWriter, r *http.Request) {
		if !deps.Control.Supervised {
			writeJSON(w, http.StatusConflict, H{"error": "未声明受外部守护(AURORIX_SUPERVISED=1), 面板重启会与守护进程各拉一个实例互杀; 请手动重启或在受管模式下使用"})
			return
		}
		triggerControl(w, deps.Control.Restart, "重启")
	})
	api.HandleFunc("POST /admin/api/system/stop", func(w http.ResponseWriter, r *http.Request) {
		triggerControl(w, deps.Control.Stop, "停止")
	})
	api.HandleFunc("/admin/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, H{"error": "接口不存在"})
	})
	mux.Handle("/admin/api/", requireAuth(sess, api))

	for id, ui := range plugin.WebUIs() {
		prefix := "/" + id
		api := http.NewServeMux()
		if ui.API != nil {
			ui.API(api)
		}
		api.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusNotFound, H{"error": "接口不存在"})
		})
		mux.Handle(prefix+"/api/", requireAuth(sess, http.StripPrefix(prefix, api)))

		if ui.SPA != nil {
			mux.Handle(prefix+"/", http.StripPrefix(prefix, plugin.ServeSPA(ui.SPA)))
		} else {
			mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
				http.NotFound(w, r)
			})
		}
	}

	mux.HandleFunc("/", serveSPA)
}

func requireAuth(sess *sessions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := config.Current()
		if cfg.AdminPassword == "" {
			if isLoopback(r.RemoteAddr) {
				next.ServeHTTP(w, r)
				return
			}
			writeJSON(w, http.StatusServiceUnavailable, H{"error": "远程管理未启用，请在 config.json 中设置 admin_password"})
			return
		}
		token, err := r.Cookie(sessionCookie)
		if err != nil || !sess.valid(token.Value, cfg.AdminPassword) {
			writeJSON(w, http.StatusUnauthorized, H{"error": "未登录或会话已失效"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func triggerControl(w http.ResponseWriter, action func(), name string) {
	if action == nil {
		writeJSON(w, http.StatusNotImplemented, H{"error": name + "控制未启用"})
		return
	}
	writeJSON(w, http.StatusAccepted, H{"ok": true, "notice": name + "已触发"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		action()
	}()
}

func serveSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, H{"error": "接口不存在"})
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/admin")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "index.html"
	}
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if name != "index.html" {
		if f, err := fsys.Open(name); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeFileFS(w, r, fsys, name)
				return
			}
		}
		if filepath.Ext(name) != "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(index)
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
