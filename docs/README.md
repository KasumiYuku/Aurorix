# 开发文档

Polarix 插件开发从入门到发布。文档按主题分层，随时跳转。

## 架构总览

```
            ┌─────────────────────────────────────────────┐
            │                Polarix 框架                   │
            │                                             │
  消息进来    │  lib/plugin   指令注册 / 匹配 / 权限 / 参数    │
 ──────────▶ │  lib/context  消息上下文与发送器              │
   QQ 开放平台 │  lib/schedule 定时任务调度                    │
              │  lib/storage  SQLite 存储                   │
              │  lib/bot      运行时外壳 (入口调用 Run)       │
            └─────────────────────────────────────────────┘
                    ▲                     │
                    │ init() 注册          │ 空导入启用
                    │                     ▼
                你的插件包            main.go 接入清单
```

插件是一个普通 Go 包：包内 `init()` 调用 `plugin.Register(...)` 完成注册，入口 `main.go` 空导入启用。匹配、权限、参数解析、配置热更、访问控制全部由框架承担——插件只声明指令与处理逻辑。

**生命周期**

```
包 init() → plugin.Register(...) → 框架注册指令/配置/权限
        → 启动时加载插件配置与访问控制
        → 消息命中 → 权限校验 → 参数解析 → Handle(ctx)
        → 管理台操作 → ApplyConfig 热更 / 启停 / 访问控制
```

## 开发环境

**不需要单独的 SDK。** 框架就是 SDK：实例的 `go.mod` 通过 `replace` 引用本地框架，IDE 直接解析 `lib/` 全部公开 API——自动补全、跳转定义、类型检查开箱即用。

| 工具 | 说明 |
|---|---|
| GoLand / VS Code | 安装官方 Go 插件（gopls），打开实例目录即可 |
| 语法补全 | `ctx.` 后即出全部方法（Text/Markdown/Image...），`plugin.` 出 Register |
| 类型检查 | 写错签名立即报错，无需编译 |
| 参考实现 | 框架 `plugins/` 下 4 个官方插件源码是最佳活文档 |

建议把框架目录加入 IDE 的同一工作区（GoLand 的 Multi-root / VS Code 的 multi-root workspace），跨工程跳转更顺。

## 第一个插件

在实例目录执行：

```bash
plrx new-plugin hello     # 生成 plugins/hello/hello.go
plrx add mybot/plugins/hello   # 写入 main.go 空导入
```

生成的骨架：

```go
package hello

import (
	"Plrx/lib/constant"
	"Plrx/lib/context"
	"Plrx/lib/plugin"
)

func init() {
	plugin.Register(&plugin.Plugin{
		Id:   "hello",
		Name: "hello",
		Commands: []*plugin.Command{
			{
				Prefix:   "hello",
				Role:     constant.RoleMember,
				Describe: "hello",
				Handle:   cmd,
			},
		},
	})
}

func cmd(ctx *context.MessageContext) error {
	return ctx.Text("插件运行中").Send()
}
```

编辑 `plugins/hello/hello.go` 写你的逻辑，然后：

```bash
plrx run    # 重新编译并启动
```

群里发送 `/hello` 验证。改代码 → `plrx run`，循环迭代。

## 文档导航

| 文档 | 主题 |
|---|---|
| [commands.md](commands.md) | 指令开发：Command 全字段、前缀与匹配、参数解析、子指令、权限角色 |
| [message.md](message.md) | 消息开发：全部构造器、组合消息、Markdown 模板、按钮键盘、图片与媒体 |
| [schedule.md](schedule.md) | 定时任务：Cron / 间隔、预设目标、任务管理 |
| [storage.md](storage.md) | 配置热更与数据存储：ConfigField、Validate/Apply、存储命名空间 |
| [events.md](events.md) | 事件与能力：群事件回调、按钮交互回执、HTTP 客户端、图床 Provider |
| [push.md](push.md) | 主动推送：HTTP 端点完整协议、鉴权、状态码、管理指令 |
| [api.md](api.md) | API 全景参考：BotAPI 门面、流式消息、HTTP 客户端、日志、常量 |
| [publishing.md](publishing.md) | 完整开发流程、发布插件、生态接入（本地文件夹 / 网络模块） |