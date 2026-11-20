# 配置热更与数据存储

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [API 参考](api.md) · [发布](publishing.md)

## 插件配置（热更）

插件声明配置表单，管理台据此渲染，保存即生效、无需重启：

```go
plugin.Register(&plugin.Plugin{
	Id: "myplugin",
	Config: []plugin.ConfigField{
		{Key: "api_key", Label: "API 密钥", Type: "password", Required: true},
		{Key: "timeout", Label: "超时秒数", Type: "number", Placeholder: "30"},
	},
	ValidateConfig: func(v map[string]any) error {
		if v["timeout"].(float64) <= 0 {
			return errors.New("timeout 必须大于 0")
		}
		return nil
	},
	ApplyConfig: func(v map[string]any) error {
		// 应用新配置, 无需重启
		return nil
	},
})
```

| ConfigField | 说明 |
|---|---|
| `Key` | 配置项标识 |
| `Label` | 表单展示名 |
| `Description` | 帮助说明 |
| `Type` | 表单类型：`string` / `number` / `password` 等 |
| `Placeholder` | 占位提示 |
| `Required` | 必填 |

流程：管理台提交 → `ValidateConfig` 校验（返回 error 拒绝）→ 保存 → `ApplyConfig` 生效。配置存放在 `config.json` 的 `plugin_settings` 中。

## 数据存储

PostgreSQL 持久化，按命名空间隔离。包路径 `github.com/KasumiYuku/Aurorix/lib/storage`，连接串由配置 `database` 指定（`postgres://用户:密码@主机:端口/库名?sslmode=disable`）。

### 命名空间

```go
ctx.PluginStorage    // 当前插件
ctx.CommandStorage   // 当前指令
ctx.UserStorage      // 当前用户
ctx.TargetStorage    // 当前会话（群 / 单聊 / 子频道 / 频道私信）
ctx.GlobalStorage    // 全局

storage.Plugin("myplugin")   // 任意位置按插件取
storage.Command("myplugin", "cmd")
storage.User("用户OpenID")
storage.Scene(constant.SceneGroup, "群OpenID")
storage.Scene(constant.SceneChannel, "子频道ID")
storage.Global()
```

### Store 方法

| 方法 | 说明 |
|---|---|
| `Set(key, value)` | 写入，任意 JSON 序列化对象 |
| `Get(key, &target)` | 读取到目标对象，返回 `(found, err)` |
| `Has(key)` | 存在性检查 |
| `Delete(key)` | 删除 |
| `Clear()` | 清空当前命名空间 |

零保留键，所有 key 自由使用。底层 `jackc/pgx/v5`，连接池默认 16 条。

### 示例

```go
func remember(ctx *context.MessageContext) error {
	var count int
	ctx.PluginStorage.Get("count", &count)
	count++
	ctx.PluginStorage.Set("count", count)
	return ctx.Text(fmt.Sprintf("第 %d 次", count)).Send()
}
```

## 管理台统计的落库语义

会话 / 每日计数在内存累加，后台协程每 5 秒把**变动过的那几行**按增量写库一次（`msg_count = msg_count + ?`，不是整表重写）。

- 消息热路径不碰数据库：单条消息的开销是内存自增加一次脏标记
- 落库失败时增量原样退回内存，下一轮重试，不丢也不重
- 正常退出时 `stats.Stop()` 同步落完最后一批；进程被强杀最多丢最后一个 5 秒窗口的计数

### 好友数是怎么算的

平台**没有**好友列表接口，所以好友数只能靠事件与消息一点点补：

- 收到 `FRIEND_ADD` → 记一条（`friend = true`），没说过话的好友也算进来
- 收到 `FRIEND_DEL` → 只摘好友标记，历史消息数留着（活跃会话该是多少还是多少）
- **单聊来消息** → 落库时顺手把 `friend` 置真：漏掉加好友事件的人，第一次说话就补上了

判断"这个人是不是已经统计过"不需要额外查询：单聊消息走的是会话计数那条路（内存 `map` 自增，5 秒批量 upsert），
数据库那边 `ON CONFLICT (scene, id)` 天然幂等。所以「来一条消息就补录」是零查询成本、也不拖慢业务。

口径上的区别：`friends` 是**当前好友位**上的人，`peers` 是**有过单聊往来**的人（含已删好友的历史），
管理台概览里的「好友」用前者，活跃会话排行里两类都会出现。
## 从 SQLite 迁移

旧版本把数据存在单个 `bot.db` 文件里，现在全部落在 PostgreSQL。迁移是一次性的，可以反复演练：

```bash
# 1. 建库建角色（按需改名字）
sudo -u postgres psql -c "CREATE ROLE aurorix LOGIN PASSWORD '换成你的密码'"
sudo -u postgres psql -c "CREATE DATABASE aurorix OWNER aurorix"

# 2. 先看它会建哪些表、哪些索引（不写库）
go run ./tools/migrate-to-pg -sqlite /旧部署/data/bot.db \
  -postgres "postgres://aurorix:密码@127.0.0.1:5432/aurorix?sslmode=disable" -dry-run

# 3. 真迁移：逐表搬完当场比对行数，再逐行比对抽样内容
go run ./tools/migrate-to-pg -sqlite /旧部署/data/bot.db \
  -postgres "postgres://aurorix:密码@127.0.0.1:5432/aurorix?sslmode=disable" -sample 500

# 4. 想全表核对（表小的时候）就把 -sample 给到足够大
go run ./tools/migrate-to-pg ... -sample 1000000
```

迁移器只读旧库（以 `mode=ro` 打开），不删不改。想再演练一遍就先 `DROP DATABASE aurorix` 重建，重复往同一张已有数据的表里写会直接主键冲突报错（宁可停下也不覆盖）。行数不一致或抽样内容对不上会直接退出并打印是哪张表哪一行哪一列。

切换顺序：

1. 停机器人（避免迁移过程中还有写入）
2. 跑迁移器
3. 改 `config.json` 的 `database` 指向新库
4. 启动，确认业务正常
5. **旧 `bot.db` 至少保留一周**，确认无问题再删

曲库 `phi.db` 不动：它是随插件分发的只读资产，继续用 SQLite。

## 测试怎么跑

单元测试直接连一个真实的 PostgreSQL，每个测试用独立 schema：

```bash
export AURORIX_TEST_DATABASE="postgres://postgres:postgres@127.0.0.1:5432/aurorix?sslmode=disable"
go test ./... -count=1
```

CI 里由 `postgres:18` service 提供这个库。`make test` 在没有该变量时会直接报错，不会静默跳过数据库测试。
