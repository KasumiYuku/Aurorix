# 消息开发

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

`MessageContext` 通过嵌入链（`MessageContext → Context → MessageManager`）直接持有全部消息构造方法。每个方法返回可 `.Send()` 的消息对象。

## 构造器全表

| 方法 | 返回 | 说明 |
|---|---|---|
| `Text(content)` | TextMessage | 纯文本。全局 Markdown 开启时自动转 Markdown 渲染 |
| `Markdown(content)` | MarkdownMessage | Markdown 消息，支持图片内嵌与按钮 |
| `Image(src, summary, size...)` | ImageMessage | 图片。`src` 支持路径 / []byte / base64 / data URL / 公网 URL |
| `Voice(src)` | UploadMessage | 语音（Send 时自动上传 QQ） |
| `Video(src)` | UploadMessage | 视频 |
| `File(src, name)` | UploadMessage | 文件 |
| `Media(fileInfo)` | MediaMessage | 引用已上传媒体（file_info 来自 QQ 上传接口） |
| `At(openID, newline...)` | AtMessage | @ 某人；`newline=true` 时 Markdown 中 @ 后换行独占一行 |
| `AtAll(newline...)` | AtMessage | @ 所有人 |
| `Quote(messageID)` | Message | 引用回复（构造器链上追加引用） |

发送：每个构造器返回的对象都有 `Send() error`：

```go
ctx.Text("你好").Send()
ctx.Markdown("# 标题\n内容").Send()
ctx.Image(pathOrURL, "图片描述").Send()
ctx.File(src, "report.pdf").Send()
```

**图片尺寸**：`ctx.Image(src, summary, width, height)` 显式标注宽高；不传则按图床探测 / URL 探测自动补齐。发送形态（Markdown 内嵌 / 独立媒体）由框架按上下文自动决策。

## 组合消息

一条消息由多个部件组成时用 `MsgBuilder`：

```go
ctx.Msg().
	Text("结果：\n").
	At(openID, true).
	Markdown("**详情**").
	Image(src, "配图").
	Send()
```

