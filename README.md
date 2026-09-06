# PolarixPlus

![Go](https://img.shields.io/badge/Go-1.26-blue)
![License](https://img.shields.io/badge/License-MIT-green)

> 用 Go 编写的面向 QQ 开放平台的官方机器人框架。

<img width="1791" height="875" alt="image" src="https://github.com/user-attachments/assets/c64a8e2b-dbd4-4bd1-958e-335ad33bf4bc" />

> [!NOTE]
> 本项目是 [YearnstudioYangyi/Polarix](https://github.com/YearnstudioYangyi/Polarix/tree/dev) 的 fork 改版，在上游核心框架的基础上定制与扩展

## 特性

- **原生性能**：纯 Go 编译，单二进制分发，内存占用远低于解释型框架
- **插件即 Go 包**：官方插件随框架分发，第三方插件 `plrx add` 一行接入，本地插件目录放入即用
- **内置管理台**：Web UI 管理指令、插件、图床、定时任务、日志，支持热更新
- **双通道接入**：Webhook 回调或 WebSocket 长连接，无公网 IP 也能运行
- **低摩擦运维**：重复启动自动接管端口，进程内自重启，systemd 友好

## 快速开始

先看你要去哪——快速开始完成后，你的工作目录长这样：

```
你的工作目录/
├── PolarixPlus/    框架源码（git clone 得到，只读参考，别往里放个人文件）
└── mybot/          你的机器人实例（plrx new 创建，与框架平级）
```

实例独立于框架目录之外：它的配置、模板、数据库、插件全部自理，框架源码保持纯净可随时更新或删除重 clone。

```bash
# 1. 获取框架源码（含 plrx 工具与全部库）
git clone git@github.com:KasumiYuku/PolarixPlus.git
cd PolarixPlus

# 2. 安装脚手架 plrx（装入 Go 工具链目录）
make install
export PATH="$PATH:$(go env GOPATH)/bin"   # 永久生效见文末 FAQ

# 3. 回到上级目录创建实例。
#    cd .. 是为了让 mybot 与框架平级 —— 实例不嵌在框架源码里。
#    --framework 指向框架目录，实例靠它定位框架（go.mod replace 引用）。
cd ..
plrx new mybot --framework PolarixPlus

# 4. 接入官方示例插件（可选）
cd mybot
plrx add Plrx/plugins/bind

# 5. 配置凭证（config.json 位于实例目录，由 plrx new 自动生成）
vim config.json   # 填入 appid / secret / admin_password

# 6. 启动
plrx run
```

启动后：

- 机器人在线。示例插件已就绪：群里 @ 机器人发送 `/uptime` 查看服务状态
- 管理台在 `http://127.0.0.1:8080/admin`（端口取配置 `port`），登录密码 `admin_password`

实例结构：

```
mybot/
├── main.go              插件接入清单（空导入）
├── config.json          凭证与运行配置
├── templates/markdown/  消息模板（实例独有，可自由增改）
├── data/                数据库（data/bot.db）与运行时数据
└── plugins/             本地插件
```

## 管理台

| 页面 | 能力 |
|---|---|
| 概览 | 运行时长 / 内存 / goroutine / 消息计数 / 网关状态 / 最近日志 |
| 日志 | 级别与来源筛选、关键字搜索、SSE 实时推送、导出 |
| 插件 | 目录卡片、配置编辑（保存即热更）、启停、访问控制 |
| 图床 | Provider 启停 / 优先级 / 配置，白名单直通 |
| 定时任务 | 任务列表、暂停 / 恢复 |
| 设置 | 核心参数分组，即时生效项与需重启项分离 |

## 插件

插件是一个 Go 包：`init()` 中注册指令，接入后在构建时编译进二进制。

### 导入插件（三种来源）

| 来源 | 做法 |
|---|---|
| **官方示例** | `plrx add Plrx/plugins/bind`（`echo` / `imagegen` / `uptime` 同理） |
| **网络第三方** | `plrx add github.com/某作者/某插件`（自动拉取依赖） |
| **本地插件文件夹** | 放入实例 `plugins/` 目录 → `plrx add` 接入 |

本地插件导入：

```bash
# 把已有的插件文件夹放进实例
cp -r ~/my-plugin mybot/plugins/my-plugin
cd mybot
plrx add mybot/plugins/my-plugin   # 写入 main.go 接入清单
plrx run                           # 编译并启动
```

### 创建自己的插件

```bash
cd mybot
plrx new-plugin hello              # 生成插件骨架
plrx add mybot/plugins/hello       # 接入
vim plugins/hello/hello.go         # 写你的指令逻辑
plrx run                           # 重新编译并运行
```

完整的插件开发流程与 API 参考见[开发文档](docs/README.md)。

## 配置

`config.json` 是唯一配置文件（位于实例目录）。完整字段与可订阅事件：

<details>
<summary>config.json 全字段（点击展开）</summary>

```json
{
  "port": 8080,
  "appid": "你的机器人AppID",
  "secret": "你的机器人AppSecret",
  "proxy": "https://api.sgroup.qq.com",
  "protocol": "webhook",
  "intents": ["GROUP_AT_MESSAGE_CREATE", "INTERACTION_CREATE"],
  "database": "data/bot.db",
  "admin_password": "设置一个管理面板密码",
  "global_markdown": false,
  "markdown_verify_image": false,
  "retry_when": [11253, 630006],
  "upload_threshold": 3145728,
  "log_level": "info",
  "plugin_settings": {},
  "plugin_access": {}
}
```

| 字段 | 说明 |
|---|---|
| `port` | 服务与管理台端口 |
| `appid` / `secret` | 开放平台机器人凭证 |
| `proxy` | API 反代地址。服务器 IP 动态时，在固定 IP 设备反代 QQ API 并填其地址，绕过 IP 白名单 |
| `protocol` | `webhook`（平台推回调）或 `websocket`（长连接网关） |
| `intents` | websocket 模式订阅的事件名 |
| `database` | SQLite 数据库路径（实例默认 `data/bot.db`） |
| `admin_password` | 管理台密码；留空仅本机可访问 |
| `global_markdown` | 所有文字按 Markdown 渲染，图片/按钮内联 |
| `markdown_verify_image` | Markdown 图片转存失败时中断发送 |
| `retry_when` | 命中这些 QQ 业务错误码自动重试 |
| `upload_threshold` | 超过该字节数走分片上传（默认 3MB） |
| `log_level` | 控制台日志级别，可在线热更 |
| `plugin_settings` | 插件配置，面板修改即时生效 |
| `plugin_access` | 插件访问控制 |

**可订阅事件（intents）**

| 事件 | 说明 |
|---|---|
| `GROUP_AT_MESSAGE_CREATE` | 群里 @ 机器人 |
| `GROUP_MESSAGE_CREATE` | 群内全部消息 |
| `C2C_MESSAGE_CREATE` | 私聊消息 |
| `INTERACTION_CREATE` | 互动事件（按钮回调等） |
| `GROUP_JOIN_REQUEST` | 入群申请 |
| `GROUP_MEMBER_ADD` / `GROUP_MEMBER_REMOVE` | 成员入群 / 退群 |
| `MESSAGE_AUDIT_PASS` / `MESSAGE_AUDIT_REJECT` | 消息审核通过 / 驳回 |
| `GROUP_ADD_ROBOT` / `GROUP_DEL_ROBOT` | 机器人被添加 / 移除 |

</details>

### 接入方式

| 方式 | 适用场景 |
|---|---|
| **Webhook**（默认） | 有公网地址或反代，平台把事件推送到 `/webhook` |
| **WebSocket** | 无公网回调地址，框架主动连网关长连接，管理台与主动推送不受影响 |

图床配置独立存于实例 `assets.json`（不入版本库），管理台可视化编辑并热更新。上传按 `priority` 从高到低尝试，失败自动切换；`whitelist` 命中 URL 原样透传；密钥字段不回显。

## 开发文档

| 文档 | 内容 |
|---|---|
| [docs/](docs/README.md) | 文档首页：架构总览、开发环境、第一个插件、导航 |
| [docs/commands.md](docs/commands.md) | 指令：字段、前缀系统、参数解析、子指令、权限 |
| [docs/message.md](docs/message.md) | 消息：构造器、组合消息、Markdown 模板、按钮、图片 |
| [docs/schedule.md](docs/schedule.md) | 定时任务 |
| [docs/storage.md](docs/storage.md) | 配置热更与数据存储 |
| [docs/events.md](docs/events.md) | 群事件、按钮回调、HTTP 客户端 |
| [docs/push.md](docs/push.md) | 主动推送：HTTP 端点完整协议与使用 |
| [docs/api.md](docs/api.md) | API 全景：流式消息、BotAPI 门面、HTTP 客户端、日志、常量 |
| [docs/publishing.md](docs/publishing.md) | 插件开发流程、发布与生态接入 |

## 常见问题

<details>
<summary>plrx 装好了但提示命令找不到？</summary>

`make install` 把 plrx 装入 Go 工具链目录，需要它在 PATH 中：

```bash
# bash / zsh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc   # 或 ~/.zshrc
source ~/.bashrc

# fish
fish_add_path (go env GOPATH)/bin

# Windows (PowerShell)
$env:Path += ";$(go env GOPATH)\bin"   # 或写入系统环境变量
```

</details>

<details>
<summary>重复启动会端口冲突吗？</summary>

不会。新实例自动终止占用同一端口的旧实例并接管。被 systemd 守护时设置 `POLARIX_SUPERVISED=1`，面板重启直接退出交由守护拉起。

</details>

<details>
<summary>修改了前端（web/）？</summary>

```bash
cd web && pnpm build   # 产物嵌入 lib/admin/dist，重新编译实例即生效
```

</details>

<details>
<summary>管理台安全吗？</summary>

登录使用 `admin_password`，会话为 HttpOnly Cookie（可保留 30 天）。密码留空仅本机可访问。

</details>

## 许可证

MIT License。本项目为 [YearnstudioYangyi/Polarix](https://github.com/YearnstudioYangyi/Polarix/tree/dev) 的个人 fork 改版。
