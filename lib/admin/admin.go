// Package admin 管理台: 鉴权、JSON API 与 SPA 静态托管。
package admin

import (
	"Plrx/lib/api"
	"Plrx/lib/assets"
	"Plrx/lib/config"
	"embed"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var distFS embed.FS

// Deps 管理台依赖。
type Deps struct {
	Assets  *assets.Manager // 可为 nil, 为 nil 时图床接口不可用
	Client  *api.BotAPI     // 热更消息选项用
	Gateway func() any      // websocket 模式的网关状态; webhook 传 nil
	Control Control
}

// Control 运行控制回调, 由 main 注入。
type Control struct {
	Restart func()
	Stop    func()
}

// Register 挂载全部 /admin 路由。
func Register(engine *gin.Engine, deps Deps) {
	sess := newSessions()
	bus := newLiveBus(deps)
	admin := engine.Group("/admin")

	admin.POST("/api/login", handleLogin(sess))
	admin.POST("/api/logout", handleLogout(sess))

	api := admin.Group("/api")
	api.Use(requireAuth(sess))
	{
		api.GET("/me", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true, "admin": true}) })
		api.GET("/overview", handleOverview(deps))
		api.GET("/stream", bus.handleStream)
		api.GET("/logs", handleLogs)
		registerPluginRoutes(api)
		if deps.Assets != nil {
			registerAssetsRoutes(api, deps.Assets)
		}
		api.GET("/jobs", handleJobs)
		api.POST("/jobs/:id/pause", handleJobPause)
		api.GET("/config", handleGetConfig)
		api.PUT("/config", handlePutConfig(deps))
		api.POST("/system/restart", func(c *gin.Context) { triggerControl(c, deps.Control.Restart, "重启") })
		api.POST("/system/stop", func(c *gin.Context) { triggerControl(c, deps.Control.Stop, "停止") })
	}

	// SPA 壳对未鉴权开放, 数据接口受保护; 未匹配路径(含前端路由与静态资源)一律走 NoRoute
	engine.NoRoute(serveSPA)
}

// requireAuth 会话校验: 未设密码时仅回环地址放行, 有密码时校验会话 Cookie。
func requireAuth(sess *sessions) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Current()
		if cfg.AdminPassword == "" {
			if isLoopback(c.Request.RemoteAddr) {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "远程管理未启用，请在 config.json 中设置 admin_password"})
			return
		}
		token, err := c.Cookie(sessionCookie)
		if err != nil || !sess.valid(token, cfg.AdminPassword) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录或会话已失效"})
			return
		}
		c.Next()
	}
}

// triggerControl 触发运行控制, 延迟 300ms 让响应先发出。
func triggerControl(c *gin.Context, action func(), name string) {
	if action == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": name + "控制未启用"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "notice": name + "已触发"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		action()
	}()
}

// serveSPA 托管构建产物; 未匹配的 GET 一律回退 index.html 支持前端路由,
// 根路径与任意挂载前缀均可, 兼容裸域名/反代入口; 缺失的静态资源按扩展名 404。
func serveSPA(c *gin.Context) {
	if c.Request.Method != http.MethodGet {
		c.Status(http.StatusNotFound)
		return
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
		return
	}

	name := strings.TrimPrefix(c.Request.URL.Path, "/admin")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		name = "index.html"
	}
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	if name != "index.html" {
		if f, err := fsys.Open(name); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeFileFS(c.Writer, c.Request, fsys, name)
				return
			}
		}
		if filepath.Ext(name) != "" {
			c.Status(http.StatusNotFound)
			return
		}
	}

	index, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", index)
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
