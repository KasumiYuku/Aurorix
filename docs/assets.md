# 图床与资源

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [插件管理台](webui.md) · [API 参考](api.md) · [发布](publishing.md)

QQ 平台的富媒体消息只认公网 URL。图床聚合器（`lib/assets`）把**本地图片、内存字节、base64、data URL** 转成公网直链，并把已经是公网的链接按白名单直通。它在发送消息时由框架自动调用，多数插件不需要直接碰它。

## 什么时候会走图床

| 输入 | 处理 |
|---|---|
| 本地路径、`file://`、base64、data URL | 上传图床，换成直链 |
| 指向 `localhost` / `127.0.0.1` / `::1` / `10.*` / `172.*` / `192.168.*` 的 `http(s)` | 视为内网，同样上传图床 |
| 其他公网 `http(s)` | 原样使用，不上传 |
| 命中 `whitelist` 前缀 | 原样使用（最先判定，内网地址也直通） |

两个自动触发点：

- **独立图片消息**：`ctx.Image(本地路径)` 在发送前过图床。
- **Markdown 内嵌图**：内容里的 `![alt](本地路径)` 发送前被替换成直链；宽高已知时补成 `![alt #Wpx #Hpx](直链)`，让客户端提前占位。**替换只发生在发送那一刻**，模板文件与日志里看到的仍是原路径。

公网图在部分客户端需要登录态才能渲染，所以"已经是公网 URL 就直通"是刻意为之——不重复搬运，也不擅自替换别人的图。

## 上传链：优先级与回退

链路按下面顺序拼装，逐个尝试，**失败自动切下一个**；全部失败时发送返回 `all providers failed: ...`。

1. `assets.json` 中未显式关闭的 provider，按 `priority` 从大到小排序（**同级保持书写顺序**）
2. 追加「默认启用但你没配置」的 provider——目前只有官方的 `qqbot`，所以它天然是最后一环兜底

两处容易踩：

- **必填字段漏填的 provider 会被静默跳过**，不报错、不打断启动，表现为「配了却没生效」。以启动日志里的 `provider 数量` 为准。
- **上传后探测**：默认对返回的 URL 发一次 `HEAD`（8 秒超时，最多跟 5 次重定向）。`Content-Type` 不是 `image/*` 视为失败，继续切下一个；`HEAD` 不可达**不算失败**，原样采用并记警告。用 `probe: false` 关闭，或选一个默认跳过探测的 provider。

## assets.json

实例目录下的独立配置文件（0600，已在 `.gitignore` 中）。文件缺失或格式错误只记警告，按空配置继续启动。

```json
{
  "providers": [
    {
      "name": "sgame",
      "enabled": true,
      "priority": 300,
      "probe": true,
      "config": { "secretId": "AKID...", "secretKey": "..." }
    }
  ],
  "whitelist": ["https://cdn.example.com/img/"]
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `providers[].name` | 是 | provider 名，见下表 |
| `providers[].enabled` | 否 | 只有显式 `false` 才跳过；省略或 `true` 都参与 |
| `providers[].priority` | 否 | 越大越优先；省略为 0，同级按书写顺序 |
| `providers[].probe` | 否 | `false` 跳过上传后探测 |
| `providers[].config` | 按 provider | 各自必填项见下表 |
| `whitelist` | 否 | 前缀数组，命中即直通，不做任何上传 |

**管理台「图床」页**编辑的就是这份配置：provider 启停、优先级、字段值、白名单，保存即热更（重建聚合器并重新注入，无需重启）。

## 内置 Provider

| 名称 | 拿谁当图床 | 必填配置 | 备注 |
|---|---|---|---|
| `qqbot` | 平台官方富媒体存储 | 无 | **默认启用**、无需配置；产物只在**同一会话**内有效（约 24 小时），别存库长期用 |
| `sgame` | 腾讯云 COS | `secretId` `secretKey` | 可选 `host` `bucketUrl` |
| `cnb` | CNB 仓库 | `token` `defaultRepo` | 可选 `baseUrl` `autodelete`；跳过探测 |
| `kurobbs` | 库街区 | `token` | 跳过探测 |
| `mihoyo` | 米游社 | `cookie` | 需含 `stuid` / `stoken` / `ltoken` |
| `bilibili` | B 站 | `sessdata` `biliJct` | |
| `qzone` | QQ 空间 | `cookie` `pSkey` `skey` `pUin` | |
| `qqstream` | 腾讯视频 | `cookie` `skey` `pSkey` `uin` | |
| `qqgame` | QQ 游戏 | 无 | 可选 `opensopUid` |
| `announce` | 群公告 | `cookie` | |
| `homework` | 群作业 | `cookie` | |
| `chinaunicom` | 联通 | 无 | 可选 `appVersion` `appChannel` |
| `chatglm` | 智谱 | 无 | 默认关闭 |

> 除 `qqbot` 与自建 COS 外，其余都是"借别人的上传接口"，随时可能失效。建议至少配两个不同来源，并让 `qqbot` 留在链尾兜底。

## 自定义 Provider（插件扩展点）

注册表对插件开放。在 `init()` 里注册，之后就能在 `assets.json` 与管理台里选用：

```go
package myplugin