| 方法 | 说明 |
|---|---|
| `Add(parts...)` | 追加消息部件（见下） |
| `Text(s)` `At(openid, newline...)` `AtAll` | 文本与 @ |
| `Markdown(s)` | Markdown 部件 |
| `Image(src, summary, size...)` | 图片部件 |
| `Voice` `Video` `File(src, name)` | 媒体部件 |
| `Keyboard(kb)` | 追加按钮键盘（见[按钮键盘](#按钮键盘)） |
| `Quote(messageID)` | 引用原消息 |
| `Send()` | 发送整条组合消息 |

**消息部件（Part）**：部件即上述构造器的返回值——`TextMessage` / `MarkdownMessage` / `ImageMessage` / `AtMessage` / `MediaMessage` / `UploadMessage` 六种，`Add()` 可任意组合追加。部件只能在 `Msg()` 链内组合；独立发送用构造器直接 `.Send()`。

## Message 语义

- **单次发送**：一个消息对象只能 `Send()` 一次，重复发送返回 `MessageUsed` 错误——每条消息构造一个对象，勿复用
- **引用**：`Quote(messageID)` 或 `msg.QuoteTo(messageID)` 让消息引用原消息
- **目标**：发送目标由构造时的上下文决定（群消息回群、私聊回私聊）；主动推送用 `ctx.Qapi` 指定目标（见[推送](push.md)）

## 按钮键盘

键盘最多 5 行、每行最多 5 个按钮。三种动作类型：命令 / 链接 / 回调。

### 构造

```go
kb := &buttons.Keyboard{}

// 命令按钮: 点击后自动执行指令
b, _ := kb.AppendButton("btn_id", "查看状态", "已查看", buttons.Gray, 0)
b.SetAutoCommand("/uptime", true, false)
//   autoSend=true 自动发送指令文本; anchor=true 唤起图片选择器

// 链接按钮: 点击跳转
b2, _ := kb.AppendButton("btn_url", "文档", "", buttons.Blue, 0)
b2.SetHref("https://example.com")

// 回调按钮: 点击触发回调函数, data 原样传给回调
b3, _ := kb.AppendButton("btn_cb", "签到", "", buttons.Blue, 1)
b3.SetCallback("payload", func(ctx *context.CallbackContext) error {
	return ctx.Text("已签到").Send()
})
```

### 按钮动作（Action）

| 方法 | 类型 | 说明 |
|---|---|---|
| `SetAutoCommand(content, autoSend, anchor)` | 命令 | 点击后执行 `content` 指令。`autoSend` 自动发送；`anchor` 唤起图片选择 |
| `SetHref(url)` | 链接 | 跳转指定 URL |
| `SetCallback(data, handle)` | 回调 | 点击回调。`data` 传给回调上下文；`handle` 自动注册到该按钮 ID |
| `SetCallbackWithoutHandle(data)` | 回调 | 只下发回调数据不注册处理，配合全局 `buttons.RegisterCallbackFunc(id, handle)` 用 |

### 按钮外观与权限

| 方法 | 说明 |
|---|---|
| `AppendButton(id, label, visited, style, row)` | 添加按钮。`visited` 为点击后展示文案；`style` 为 `buttons.Gray` / `buttons.Blue`；`row` 0-4（行满 5 个报错） |
| `SetPermission(perm)` | 点击权限：`buttons.SomeUser` / `buttons.Admin` / `buttons.AllUser` |
| `SetUserWhiteList(users)` | 仅指定用户可点（自动置为 `SomeUser`） |
| `SetUnsupportedTip(tip)` | 客户端不支持按钮时的提示文案 |

### 发送与回调

```go
// 发送 (keyboard 实现 contract.CanMarshal)
ctx.Msg().Text("选择操作").Keyboard(kb).Send()

// 回调处理两种写法:
// 1. SetCallback 内联 (见上)
// 2. 全局注册 + SetCallbackWithoutHandle
buttons.RegisterCallbackFunc("btn_cb", func(ctx *context.CallbackContext) error {
	// ctx.Data   回调数据 (SetCallback 的 data)
	// ctx.ButtonId 按钮 ID
	// ctx.PluginId 所属插件
	// ctx.InteractionID 平台交互 ID
	ctx.Done()   // 3 秒内回执, 终止 QQ 端按钮 loading
	return nil
})
```

- 回调上下文 `CallbackContext`：字段 `Data` / `PluginId` / `ButtonId` / `InteractionID`
- `Done()` 回执交互；**必须在 3 秒内调用**，超时按钮判定失败
- 按钮交互依赖 `INTERACTION_CREATE` 事件（websocket 模式记得订阅）

## MessageContext 字段

| 字段 | 说明 |
|---|---|
| `Raw` | 原始消息文本 |
| `Parsed` | 解析后的参数（`Args` 结构体指针或剩余文本） |
| `Mentions` | @ 提及列表 |
| `Quote` | 引用消息 |
| `Emojis` | 表情文本 |
| `AttachmentTypes` | 附件分类 |
| `AvatarURL` | 发送者头像 |
| `GroupId` / `UserId` | 群 OpenID / 用户 OpenID |
| `Target` | `constant.GroupMessage` / `constant.PrivateMessage` |

## Markdown 模板

模板随插件/框架通过 `go:embed` 编译进二进制，按文件名注册，`{{.key}}` 占位：

```go
// 插件内 templates/markdown/MyCard.md:
// # {{.title}}
// {{.body}}

//go:embed templates/markdown/*.md
var templateFS embed.FS

plugin.Register(&plugin.Plugin{Id: "my", TemplateFS: templateFS, ...})

// 插件指令内
ctx.MarkdownTemplate("MyCard", &templates.Args{"title": "周报", "body": "本周..."}).Send()
ctx.UnsafeMarkdownTemplate("MyCard", &templates.Args{}) // 填充失败时 panic
```

### 命名空间解析

- 插件 `TemplateFS` 下的模板注册到**插件命名空间**（插件 ID），`ctx.MarkdownTemplate` 优先查插件命名空间，未命中回落**全局命名空间**（框架内置模板）
- 无插件上下文（定时任务/框架内部）直接查全局命名空间
- 同名模板在不同插件间互不干扰；框架内置模板（如 `Card`）由全局命名空间提供
- 模板增删改后重新编译生效，无运行时文件依赖

> **注意**：插件模板目录是 embed 资源，发布插件时必须进 git。若仓库 `.gitignore` 全局忽略了 `*.md`，请显式放行 `templates/markdown/*.md`，否则拉取者编译会因缺文件失败。

## 消息撤回

机器人撤回自己发过的消息（群/私聊通用）：

```go
// 群消息: hideTip=true 时群内不提示"撤回了一条消息"
ctx.Qapi.RecallGroupMessage(groupOpenID, messageID, true)

// 私聊消息
ctx.Qapi.RecallC2CMessage(openID, messageID, false)
```

`messageID` 为要撤回的消息 ID（发送结果或入站消息带）。

## 媒体上传

`ctx.Image/Voice/File` 等构造器已自动处理上传。底层能力直接可用：

```go
// 上传图片文件返回 file_info (之后可用 ctx.Media(fileInfo) 发送)
fileInfo, err := ctx.Qapi.UploadImage(constant.GroupMessage, groupID, userID, "/path/to/img.png")

// 上传媒体 (自定义类型等), MediaUpload{FileType: 1图片 2视频 3语音 4文件}
fileInfo, err := ctx.Qapi.UploadMedia(constant.GroupMessage, groupID, userID,
	api.MediaUpload{FileType: 1, Data: data, Filename: "a.png"})
```

超过配置 `upload_threshold`（默认 3MB）自动走分片上传，无需关心。

## 入群审批

```go
// 同意
ctx.Qapi.AcceptGroupJoinRequest(requestId, groupId, userId)

// 拒绝 (reason 为拒绝理由)
ctx.Qapi.RejectGroupJoinRequest(requestId, groupId, userId, reason)

// 拒绝并拉黑
ctx.Qapi.RejectGroupJoinRequestAndAddToBlacklist(requestId, groupId, userId, reason)
```

入群申请的拦截回调见 [events.md](events.md)。

## 主动推送（无消息上下文时）

进程内主动推送（定时任务 / HTTP 服务 / 任何有 `*api.BotAPI` 的地方）：

```go
// 消息结构 (与 ctx 构造器产出同构, 此处演示组一个 Markdown)
payload, _ := json.Marshal(map[string]any{
	"msg_type": 2,
	"markdown": map[string]any{"content": "# 通知\n内容"},
})
ctx.Qapi.SendGroupMessage(payload, groupID)     // 群
ctx.Qapi.SendPrivateMessage(payload, userID)    // 私聊
```

定时任务可在 `Job` 预设 `GroupId` / `UserId` / `Target`，触发后 `ctx.Text(...).Send()` 自动直达（见 [schedule.md](schedule.md)）。

**外部 HTTP 推送**（第三方服务推消息给机器人）：框架内置 `/push/:scope/:openid` 端点，完整协议见 [push.md](push.md)。
