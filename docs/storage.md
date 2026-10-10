# 配置热更与数据存储

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

## 插件配置（热更）

插件声明配置表单，管理台据此渲染，保存即生效、无需重启：

```go
plugin.Register(&plugin.Plugin{
	Id: "myplugin",
	Config: []plugin.ConfigField{
		{Key: "api_key", Label: "API 密钥", Type: "password", Required: true},
		{Key: "timeout", Label: "超时秒数", Type: "number", Placeholder: "30"},
	},
	ValidateConfig: func(v map[string]any) error {
		if v["timeout"].(float64) <= 0 {
			return errors.New("timeout 必须大于 0")
		}
		return nil
	},
	ApplyConfig: func(v map[string]any) error {
		// 应用新配置, 无需重启
		return nil
	},
})
```

| ConfigField | 说明 |
|---|---|
| `Key` | 配置项标识 |
| `Label` | 表单展示名 |
| `Description` | 帮助说明 |
| `Type` | 表单类型：`string` / `number` / `password` 等 |
| `Placeholder` | 占位提示 |
| `Required` | 必填 |

流程：管理台提交 → `ValidateConfig` 校验（返回 error 拒绝）→ 保存 → `ApplyConfig` 生效。配置存放在 `config.json` 的 `plugin_settings` 中。

## 数据存储

SQLite 持久化，按命名空间隔离。包路径 `github.com/KasumiYuku/Aurorix/lib/storage`，数据库路径由配置 `database` 指定（实例默认 `data/bot.db`）。

### 命名空间

```go
ctx.PluginStorage    // 当前插件
ctx.CommandStorage   // 当前指令
ctx.UserStorage      // 当前用户
ctx.GroupStorage     // 当前群
ctx.GlobalStorage    // 全局

storage.Plugin("myplugin")   // 任意位置按插件取
storage.Command("myplugin", "cmd")
storage.User("用户OpenID")
storage.Group("群OpenID")
storage.Global()
```

### Store 方法

| 方法 | 说明 |
|---|---|
| `Set(key, value)` | 写入，任意 JSON 序列化对象 |
| `Get(key, &target)` | 读取到目标对象，返回 `(found, err)` |
| `Has(key)` | 存在性检查 |
| `Delete(key)` | 删除 |
| `Clear()` | 清空当前命名空间 |

零保留键，所有 key 自由使用。底层 `modernc.org/sqlite` 纯 Go 驱动，无需 CGO。

### 示例

```go
func remember(ctx *context.MessageContext) error {
	var count int
	ctx.PluginStorage.Get("count", &count)
	count++
	ctx.PluginStorage.Set("count", count)
	return ctx.Text(fmt.Sprintf("第 %d 次", count)).Send()
}
```