import (
	"context"
	"fmt"

	"github.com/KasumiYuku/Aurorix/lib/assets"
)

func init() {
	assets.Register("mine", func(cl *assets.Client, cfg map[string]any) (assets.ImageProvider, error) {
		token, _ := cfg["token"].(string)
		if token == "" {
			return nil, fmt.Errorf("mine: token 必填")
		}
		return &mine{cl: cl, token: token}, nil
	}, []assets.ConfigField{
		{Key: "token", Label: "令牌", Type: "password", Required: true},
	})
}

type mine struct {
	cl    *assets.Client
	token string
}

func (m *mine) Name() string { return "mine" }

func (m *mine) Upload(ctx context.Context, in assets.ProviderInput) (string, error) {
	// in.Buffer 是图片字节；in.Filename / in.MimeType 是线索
	// in.GroupID / in.UserID 是本次发送的会话目标，会话绑定的图床需要它
	return uploadToMine(ctx, m.cl, m.token, in.Buffer)
}
```

| 注册函数 | 语义 |
|---|---|
| `assets.Register` | 必须显式配置才启用，上传后探测 |
| `assets.RegisterNoProbe` | 必须显式配置才启用，跳过探测 |
| `assets.RegisterDefault` | 未配置也参与（排在配置项之后），跳过探测 |

可选实现 `SessionScoped() bool` 并返回 `true`，向框架声明"我签出的直链只与签发会话绑定"，方便调用方判断能否续用。schema 里的 `ConfigField` 会直接渲染成管理台图床页的输入项；`assets.Names()` 与 `assets.ProviderSchema(name)` 可用于自检注册结果。

## 观测与排障

以 `assets` 为来源前缀的日志：

| 日志 | 含义 |
|---|---|
| `图床转换成功: provider=..., 上传=..., 探测=..., 合计=..., 返回 URL=...` | 成功。`探测` 一栏会写明 `HEAD 200, image/png`、`HEAD 不可达` 或 `已按配置跳过探测`；最终 URL 与原始不同时会标出跟随后地址 |
| `图床转换失败: provider=...` | 该 provider 失败，链路继续往下走 |
| `图床 Content-Type 不匹配, 视为失败` | 直链不是图片，视为失败继续切 |
| `最终 URL HEAD 不可达, 原样使用` | 探测不可达但不影响使用 |
| `已启用图床聚合，provider 数量: N` | 启动时打印；为 0 或缺失说明图床整体没启用 |

管理台概览页可看已启用 provider 数量与本次进程累计上传数。

排查顺序：

1. 启动日志的 `provider 数量` 是不是 0——漏填必填项会静默出局
2. `priority` 是否把想要的 provider 排到了后面，导致还没轮到就已被前面的兜底接管
3. 目标平台是否拒收该直链——看 `图床转换失败` 里的 err 详情

## 相关

- 消息构造与发送（`ctx.Image` / Markdown 内嵌图）见 [message.md](message.md)
- 管理台与实例文件布局见仓库根目录 README
