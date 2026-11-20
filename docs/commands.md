# 指令开发

> 文档导航：[开发文档首页](README.md) · [指令](commands.md) · [消息](message.md) · [事件](events.md) · [定时任务](schedule.md) · [存储](storage.md) · [推送](push.md) · [图床](assets.md) · [配置](configuration.md) · [API 参考](api.md) · [发布](publishing.md)

## Command

```go
&plugin.Command{
	Prefix:   "hello",              // 规范指令名, 不带前缀符号
	Aliases:  []string{"打招呼"},     // 别名, 中文亦可
	Role:     constant.RoleMember,  // 最低权限
	DisablePrivate: false,          // 禁止私聊
	NoFooter: false,                // 为 true 时这条指令的回复不带底部按钮
	Describe: "打招呼",
	Handle:   hello,                // func(*context.MessageContext) error
}
```

| 字段 | 说明 |
|---|---|
| `Prefix` | 规范指令名。注册时自动剥离 `/` `#` `!` 前缀符号 |
| `Aliases` | 别名，与按钮规范化共用 |
| `Role` | 最低权限，见[权限角色](#权限角色) |
| `DisablePrivate` | 为 true 时禁止私聊触发 |
| `NoFooter` | 为 true 时这条指令（及其子指令）的回复不追加框架级底部按钮；只想让单条消息不带就写 `ctx.Msg().NoFooter()` |
| `Describe` | 指令描述（管理台 / 用法文案） |
| `Handle` | 处理函数 `func(*context.MessageContext) error` |
| `PermissionDenied` | 权限不足时的处理，默认回复"无权限" |
| `HandleError` | `Handle` 返回错误后调用 |
| `SubCommand` | 子指令树，可多层嵌套 |
| `SubCommandFallback` | 子指令未匹配时的回退 |
| `Args` | 参数结构体（kong tag），`nil` 时剩余文本整体给 `ctx.Parsed` |

## 前缀系统

前缀符号（`/` `#` 与无前缀 `""`）在配置 `prefixes` 中定义，默认全开。前缀不属于插件。

**匹配三态**：

| 形态 | 示例 | 说明 |
|---|---|---|
| 精确 | `/hello` | 完整命中 |
| 独立成词 | `/echo hi` | 符号与参数空格分隔 |
| 粘合 | `/echoilove you` | 自动拆为 `echo` + 参数，仅对带前缀符号的词元尝试，口语词不误伤 |

## 子指令

```go
{
	Prefix: "db",
	SubCommand: []*plugin.Command{
		{Prefix: "clean", Handle: cleanDB},
		{Prefix: "backup", Handle: backupDB},
	},
}
```

命中 `db` 后按下一个词继续匹配，可多层嵌套。访问控制为每个子指令路径生成独立规则（如 `db clean`）。

## 参数解析

不用手写字符串解析。给指令声明一个结构体，框架自动帮你拆词、转类型、校验——拆错了还会自动把"用法说明"回复给用户。从最简单的开始，一步步加能力。

### 第一步：一个位置参数

"位置参数"= 直接跟在指令后面的值。声明 `kong:"arg"`：

```go
type args struct {
	UID string `kong:"arg,name='uid',help='要绑定的 UID'"`
}

func bind(ctx *context.MessageContext) error {
	args := ctx.Parsed.(*args)   // 走到这里说明解析成功
	return ctx.Text("已绑定 " + args.UID).Send()
}
```

```text
/bind U123456     → args.UID = "U123456"
/bind            → 自动回复用法: 缺 uid
```

### 第二步：加一个命名参数

"命名参数"= 用 `--名字 值` 传。带默认值，用户不传也不报错：

```go
type args struct {
	UID   string `kong:"arg,name='uid',help='要绑定的 UID'"`
	Note  string `kong:"name='note',default='无备注',help='备注'"`
}
```

```text
/bind U123456 --note 朋友  → args.Note = "朋友"
/bind U123456              → args.Note = "无备注" (默认值)
```

### 第三步：加一个开关

"开关"= 传了就是 true，不传就是 false：

```go
type args struct {
	UID   string `kong:"arg,name='uid',help='要绑定的 UID'"`
	Silent bool  `kong:"flag,name='silent',help='静默绑定, 不广播'"`
}
```

```text
/bind U123456 --silent  → args.Silent = true
/bind U123456           → args.Silent = false
```

### 第四步：限制取值范围

`enum` 限制可选值，超出自动报错并回用法；`sep` 把逗号分隔的一串转成切片：

```go
type args struct {
	UID   string   `kong:"arg,name='uid',help='要绑定的 UID'"`
	Mode  string   `kong:"name='mode',enum='fast,full',default='fast',help='模式: fast/full'"`
	Tags  []string `kong:"name='tags',sep=',',help='标签, 逗号分隔'"`
}
```

```text
/bind U123456 --mode full --tags a,b,c  → Mode="full", Tags=["a","b","c"]
/bind U123456 --mode slow               → 自动回复用法 (slow 不在枚举内)
```

### 完整 tag 速查（进阶）

| Tag | 作用 | 示例 |
|---|---|---|
| `arg` | 位置参数，按声明顺序取值，可多个 | `kong:"arg"` |
| `name='x'` | 命名参数名（`--x` 传值） | `kong:"name='limit'"` |
| `flag` | 布尔开关（`--silent`），`negatable` 支持 `--no-silent` | `kong:"flag"` |
| `required` / `optional` | 必填 / 可选（默认可选） | `kong:"required"` |
| `default='x'` | 缺省值 | `kong:"default='20'"` |
| `help='...'` | 用法文案（出现在自动回复里） | `kong:"help='条数上限'"` |
| `short='x'` | 单字母短标志（`-l`） | `kong:"short='l'"` |
| `enum='a,b'` | 枚举校验 | `kong:"enum='fast,full'"` |
| `sep=','` | 切片分隔符 | `kong:"sep=','"` |

支持类型：`string` / `int` / `float64` / `bool` / `time.Duration` / `[]string`（配 `sep`）/ 实现 `encoding.TextUnmarshaler` 的自定义类型。

### 不要参数时

`Args` 留 `nil`，`ctx.Parsed` 就是指令后的原始字符串。这个通道不会走 kong 参数解析，适合复读、公告、Markdown 草稿这类要保留换行、`#`、`>`、反引号等特殊符号的指令：

```go
func echo(ctx *context.MessageContext) error {
	text, _ := ctx.Parsed.(string)   // /echo 之后的所有内容
	return ctx.Text(text).Send()
}
```

### 参考实现

- `plugins/bind`：`kong:"arg,name='uid',help='要绑定的 UID'"`
- 框架内置推送服务（`lib/push`）：`kong:"arg,name='key',help='推送密钥'"`

## 权限角色

| 常量 | 含义 |
|---|---|
| `constant.RoleOwner` | 机器人所有者 |
| `constant.RoleAdmin` | 管理员（含 owner） |
| `constant.RoleMember` | 普通成员（默认） |

`Role` 只是插件声明的**最低要求**，最终是否放行由管理台的访问控制规则决定（`off` 全放行 / `whitelist` 名单 / `blacklist` 禁用，支持插件级与指令级覆盖）。

维护开关分三层，**只能加拦、不能解拦**：Bot 全局维护（连管理员都拦，只能在控制台关）、插件整体维护、指令级维护（在面板「指令覆盖」里逐条开）。后两层对管理员与主人豁免；指令级文案留空时先回落到插件级文案，再回落到框架默认话术。

`whitelist` / `blacklist` 与「身份」档不只管指令：**消息监听型插件**（自己不注册指令，只订阅消息，比如链接解析）一律按**默认规则**判，名单外的群/人不会触发它。想做到"只在这几个群解析链接"，就把默认规则设成白名单，把这几个群的 openid 填进「群」。

名单拦下之后**默认静默**——一个字都不回，对方以为机器人没收到。想让它说话就在管理台逐层打开：插件级「名单拦截」是总闸，每条指令自己还能单独表态；指令没表态就跟随插件级，拨回与插件级一致的位置即恢复跟随。文案留空时先回落到插件级文案，再回落到框架默认话术「你没有权限使用该指令」，超 300 字按字符截断。语义随模式走：`whitelist` 是「白名单外」，`blacklist` 是「黑名单内」。

这套回话和维护开关**不共用字段**：维护说的是「现在别用」，名单说的是「你不该用」。两者同时命中时按判定顺序，维护在前，名单在后，谁先拦下就用谁的文案。

再往上游还有一层**封禁名单**（管理台「封禁」页，或插件直接调 `lib/ban`）：指令命中之后、权限判定之前就拦下，按钮点击同样拒，命中即按记录里的话术回一句并在 15 秒内不重复回。这层不区分管理员与主人。语义也不同：访问控制是「这个人在这个群不能用这条指令」，封禁是「这个账号不该再跟机器人说话」。
