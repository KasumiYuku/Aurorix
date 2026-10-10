# 事件

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [插件管理台](webui.md) · [API 参考](api.md) · [发布](publishing.md)

平台推送的所有事件经事件总线解析为类型化对象，插件可订阅任意事件并写回调。

## 订阅

`event.On` 注册回调，回调签名：`func(ctx *event.Context, e *具体事件类型)`

```go
import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/event"
)

func init() {
	event.On(constant.GROUP_MEMBER_ADD, func(ctx *event.Context, e *event.MemberEvent) {
		// e 直接是 *MemberEvent, 字段直接访问
		profile := queryProfile(e.UserID)

		// ctx 消息链自动发往事件所在群
		ctx.Msg().Text("欢迎 " + profile.Name + " 加入本群!").Send()
		// 一行等价: ctx.Text("...").Send()
	})
}
```

- 一个事件可多次 `On`，回调全部触发（并发执行，顺序不保证）
- `event.OnAny` 可订阅所有事件，适合做审计、日志或观察平台新增事件；业务逻辑优先订阅具体事件
- 回调 panic 由框架捕获，不影响其他订阅者与内置分发
- 未知事件按字符串名订阅即可（见下文）

## 事件对象与环境

| 事件 | 事件对象 (回调 `e`) | 字段 |
|---|---|---|
| `GROUP_AT_MESSAGE_CREATE` / `GROUP_MESSAGE_CREATE` | `*MessageEvent` | `EventID` `MessageID` `GroupID` `UserID` `Content`(已清洗@) `RawContent` `Mentions` `Attachments` `Quote` `Origin` |
| `C2C_MESSAGE_CREATE` | `*MessageEvent` | 同上（`GroupID` 空，私聊目标） |
| `INTERACTION_CREATE` | `*InteractionEvent` | `EventID` `ButtonID` `Data` `GroupID` `UserID` `MessageID` `Scene` `ChatType` |
| `GROUP_JOIN_REQUEST` | `*JoinRequestEvent` | `RequestID` `GroupID` `UserID` `Method` `Answer` `MessageID` |
| `MESSAGE_AUDIT_PASS` / `MESSAGE_AUDIT_REJECT` | `*AuditEvent` | `AuditID` `MessageID` `Approved` |
| `GROUP_MEMBER_ADD` / `GROUP_MEMBER_REMOVE` | `*MemberEvent` | `EventID` `MessageID` `GroupID` `UserID` `OperatorID` `Timestamp` `Added` |
| `GROUP_ADD_ROBOT` / `GROUP_DEL_ROBOT` | `*RobotEvent` | `GroupID` `OperatorID` `Timestamp` `Added` |
| `GROUP_MSG_RECEIVE` / `GROUP_MSG_REJECT` | `*ReceiveEvent` | `GroupID` `OperatorID` `Timestamp` `Enabled` |
| `C2C_MSG_RECEIVE` / `C2C_MSG_REJECT` | `*ReceiveEvent` | `UserID` `OperatorID` `Timestamp` `Enabled` |

> `Origin` 取值 `constant.GroupMessage` / `constant.PrivateMessage`。
>
> 各事件的 `UserID` / `OperatorID` 都取自该事件载荷里真实的身份字段（消息事件在 `author` 内，
> 交互事件与通知类事件在顶层），插件无需自己判断字段位置，用事件对象的字段即可。
> 判定交互事件是群聊还是单聊统一用 `InteractionEvent.IsPrivate()`：平台按场景只下发
> `chat_type` 与 `scene` 其中一个，两者都认才不会判错。

## ctx 能力（与指令上下文同款）

`ctx` 内嵌 `MessageManager`：`Text` / `Markdown` / `Msg()` 链 / `Image` / `At` / `MarkdownTemplate` 全部可用，发送目标自动注入（群事件发群、私聊事件发私聊）。底层 `ctx.Qapi`（`*api.BotAPI`）提供撤回、审批、回执等平台操作：

```go
// 按钮回执
event.On(constant.INTERACTION_CREATE, func(ctx *event.Context, e *event.InteractionEvent) {
	ctx.Qapi.InteracteCallback(e.EventID)
})

// 入群申请审批
event.On(constant.GROUP_JOIN_REQUEST, func(ctx *event.Context, e *event.JoinRequestEvent) {
	ctx.Qapi.AcceptGroupJoinRequest(e.RequestID, e.GroupID, e.UserID)
	// 或 ctx.Qapi.RejectGroupJoinRequest(req, group, user, "原因")
})
```

`MessageManager` 与 `BotAPI` 完整方法见 [message.md](message.md) 与 [API 参考](api.md)。

消息类事件可以用被动回复: `ctx.CanReply()` 为 true 时, 回复会带 `msg_id`/`event_id`,
不消耗主动消息额度。通知类事件里 **入群(`GROUP_MEMBER_ADD`)支持 `event_id` 被动回复**,
上下文会带出锚点; 而 **退群(`GROUP_MEMBER_REMOVE`)、戳一戳等不支持**, 带锚点会被拒
40034027。不支持的事件要发消息就用主动推送:

```go
// 主动推送: 发送时不带 event_id/msg_id
ctx.Msg().At(userID, true).Markdown("欢迎加入").Initiative().Send()
```

被动回复若遇到 `40034025`(event_id 尚未落库), 框架会自动等待 200ms 重试, 最多 3 次 ——
这是平台 event_id 落库时延导致的偶发失败, 无需调用方处理。

**主动消息按群授权**: 群未开启「允许主动消息」时发送会被拒 40034105。发之前用
`ctx.Qapi.Group.GetBotState(groupID)` 看 `AllowProactiveMsg`, 或订阅
`GROUP_MSG_RECEIVE`(开启)/`GROUP_MSG_REJECT`(关闭) 事件跟踪这个开关的变化。

## 未知事件

未注册的事件不丢弃：以 `UnknownEvent` 透传（`RawType` 原始事件名、`RawBody` 原始载荷）。
框架会尽量从常见字段里带出 `EventID` `MessageID` `GroupID` `UserID`, 方便插件对新 notice
先做观察或兼容。订阅可用具体字符串名, 也可用 `event.OnAny`：

```go
event.On(constant.EventType("PLATFORM_NEW_EVENT"), func(ctx *event.Context, e *event.UnknownEvent) {
	_ = e.RawBody
})
```

## 自定义事件

插件可用 `event.Register` 给事件换解析器，或注册新平台事件：

```go
event.Register(constant.GROUP_MESSAGE_CREATE, func(p structers.Payload) event.Event {
	return &myEvent{...} // 实现 Type() + 自定义字段
})
```
