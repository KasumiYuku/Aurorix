# 配置参考

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [插件管理台](webui.md) · [API 参考](api.md) · [发布](publishing.md)

`config.json` 是框架唯一的配置文件，放在实例目录下。管理台「设置」页编辑的是同一份文件，两处效果一致。

## 加载时机

启动时读 `./config.json`：**文件读不到、或 JSON 解析失败，会打印 `请正确配置config.json` 并直接退出**。字段取值是否合理不在这里拦——例如 `appid` 填错，要等到第一次调用平台 API 才会暴露。

读入后框架会补齐默认值（下表「默认」列即来源），然后写入内存中的当前配置。管理台保存时走增量更新：只改你提交的字段，其余原样保留。

## 最小可用配置

```json
{
  "port": 8080,
  "appid": "你的机器人AppID",
  "secret": "你的机器人AppSecret",
  "protocol": "websocket"
}
```

其余字段全部有默认值。可直接复制 `config.example.json` 作为起点。

## 全字段

「生效方式」一列的含义：**热更** = 管理台保存即生效；**需重启** = 改完重启进程才生效；**仅文件** = 管理台不提供编辑入口，只能改 `config.json` 后重启。

| 字段 | 类型 | 默认 | 生效方式 | 说明 |
|---|---|---|---|---|
| `port` | 整数 | 无（必填） | 需重启 | 服务与管理台监听端口，不能为 0 |
| `appid` | 字符串 | 无（必填） | 需重启 | 开放平台机器人 AppID |
| `secret` | 字符串 | 无（必填） | 需重启 | 开放平台机器人 AppSecret，管理台不回显 |
| `proxy` | 字符串 | `https://api.bot.qq.com` | 需重启 | 平台 API 地址。服务器 IP 不固定时，在固定 IP 的机器上反代 QQ API 并填其地址，绕过 IP 白名单 |
| `protocol` | 字符串 | `webhook` | 需重启 | `webhook` 或 `websocket`，其他值报错 |
| `intents` | 字符串数组 | 13 项常用事件 | 需重启 | 仅在 `websocket` 模式生效，见下节 |
| `gateway_url` | 字符串 | 空 | 需重启 | 自定义网关地址，须以 `ws://` 或 `wss://` 开头；仅在 `websocket` 模式生效 |
| `database` | 字符串 | `bot.db` | 需重启 | SQLite 文件路径；实例模板默认写 `data/bot.db` |
| `admin_password` | 字符串 | 空 | 热更 | 管理台密码。留空则管理台仅本机可访问；修改后现有会话立即失效 |
| `global_markdown` | 布尔 | `false` | 热更 | 所有文字按 Markdown 渲染，图片与按钮内联 |
| `retry_when` | 整数数组 | 空 | 热更 | 命中这些平台业务错误码时自动重试（最多 2 次尝试） |
| `upload_threshold` | 整数 | `3145728`（3 MiB） | 热更 | 超过该字节数改走分片上传 |
| `log_level` | 字符串 | `info` | 热更 | 控制台日志级别：`debug` / `info` / `warn` / `error` |
| `prefixes` | 字符串数组 | `["/", "#", ""]` | 需重启 | 指令前缀符号；`""` 表示允许无前缀裸指令 |
| `memory_limit_mb` | 整数 | 不设（自动推导） | 仅文件 | 见下节「内存与性能」 |
| `gc_percent` | 整数 | `0`（Go 默认） | 仅文件 | GC 触发比例，对应 `GOGC` |
| `pprof_addr` | 字符串 | 空（关闭） | 仅文件 | 开启 pprof 调试端口，见下节 |
| `plugin_settings` | 对象 | `{}` | 热更 | 各插件配置，由管理台「插件」页维护 |
| `plugin_access` | 对象 | `{}` | 热更 | 各插件访问控制，由管理台「插件」页维护 |

`plugin_settings` 与 `plugin_access` 的结构由各插件自己的 `Config` 字段定义，见 [storage.md](storage.md)。

## protocol：两种接入方式

| 模式 | 链路 | 要求 |
|---|---|---|
| `webhook` | 平台把事件 POST 到你的 `/webhook`，框架校验签名后分发 | 需要平台能访问到你的服务（公网地址或反代） |
| `websocket` | 框架主动连平台网关，长连接推送 | 需要 `intents` 声明订阅范围；不需要公网入口 |

`gateway_url` 用于 `websocket` 模式下接第三方中转站：填 `wss://...` 走中转，留空直连官方网关。填了但协议是 `webhook` 时框架会打一条警告并忽略该项。

## intents：websocket 订阅哪些事件

共 17 项可选：

| 事件 | 说明 |
|---|---|
| `GROUP_AT_MESSAGE_CREATE` | 群里 @ 机器人 |
| `GROUP_MESSAGE_CREATE` | 群内全部消息 |
| `C2C_MESSAGE_CREATE` | 私聊消息 |
| `INTERACTION_CREATE` | 互动事件（按钮回调等） |
| `GROUP_JOIN_REQUEST` | 入群申请 |
| `GROUP_MEMBER_ADD` | 成员入群 |
| `GROUP_MEMBER_REMOVE` | 成员退群 |
| `GROUP_ADD_ROBOT` | 机器人被加入群 |
| `GROUP_DEL_ROBOT` | 机器人被移出群 |
| `GROUP_MSG_RECEIVE` | 群消息接收开启 |
| `GROUP_MSG_REJECT` | 群消息接收关闭 |
| `C2C_MSG_RECEIVE` | 私聊消息接收开启 |
| `C2C_MSG_REJECT` | 私聊消息接收关闭 |
| `FRIEND_ADD` | 好友添加 |
| `FRIEND_DEL` | 好友删除 |
| `MESSAGE_AUDIT_PASS` | 消息审核通过 |
| `MESSAGE_AUDIT_REJECT` | 消息审核驳回 |

两个默认行为，都容易踩：

- **省略 `intents`** → 使用 13 项常用清单（上表去掉四个「接收开关」与两个好友事件）。
- **写成空数组、或全是框架不认识的名字** → 退化为框架内置的默认订阅位掩码，而不是「什么都不订阅」。想精确控制就按名逐个列出。

改完 `intents` 需要重启，且只在 `websocket` 模式生效。事件对象与订阅写法见 [events.md](events.md)。

## 内存与性能

三项只在 `config.json` 里可改（管理台不提供入口），改完重启生效：

| 字段 | 行为 |
|---|---|
| `memory_limit_mb` | 设置 Go 运行时软内存上限（`GOMEMLIMIT`）。**不配**时框架按整机内存自动推导一个上限（约为整机的若干分之一）；配 `0` 表示不设，完全交给运行时 |
| `gc_percent` | 设置 `GOGC`。`0` 表示保持 Go 默认（通常为 100） |
| `pprof_addr` | 开启 `/debug/pprof`。以 `:` 开头会自动补成 `127.0.0.1:...`，即**默认只监听本机**，需要远程访问请用 SSH 隧道而不是暴露到公网 |

启动日志会打印实际生效的值，例如 `运行时内存参数: GOMEMLIMIT=...MiB`，据此确认有没有配错。

## 相关

- 图床 provider 的凭证不在本文件，而在同目录的 `assets.json`，见 [assets.md](assets.md)
- 插件自身的配置项、访问控制见 [storage.md](storage.md) 与 [commands.md](commands.md)
- 实例目录布局与接入方式见仓库根目录 README
