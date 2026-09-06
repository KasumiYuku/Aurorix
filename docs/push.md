# 主动推送（HTTP）

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

框架内置主动推送服务（`lib/push`）：**任何外部系统**通过 HTTP 把消息推给机器人，机器人转发到指定群或私聊。适合定时提醒、监控告警、外部系统通知等场景——无需登录 QQ，一个 `curl` 即可。

## 三步开通

```bash
# 1. 机器人端: 允许推送到目标会话 (群主/机器人所有者在目标群里发)
#    /enablepush    开启推送
#    /pushkey 123456 # 设置该会话的推送密钥
#    /pushstatus    查看状态
#    /disablepush   关闭推送

# 2. 外部系统: 携带密钥 POST 推送 (X-Push-Key 头或 body 的 key 字段)
curl -X POST 'http://你的地址:8080/push/group/群OpenID' \
  -H 'Content-Type: application/json' \
  -H 'X-Push-Key: 123456' \
  -d '{"type":"markdown","content":"# 告警\n服务器负载过高"}'

# 3. 机器人收到后转发到目标群
```

## 端点

```
POST /push/:scope/:openid
```

| 参数 | 值 | 说明 |
|---|---|---|
| `scope` | `group` / `private` | 推送目标类型 |
| `openid` | QQ 开放平台 OpenID | 群 OpenID 或用户 OpenID（`/pushstatus` 可查询当前会话 ID） |

## 请求体

```json
{
  "key": "123456",
  "type": "markdown",
  "content": "# 通知内容"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `key` | 否 | 推送密钥；也可用 `X-Push-Key` 请求头（头部优先） |
| `type` | 否 | 消息类型：`text`（默认）/ `markdown`（或 `md`） |
| `content` | 是 | 消息内容；`text` 为纯文本，`markdown` 为 Markdown 原文 |

## 鉴权与前置条件

- **密钥**：目标会话必须先用 `/pushkey` 设置密钥。密钥用常量时间比较（防侧信道枚举）
- **开启**：目标会话必须 `/enablepush`，未开启返回 403
- **限流**：密钥错误时全局滑动窗口限速（20 次/分钟），超限返回 429——防暴力枚举

## 响应

| 状态码 | 场景 |
|---|---|
| `200` | 推送成功，`{"ok":true}` |
| `400` | scope 非法 / openid 缺失 / 请求体非法 / content 为空 / type 不支持 |
| `401` | 密钥缺失或错误 |
| `403` | 目标会话未开启推送 |
| `429` | 触发限流 |
| `502` | QQ 发送失败（附错误详情） |

错误响应体：`{"ok":false,"error":"原因"}`。

## 进程内推送对照

HTTP 端点适合**外部系统**。插件代码内部主动推送（定时任务、事件处理中）直接用 Go API，无需走 HTTP：

```go
ctx.Qapi.SendGroupMessage(payload, groupID)     // 群
ctx.Qapi.SendPrivateMessage(payload, userID)    // 私聊
```

定时任务预设目标后 `ctx.Text(...).Send()` 直达（见 [schedule.md](schedule.md)）。

## 管理指令

| 指令 | 角色 | 说明 |
|---|---|---|
| `/enablepush` | 所有者 | 允许向当前会话推送 |
| `/disablepush` | 所有者 | 关闭当前会话推送 |
| `/pushkey <密钥>` | 所有者 | 设置当前会话推送密钥 |
| `/pushstatus` | 所有者 | 查看推送状态 |
