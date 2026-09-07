# 定时任务

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [定时任务](schedule.md) · [存储](storage.md) · [事件](events.md) · [推送](push.md) · [API 参考](api.md) · [发布](publishing.md)

插件在 `init()` 中注册定时任务，框架进程内调度执行。包路径 `github.com/KasumiYuku/Aurorix/lib/schedule`。

## Job

```go
&schedule.Job{
	Id:       "daily-report",   // 任务唯一 ID, 同 ID 重复注册会覆盖旧任务
	PluginId: "myplugin",       // 所属插件 ID (插件停用时任务不再触发)
	Cron:     "0 9 * * *",      // 5 段 cron: 分 时 日 月 周
	Interval: time.Hour,        // 固定间隔; 与 Cron 同时设置时优先 Interval
	Immediate: true,            // Interval 任务启动时立即执行一次
	GroupId:  "",               // 预设群 OpenID (可选)
	UserId:   "",               // 预设用户 OpenID (可选)
	Target:   constant.GroupMessage, // 预设发送目标 (可选)
	Handle:   func(ctx *context.ScheduleContext) error { ... },
}
```

**预设目标**：设置 `GroupId` / `UserId` / `Target` 后，触发时自动写入 `ScheduleContext`，处理函数里 `ctx.Text(...).Send()` 直达预设会话。

**无预设目标**时主动推送：`ctx.Qapi.SendGroupMessage(payload, groupID)` / `ctx.Qapi.SendPrivateMessage(payload, userID)`。

## 示例

```go
func init() {
	schedule.Register(&schedule.Job{
		Id:       "daily-report",
		PluginId: "myplugin",
		Cron:     "0 9 * * *",   // 每天 09:00
		GroupId:  "群OpenID",
		Target:   constant.GroupMessage,
		Handle:   report,
	})
	schedule.Register(&schedule.Job{
		Id:        "heartbeat",
		PluginId:  "myplugin",
		Interval:  time.Hour,
		Immediate: true,
		Handle:    heartbeat,
	})
}

func report(ctx *context.ScheduleContext) error {
	return ctx.Text("每日报告已生成").Send()
}

func heartbeat(ctx *context.ScheduleContext) error {
	// 无预设目标, 主动推送
	return ctx.Qapi.SendPrivateMessage(payload, "用户OpenID")
}
```

## 任务管理

| 函数 | 说明 |
|---|---|
| `schedule.Register(job)` | 注册（init 中调用） |
| `schedule.Cancel(id)` | 取消 |
| `schedule.Pause(id)` / `schedule.Resume(id)` | 暂停 / 恢复 |
| `schedule.IsPaused(id)` | 查询暂停状态 |
| `schedule.Jobs()` | 全部任务快照（管理台展示） |

管理台「定时任务」页提供可视化的暂停 / 恢复。`ScheduleContext` 字段：`JobId` / `PluginId` / `FiredAt`。
