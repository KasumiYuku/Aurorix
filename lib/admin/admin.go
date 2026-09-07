// Package admin 管理台: 鉴权、JSON API 与 SPA 静态托管。
package admin

import (
	"embed"
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/assets"
	"github.com/KasumiYuku/Aurorix/lib/config"
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

// writeJSON 写 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// readJSON 解析请求 JSON 体, 限 1MB 防超大载荷。
func readJSON(r *http.Request, dst any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}

// Deps 管理台依赖。
type Deps struct {
	Assets  *assets.Manager // 可为 nil, 为 nil 时图床接口不可用
	Client  *api.BotAPI     // 热更消息选项用
	Gateway func() any      // websocket 模式的网关状态; webhook 传 nil
	Control Control
	Profile *profileStore // 机器人档案缓存, 可为 nil
}

// Control 运行控制回调, 由 main 注入。
type Control struct {
	Restart func()
	Stop    func()
}

// Register 挂载全部 /admin 路由。
func Register(mux *http.ServeMux, deps Deps) {
	sess := newSessions()
	bus := newLiveBus(deps)

	mux.HandleFunc("POST /admin/api/login", handleLogin(sess))
	mux.HandleFunc("POST /admin/api/logout", handleLogout(sess))

	// /admin/api/* 全部受鉴权保护, 由子 mux 分发
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
		triggerControl(w, deps.Control.Restart, "重启")
	})
	api.HandleFunc("POST /admin/api/system/stop", func(w http.ResponseWriter, r *http.Request) {
		triggerControl(w, deps.Control.Stop, "停止")
	})
	api.HandleFunc("/admin/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, H{"error": "接口不存在"})
	})
	mux.Handle("/admin/api/", requireAuth(sess, api))

	// SPA 壳对未鉴权开放, 数据接口受保护; 未匹配路径一律走 serveSPA
	mux.HandleFunc("/", serveSPA)
}

// requireAuth 会话校验: 未设密码时仅回环地址放行, 有密码时校验会话 Cookie。
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

// triggerControl 触发运行控制, 延迟 300ms 让响应先发出。
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

// serveSPA 托管构建产物; 未匹配的 GET 一律回退 index.html 支持前端路由,
// 根路径与任意挂载前缀均可, 兼容裸域名/反代入口; 缺失的静态资源按扩展名 404。
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

// isLoopback 判断请求是否来自回环地址。
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
