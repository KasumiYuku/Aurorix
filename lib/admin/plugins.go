package admin

import (
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerPluginRoutes 插件目录/详情/配置/访问控制, 契约与旧版一致。
func registerPluginRoutes(api *gin.RouterGroup) {
	api.GET("/plugins", func(c *gin.Context) {
		c.JSON(http.StatusOK, plugin.ManagedPlugins())
	})
	api.GET("/plugins/:id", func(c *gin.Context) {
		managed, ok := plugin.ManagedPluginByID(c.Param("id"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "插件不存在"})
			return
		}
		c.JSON(http.StatusOK, managed)
	})
	api.PUT("/plugins/:id", func(c *gin.Context) {
		var input map[string]any
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
			return
		}
		prepared, err := plugin.PrepareConfiguration(c.Param("id"), input)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := config.SavePluginSettings(c.Param("id"), prepared); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败"})
			return
		}
		if err := plugin.ApplyConfiguration(c.Param("id"), prepared); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	api.PUT("/plugins/:id/access", func(c *gin.Context) {
		var input plugin.AccessConfig
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式无效"})
			return
		}
		prepared, err := plugin.PrepareAccessConfiguration(c.Param("id"), input)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		if err := config.SavePluginAccess(c.Param("id"), persisted); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存访问控制失败"})
			return
		}
		plugin.ApplyAccessConfiguration(c.Param("id"), prepared)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

func toConfigAccessRule(rule plugin.AccessRule) config.AccessRule {
	return config.AccessRule{Mode: rule.Mode, Users: rule.Users, Groups: rule.Groups}
}
