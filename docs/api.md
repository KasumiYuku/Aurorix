# API 全景参考

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

本篇穷尽插件可用的框架 API：门面入口、流式消息、HTTP 客户端、日志、常量。按主题文档（commands/message/schedule/storage/events/push）是使用向导，本篇是符号索引。

## BotAPI 门面（ctx.Qapi）

`MessageContext` / `ScheduleContext` / `CallbackContext` 都经 `ctx.Qapi` 触达完整门面。全表：

| 方法 | 说明 |
|---|---|
| `SendGroupMessage(data, groupId)` | 群发消息（data 为 JSON 序列化消息结构） |
| `SendPrivateMessage(data, userId)` | 私聊发消息 |
| `UploadImage(target, groupID, userID, filePath)` | 上传图片返回 file_info |
| `UploadMedia(target, groupID, userID, MediaUpload)` | 上传媒体（图片/视频/语音/文件） |
| `RecallGroupMessage(openID, msgID, hideTip)` | 撤回群消息 |
| `RecallC2CMessage(openID, msgID, hideTip)` | 撤回私聊消息 |
| `InteracteCallback(eventId)` | 按钮交互回执 |
| `AcceptGroupJoinRequest(rid, gid, uid)` | 同意入群申请 |
| `RejectGroupJoinRequest(rid, gid, uid, reason)` | 拒绝入群申请 |
| `RejectGroupJoinRequestAndAddToBlacklist(...)` | 拒绝并入黑名单 |
| `SendStreamMessage(userID, StreamMessage)` | 流式消息分片（见下） |
| `NewStreamSession(userID, eventID, msgID)` | 创建流式会话 |
| `GatewayBot()` / `GatewayURL()` | 网关信息（框架内部使用为主） |
| `AccessToken()` | 当前 AccessToken |

## 流式消息

一条消息会话内连续分片更新（如逐字生成内容），仅私聊场景。三步：

```go
// 1. 创建会话 (eventID/msgID 取触发事件的消息标识; 无被动消息时传空串)
sess := ctx.Qapi.NewStreamSession(userID, "", "")

// 2. 持续更新: 每次调用替换上一条内容
sess.Update("# 思考中...")        // 输入状态: 生成中
sess.Update("# 结论\n内容...")

// 3. 结束 (输入状态: 完成)
sess.Finish("## 最终结果")
```

| StreamSession 方法 | 说明 |
|---|---|
| `Update(content)` | 更新当前内容（生成中状态） |
| `Finish(content)` | 写入最终内容并结束 |
| `SendContent(content, inputState)` | 底层分片发送，可自控状态 |
| `StreamID()` | 已建立的流式消息 ID |

状态常量：`StreamNotStream(0)` / `StreamGenerating(1)` / `StreamDone(10)`。

## HTTP 客户端（ctx.Request）

外部请求一律走 `ctx.Request`（内置超时、连接池、重试），勿自建裸 client：

```go
var result map[string]any
ctx.Request.Get("https://api.example.com/x", &result, nil)
ctx.Request.Post("https://api.example.com/x", body, &result, nil)
ctx.Request.Put(url, body, &result, nil)
ctx.Request.Patch(url, body, &result, nil)
ctx.Request.Delete(url, nil, &result, nil)
```

| 方法 | 说明 |
|---|---|
| `Get(url, &result, headers)` | GET，JSON 解码到 result |
| `Post(url, body, &result, headers)` | POST（body 自动 JSON 化） |
| `Put` / `Patch` / `Delete` | 同构 |
| `PostForm(url, form, &result, headers)` | 表单提交 |
| `PostMultipart(url, mp, &result, headers)` | 文件上传 |
| `DoBytes(method, url, body, headers)` | 原始字节往返 |
| `DoBytesTimeout(...)` | 带单次超时（分片直传等慢通道） |

body 构造器：`requests.JSON(v)` / `requests.Bytes([]byte)` / `requests.Form(url.Values)`。

## 日志

```go
var log = logx.New("myplugin")   // 插件内全局一次

log.Debugf("调试: %v", v)
log.Infof("信息: %v", v)
log.Warnf("警告: %v", v)
log.Errorf("错误: %v", v)
```

日志进管理台「日志」页（级别/来源筛选、实时推送），级别受 `config.json` 的 `log_level` 控制。

## 常量

| 包 | 常量 | 说明 |
|---|---|---|
| `constant.RoleRequired` | `RoleOwner` / `RoleAdmin` / `RoleMember` | 指令最低权限 |
| `constant.MessageOrigin` | `GroupMessage` / `PrivateMessage` | 消息来源 / 发送目标 |
| `constant.MessageType` | `PlainText` / `Markdown` | 消息类型 |
| `constant` | `MediaVideo=2` / `MediaVoice=3` / `MediaFile=4` | 媒体类型 |
| `constant.EventType` | `GROUP_AT_MESSAGE_CREATE` 等 | 事件类型（事件名同 README 可订阅表） |

## 错误类型

发送相关错误可用 `errors.As` 识别：

| 类型 | 触发 |
|---|---|
| `message.MessageUsed` | 同一消息对象二次 Send |
| `message.JSONMarshalError` | 消息序列化失败 |
| `message.ReplyMessageReachLimit` | 回复达到上限 |

## 未覆盖的底层包

`lib/gateway`（网关）、`lib/admin`（管理台）、`lib/middleware`（中间件）、`lib/structers`（入站协议结构）属于框架内部实现，插件不直接依赖。入站消息结构（`UserMessage` / `Mention` / `Quote` / `Attachment`）经 `MessageContext` 字段暴露，见 [message.md](message.md)。
