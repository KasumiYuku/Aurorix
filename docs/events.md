# 事件与能力

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

## 群事件

### 入群申请

全局注册一次处理函数，所有入群申请统一走它：

```go
func init() {
	plugin.SetGlobalJoinGroupHandle(func(ctx *context.ApplyJoinGroupContext) {
		ctx.Accept()
		// ctx.Deny(reason)
		// ctx.DenyAndAddToBlacklist(reason)
	})
}
```

需订阅 `GROUP_JOIN_REQUEST` 事件。处理函数返回 `error`，框架按返回值处理。

### 成员变动

`GROUP_MEMBER_ADD` / `GROUP_MEMBER_REMOVE` 事件经事件总线分发，按需订阅（见根 README「可订阅事件」）。

## 按钮交互

```go
// 发送带按钮的消息
ctx.Msg().Text("选择操作").Keyboard(keyboard).Send()

// 注册按钮回调
buttons.RegisterCallbackFunc("action_id", func(ctx *context.CallbackContext) {
	// 处理点击
	ctx.Done() // 3 秒内回执, 终止 QQ 端按钮 loading
})
```

- 按钮点击产生 `INTERACTION_CREATE` 事件，需订阅
- `CallbackContext.Done()` 必须在 3 秒内回执，否则 QQ 端按钮判定超时

## HTTP 客户端

外部请求一律走 `ctx.Request`（内置超时、连接池、重试），勿自建裸 client：

```go
var result map[string]any
ctx.Request.Get("https://api.example.com/x", &result, nil)
ctx.Request.Post("https://api.example.com/x", body, &result, nil)
```

## 图床 Provider

上传失败自动切换 Provider、`whitelist` 直通等能力由框架承担。自定义 Provider 只需实现接口并在 `init()` 注册，管理台自动出现配置表单：

```go
func init() {
	assets.Register("mine", newMine, []assets.ConfigField{
		{Key: "token", Label: "访问令牌", Type: "password", Required: true},
	})
}

type mine struct{ cl *assets.Client; token string }

func newMine(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) { /* ... */ }
func (p *mine) Name() string { return "mine" }
func (p *mine) Upload(ctx context.Context, in assets.ProviderInput) (string, error) { /* ... */ }
```