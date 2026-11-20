# API 全景参考

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [API 参考](api.md) · [发布](publishing.md)

本篇穷尽插件可用的框架 API：门面入口、流式消息、HTTP 客户端、日志、常量。按主题文档（commands/message/schedule/storage/events/push）是使用向导，本篇是符号索引。

## BotAPI 门面（ctx.Qapi）

`MessageContext` / `ScheduleContext` / `CallbackContext` 都经 `ctx.Qapi` 触达完整门面。全表：

| 方法 | 说明 |
|---|---|
| `Session(constant.Target)` | 取一个场景会话，发送 / 撤回 / 上传都走它（见下） |
| `Group.GetInfo(groupOpenID)` | 群资料（含群名） |
| `Channel.Info / Guild / Guilds / Channels / Member` | 子频道、频道、频道列表与频道成员 |
| `InteracteCallback(eventId)` | 按钮交互回执 |
| `AcceptGroupJoinRequest(rid, gid, uid)` | 同意入群申请 |
| `RejectGroupJoinRequest(rid, gid, uid, reason)` | 拒绝入群申请 |
| `RejectGroupJoinRequestAndAddToBlacklist(...)` | 拒绝并入黑名单 |
| `SendStreamMessage(userID, StreamMessage)` | 流式消息分片（见下） |
| `NewStreamSession(userID, eventID, msgID)` | 创建流式会话 |
| `GatewayBot()` / `GatewayURL()` | 网关信息（框架内部使用为主） |
| `AccessToken()` | 当前 AccessToken |

## Session：一个会话的出口

`constant.Target{Scene, ID, User}` 描述「这条消息发给谁」，`Session` 把该场景的收发地址、能力差异收在一处：

`ctx.Qapi.Session(target)` 的用法就是五个动作: `Send(data)` 发一条已序列化的消息并返回平台消息 ID、`Recall(id, hideTip)` 撤回、`Upload(...)` 上传媒体(仅群/单聊)、`SendImage(...)` 与 `SendURLImage(...)` 发图(仅频道域)。

场景决定地址：`group` → `/v2/groups/{id}/messages`、`c2c` → `/v2/users/{id}/messages`、`channel` → `/channels/{id}/messages`、`dm` → `/dms/{id}/messages`。该场景没有的能力返回 `*api.Unsupported`，错误信息直接说明「哪个场景不支持什么」。

## 流式消息

一条消息在会话内连续分片更新（如逐字生成），仅私聊场景。三步: `NewStreamSession(userID, eventID, msgID)` 建会话(eventID/msgID 取触发事件的消息标识, 没有被动消息就传空串) → `Update(内容)` 反复替换 → `Finish(内容)` 收尾。

| StreamSession 方法 | 说明 |
|---|---|
| `Update(content)` | 更新当前内容（生成中状态） |
| `Finish(content)` | 写入最终内容并结束 |
| `SendContent(content, inputState)` | 底层分片发送，可自控状态 |
| `StreamID()` | 已建立的流式消息 ID |

状态常量：`StreamNotStream(0)` / `StreamGenerating(1)` / `StreamDone(10)`。

## HTTP 客户端（ctx.Request）

外部请求一律走 `ctx.Request`（内置超时、连接池、重试），勿自建裸 client。方法见下表，`headers` 传 nil 即可。

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

插件里 `logx.New("myplugin")` 取一个带来源的记录器(全局一次即可), 然后 `Debugf` / `Infof` / `Warnf` / `Errorf`。

日志进管理台「日志」页（级别/来源筛选、实时推送），级别受 `config.json` 的 `log_level` 控制。

## 常量

| 包 | 常量 | 说明 |
|---|---|---|
| `constant.RoleRequired` | `RoleOwner` / `RoleAdmin` / `RoleMember` | 指令最低权限 |
| `constant.Scene` | `SceneGroup` / `SceneC2C` / `SceneChannel` / `SceneDirect` | 会话场景，与面板 scope、存储命名空间同一套单词 |
| `constant.Target` | `{Scene, ID, User}` | 一次投递的会话目标：场景 + 会话 ID + 触发者 |
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

## 封禁名单（`lib/ban`）

框架在指令命中之后、权限判定之前查这张名单: 命中就按记录里的话术回一句(同一个人 15 秒内只回一次), 不再往下走, 按钮点击同样拒。插件读写的是同一份: `ban.Put(Entry)`(先写库成功才改内存)、`ban.Hit(openid)`(返回该回的话与是否命中)、`ban.List()`(最近封的在前)、`ban.Delete(openid)` 解封。`Entry.Message` 留空用 `ban.DefaultMessage`, 上限 `ban.MessageLimit`(2000 字); 名单存在 `kv_data` 的 global 作用域, 启动时整份读进内存, 写时整份写回。

## 收件箱（`lib/inbox`）

把机器人收发的消息落进 PostgreSQL 并聚合成会话, 管理台「收件箱」页读的就是它。插件通常不需要碰它; 要在消息入库后做旁路, 挂 `inbox.SetSavedHook(fn)`, 存成功之后才会被调用。保留策略写死在代码里(90 天、每个会话 500 条), 启动时清理一次。数据库不可用时只是不存档, 收发照旧。

## 未覆盖的底层包

以下包属于框架内部实现，插件不直接 `import`，本篇不做符号级索引；它们的效果都通过配置或其它文档体现：

| 包 | 作用 | 在哪看 |
|---|---|---|
| `lib/admin` | 管理台与它的 HTTP 接口 | 仓库根 README 的「管理台」一节 |
| `lib/gateway` | WebSocket 网关：订阅、重连与退避 | [configuration.md](configuration.md) 的 `protocol` / `intents` |
| `lib/middleware` | 入站校验、事件分发与错误兜底 | 行为经 [events.md](events.md) 体现 |
| `lib/structers` | 入站协议结构 | 经 `MessageContext` 字段暴露，见 [message.md](message.md) |
| `lib/notice` | 入群欢迎与退群提示 | [configuration.md](configuration.md) 的 `welcome_*` / `farewell_*` |
