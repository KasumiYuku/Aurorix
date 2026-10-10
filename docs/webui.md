# 插件自带管理台

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [插件管理台](webui.md) · [API 参考](api.md) · [发布](publishing.md)

管理台是**可选能力**。插件不声明 `WebUI`，就只是一个纯指令插件：框架不会为它注册任何路由，框架管理台里也不会出现任何入口——小型插件零负担。

只有需要独立操作界面的插件（曲库管理、用户档案、批量运维……）才声明它。

## 声明

在 `plugin.Register` 的 `Plugin` 结构里加一个字段：

```go
//go:embed all:web/dist
var distFS embed.FS

func init() {
	plugin.Register(&plugin.Plugin{
		Id:   "phigros",
		Name: "Phigros",
		WebUI: &plugin.WebUI{
			SPA: distFS,                       // 前端产物根（含 index.html）
			API: func(mux *http.ServeMux) {    // 只注册 /api/ 前缀下的具体路径
				mux.HandleFunc("GET /api/overview", handleOverview)
			},
		},
		Commands: []*plugin.Command{ /* ... */ },
	})
}
```

| 字段 | 作用 | 可省略 |
|---|---|---|
| `SPA` | 前端产物 `fs.FS`，托管 `/<id>/` 下的单页应用 | 是 |
| `API` | 注册 `/<id>/api/...` 数据路由的回调 | 是 |

三种形态都合法：**只 `SPA`**（纯展示，数据走框架接口）、**只 `API`**（供脚本调用，框架管理台不显示入口）、**两者都有**（标准管理台）。

## 挂载语义

框架在 `admin.Register` 时按插件 Id 挂到同一个 mux，与 `/admin` 完全同构：

| 路径 | 鉴权 | 说明 |
|---|---|---|
| `/<id>/` | 壳对未鉴权开放 | 只返回前端资源，不含任何数据 |
| `/<id>/api/...` | 复用 `/admin` 会话（`requireAuth`） | 与框架共用一条登录态，无需自建登录 |

- **单一端口、单一登录**：插件不需要自己的账号体系，会话 Cookie `px_admin` 的 `Path=/`，所以 `/<id>/api/*` 收得到。
- **前端路由回退**：未命中的路径回退 `index.html`（支持深链如 `/phigros/songs/xxx`）；带静态资源扩展名却没命中则 404，不会把 HTML 当成 js/css 返回。
- **未知接口**：`/<id>/api/` 下未注册的路径返回 `404 {"error":"接口不存在"}`。
- 插件停用（访问控制里的「停用插件」）只影响指令与定时任务，管理台仍可访问，便于恢复配置。

## 框架管理台里的入口

框架会把声明了 `SPA` 的插件列进 `/admin`：

- 侧边栏「插件控制台」分组，一键直达 `/<id>/`；
- 插件页每张卡片与详情抽屉里的「打开管理台」。

入口数据来自 `GET /admin/api/plugins` 的 `console` 字段（未声明 `SPA` 时为空，前端据此不渲染入口）。因此插件不需要自己往框架页面里塞链接。

## 前端约定

- 复用设计系统 `@aurorix/webui`（令牌、组件、主题切换、SSE 与查询层），产物用 `go:embed` 打进插件二进制——**缺 `dist/index.html` 会编译失败**，仓库里保留占位文件。
- 前端路由 `basename` 用 `/<id>`，接口前缀 `/<id>/api`，与挂载语义对齐。
- 管理台头部放一个返回框架控制台的链接（`/admin/overview`），避免用户进去后出不来。

## 本地调试

```bash
cd <plugin>/web && pnpm build   # 产物落到 web/dist，被 go:embed 收集
```

改完重编实例即可；插件管理台与框架管理台同端口，浏览器直接访问 `http://127.0.0.1:<port>/<id>/`。
