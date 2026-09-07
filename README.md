# Aurorix

![Go](https://img.shields.io/badge/Go-1.26-blue)
![License](https://img.shields.io/badge/License-MIT-green)

> 基于 Go 语言构建的现代化 QQ 开放平台机器人框架

<p align="center">
  <img alt="Aurorix Screenshot" src="https://github.com/user-attachments/assets/9c0d1ada-08e4-4a4d-9ada-d5ab42c0d93a" width="100%" />
</p>

> [!NOTE]
> Aurorix 深度重构并扩展自优秀的开源项目 [Polarix](https://github.com/YearnstudioYangyi/Polarix)。我们在完整保留其核心架构与设计哲学的基础上，进行了大量功能延伸
> 
> 本项目严格遵循 MIT 协议，保留上游全部版权声明与提交历史，在此向 Polarix 的原作者及全体贡献者致以崇高敬意。若您寻求基础稳固、久经考验且官方活跃维护的原生体验，我们强烈推荐您优先关注并使用原版 [Polarix](https://github.com/YearnstudioYangyi/Polarix)

## 快速开始

**环境要求**
- [Go](https://go.dev/dl/) >= 1.26
- [Git](https://git-scm.com/downloads)

Aurorix 采用**框架与实例分离**的设计模式。实例的配置、数据与插件独立维护，确保框架源码纯净，支持无缝升级。初始化后的目录结构如下：

```text
workspace/
├── Aurorix/      # 框架源码
└── mybot/        # 机器人实例 (由 aurx CLI 生成)
```

### 安装与运行

```bash
# 1. 获取框架源码
git clone https://github.com/KasumiYuku/Aurorix.git
cd Aurorix

# 2. 安装脚手架工具 aurx
# Windows 环境请替换为: go install ./tools/aurx
make install

# 3. 创建机器人实例
cd ..
aurx new mybot --framework Aurorix

# 4. 接入官方示例插件（可选）
cd mybot
aurx add github.com/KasumiYuku/Aurorix/plugins/bind
```

### 配置凭证

进入 `mybot` 目录，编辑 `config.json` 文件，填入你的机器人凭证：
- `appid` / `secret`：平台提供的 API 凭证
- `admin_password`：自定义的管理台密码

完成配置后，启动实例：

```bash
aurx run
```

> **验证状态**：启动成功后，终端将输出 `WebSocket 网关已启动`，并可通过 `http://127.0.0.1:8080/admin` 访问控制台。如出现持续重连或 Error 日志，请检查 `config.json` 凭证是否正确。

---

启动后：

- 机器人在线。示例插件已就绪：群里 @ 机器人发送 `/uptime` 查看服务状态
- 管理台在 `http://127.0.0.1:8080/admin`（端口取配置 `port`），登录密码 `admin_password`

实例结构：

```
mybot/
├── main.go              插件接入清单（空导入）
├── config.json          凭证与运行配置
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
| **官方示例** | `aurx add github.com/KasumiYuku/Aurorix/plugins/bind`（`echo` / `imagegen` / `uptime` 同理） |
| **网络第三方** | `aurx add github.com/某作者/某插件`（自动拉取依赖） |
| **本地插件文件夹** | 放入实例 `plugins/` 目录 → `aurx add` 接入 |

本地插件导入：

```bash
# 把已有的插件文件夹放进实例
cp -r ~/my-plugin mybot/plugins/my-plugin
cd mybot
aurx add mybot/plugins/my-plugin   # 写入 main.go 接入清单
aurx run                           # 编译并启动
```

### 创建自己的插件

```bash
cd mybot
aurx new-plugin hello              # 生成插件骨架
aurx add mybot/plugins/hello       # 接入
vim plugins/hello/hello.go         # 写你的指令逻辑
aurx run                           # 重新编译并运行
```

生成后目录长这样：

```
mybot/plugins/hello/
├── hello.go              插件主体：注册 + 指令逻辑
└── templates/
    └── markdown/
        └── Hello.md      模板文件（随插件编译进二进制，用户无需手动放置）
```

生成的 `hello.go` 自带模板装载三件套：`//go:embed templates/markdown/*.md` 声明、`templateFS` 变量、`TemplateFS: templateFS` 挂载。在指令里用 `ctx.MarkdownTemplate("Hello", &templates.Args{...})` 就能按模板发消息（用法细节见[开发文档](docs/README.md)）。

群里发送 `/hello` 验证。改代码 → `aurx run`，循环迭代。

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
| `GROUP_MSG_RECEIVE` / `GROUP_MSG_REJECT` | 群消息接收开启 / 关闭 |
| `C2C_MSG_RECEIVE` / `C2C_MSG_REJECT` | 私聊消息接收开启 / 关闭 |

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
<summary><b>终端提示 <code>aurx: command not found</code>？</b></summary>

<br>
这通常是因为 Go 的二进制目录未加入系统环境变量。请根据你的操作系统执行以下命令，并<b>重启终端</b>：

**Linux / macOS**
```bash
# Bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc && source ~/.bashrc

# Zsh
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc && source ~/.zshrc

# Fish
fish_add_path (go env GOPATH)/bin
```

**Windows (PowerShell)**
```powershell
$path = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$path;$(go env GOPATH)\bin", "User")
```
</details>

<details>
<summary>重复启动会端口冲突吗？</summary>

不会。新实例自动终止占用同一端口的旧实例并接管。被 systemd 守护时设置 `AURORIX_SUPERVISED=1`，面板重启直接退出交由守护拉起。

</details>

<details>
<summary>修改了前端（web/）？</summary>

```bash
cd web && pnpm build   # 产物嵌入 lib/admin/dist，重新编译实例即生效
```

</details>

<details>
<summary>管理台安全吗？</summary>

登录使用 `admin_password`，会话为 HttpOnly Cookie（可保留 30 天）。密码留空仅本机可访问。登录失败会触发按来源 IP 的临时限流（5 分钟窗口），防密码爆破。

</details>

## 开源协议 (License)

本项目采用 [MIT LICENSE](LICENSE) 开源

* 核心架构与原版历史代码版权归原 [Polarix](https://github.com/YearnstudioYangyi/Polarix) 作者及贡献者所有
* 后续新增特性与重构代码版权归 Aurorix 维护者所有
* 您可以自由地使用、修改和分发本项目，但请务必保留原作者及本项目的版权声明
