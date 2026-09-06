# 插件开发流程与发布

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

## 完整开发流程

```
plrx new-plugin <名字>    1. 生成骨架
plrx add <模块路径>       2. 接入 main.go
编辑 plugins/<名字>/      3. 写指令逻辑 (IDE 自动补全, 见开发文档首页)
plrx run                 4. 编译并启动, 群内验证
改 → plrx run             5. 迭代: Go 是编译型语言, 每次改动重新编译
```

### 开发环境

- 不需要独立 SDK：框架即 SDK，`go.mod` replace 引用本地框架，IDE 直接解析全部 API（见[开发文档首页](README.md#开发环境)）
- 参考实现：框架 `plugins/` 下官方插件（bind / echo / imagegen / uptime）源码

## 插件的两种载体

| 载体 | 适用 | 接入 |
|---|---|---|
| 实例内 `plugins/` 目录 | 自己用 / 本地开发 | `plrx add mybot/plugins/名字` |
| 独立 Go module | 发布给他人 | `plrx add github.com/某作者/某插件` |

## 导入已有插件

### 本地插件文件夹

别人给的、或你以前写的插件目录，直接放进实例：

```bash
cp -r ~/my-plugin mybot/plugins/my-plugin
cd mybot
plrx add mybot/plugins/my-plugin   # 写入 main.go 空导入
plrx run                           # 编译进二进制并启动
```

要求：目录含合法的 Go 包（`package xxx` + `init()` 中 `plugin.Register`）。

### 网络模块

```bash
plrx add github.com/某作者/某插件   # 注入 import + go get 拉依赖
plrx run
```

## 发布插件

把插件做成独立 Go module 推 GitHub 并打 tag：

```bash
# 插件仓库
go mod init github.com/某作者/某插件
# 在包内 init() 中 plugin.Register(...)
git tag v0.1.0 && git push --tags
```

使用者即可 `plrx add github.com/某作者/某插件`。

**发布规范**：

- 插件 = 独立目录 + 独立 package，`Id` 全局唯一
- 只依赖框架公开 API（`lib/plugin` `lib/context` `lib/constant` `lib/schedule` `lib/storage`），不触碰框架内部实现
- 敏感配置（密钥等）用 `Config` 声明，不硬编码进源码
- 指令名小写英文为主，中文别名放 `Aliases`
- 对外请求走 `ctx.Request`，勿自建裸 `http.Client`

## 移除插件

```bash
rm -rf mybot/plugins/my-plugin     # 删除目录
# 同时从 main.go 删掉对应空导入行
plrx run                           # 重新编译
```

## 修改框架 / 前端 / 插件后

| 你改了 | 需要做什么 |
|---|---|
| 插件代码 | 重新编译实例：`plrx run` |
| 新增插件目录 | `plrx add` 或手动加空导入 |
| 框架代码（lib/） | replace 指向本地，改动即时可见，重编实例 |
| 管理台前端（框架 web/） | `cd web && pnpm build`，再重编实例 |
| 更新框架 | `git pull`，重编实例；官方插件有变按报错调整 |
