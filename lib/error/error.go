// Package errorx QQ 平台错误语义化: 错误码分类、重试决策与中文提示。
package errorx

import (
	"errors"
	"time"
)

// Kind 错误语义分类, 决定是否可重试与退避方式。
type Kind int

const (
	KindUnknown Kind = iota
	KindRateLimit
	KindAuth
	KindPermission
	KindNotFound
	KindAudit
	KindServer
	KindParam
	// KindPassiveExpired 被动回复窗口已失效（msg_id/event_id 过期，或次数超限）。
	//
	// 单独成一类而不是塞进 KindNotFound：那类说的是"这条消息不存在了"，
	// 而这里说的是"**你还能发消息，只是不能再回复那条了**" —— 后者的处置是
	// 换主动通道重发（见 lib/message 的三级降级），与 KindNotFound 完全不同。
	//
	// 实测现场（2026-10-14）：一次冷启动全量扫描 4m58s，结果出来时
	// `ERROR 消息已过期，不能回复`，图已经渲染好也传到了图床，就是发不出去。
	KindPassiveExpired
)

// QQError 业务错误。ResetAfter 仅 KindRateLimit 时有值。
type QQError struct {
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Kind       Kind
	ResetAfter time.Duration
}

func (e *QQError) Error() string {
	return Describe(e.Code) + " " + e.Message
}

// New 构造错误并自动分类。
func New(code int, message string) *QQError {
	return &QQError{Code: code, Message: message, Kind: Classify(code)}
}

var errorHints = map[int]string{
	100016: "secret 输入错误",
	10004:  "appid 输入错误",
	100007: "机器人被封禁或不存在",
	100017: "接口调用超过频率限制",

	11300: "仅私域机器人可用（link type check failed）",
	11700: "机器人已取消该能力",

	11253:  "按钮回调权限不足",
	630006: "header appid 获取失败",

	304023: "消息正在审核中（audit）",
	304004: "无权限使用该 ARK 模板",
	304036: "无 Markdown 模板权限",
	304061: "消息内容无效",
	304064: "订阅消息未授权",
	304080: "文件信息无效",
	304103: "消息 ID 已过期，不能回复",
	305007: "键盘样式参数错误",
	340069: "消息类型无效",

	22006:    "消息类型与内容不匹配",
	40034004: "富媒体信息转存失败，请重试",
	40034005: "回复消息 msg_id 已过期",
	40034006: "消息内容违规",
	40034008: "markdown 参数有空值",
	40034009: "markdown 参数有换行符",
	40034010: "模板参数不能含 markdown 语法",
	40034011: "无效的 markdown 内容",
	40034024: "msg_id 无效或越权",
	40034025: "event_id 无效（事件尚未落库）",
	40034026: "event_id 已过期",
	40034027: "该事件不支持被动回复",
	40034028: "消息内容包含平台不允许的链接",
	40034029: "内联键盘行/列超限",
	40034031: "消息已过期，不能回复",
	40034100: "主动消息超过频控限制",
	40034101: "机器人非群成员",
	40034105: "主动消息发送失败，无权限",
	40034106: "消息不支持该指令类型",
	40034108: "指令参数长度超限",
	40034109: "指令参数解析失败",
	40034124: "markdown 消息参数错误",
	40034127: "无 markdown 模板权限",
	40034128: "被动回复时间或次数超限",
	40054002: "机器人被禁言",
	40054003: "机器人不是群成员",
	40054005: "消息被去重（同一 msg_id + msg_seq 重复发送）",
	40054007: "消息长度超限",
	40054010: "不允许发送 URL",
	40054016: "机器人已下线",
	40064004: "已超出消息撤回时限",
	50055001: "消息发送异常，请稍后重试",
	50055006: "ARK 消息发送异常，请稍后重试",
}

// Describe 返回错误码的中文描述。
func Describe(code int) string {
	if hint, ok := errorHints[code]; ok {
		return hint
	}
	return "未知错误"
}

var semantic = map[int]Kind{
	100016: KindAuth,
	10004:  KindAuth,
	100007: KindAuth,

	100017:   KindRateLimit,
	40034100: KindRateLimit,

	//权限类：机器人不在群里、被禁言、没有主动消息权限、无 ARK/Markdown 模板权限。
	//后面几个是**主动降级失败**最常见的原因（见 lib/message 的三级降级收尾日志）：
	//主动群聊撞上这些码，就该退到私聊。
	11300:    KindPermission,
	11700:    KindPermission,
	304004:   KindPermission,
	304036:   KindPermission,
	304064:   KindPermission,
	40034127: KindPermission,
	//被动窗口失效：都能靠"改走主动通道"救回来，所以单独一类。
	//304103 / 40034005 / 40034031 是同一件事的三种上游表述（消息 ID / msg_id / 消息）。
	304103:   KindPassiveExpired,
	40034005: KindPassiveExpired,
	40034031: KindPassiveExpired,
	//event_id 过期、或该事件不支持被动回复 —— 事件维度的窗口没了，与消息维度同一处置。
	40034026: KindPassiveExpired,
	40034027: KindPassiveExpired,
	//40034128「被动回复时间或次数超限」也归这里：现象同源（被动窗口没了），处置相同。
	//标注：它没有实测样本（我们是按错误码语义归的），第一次真遇到时留意日志确认。
	40034128: KindPassiveExpired,
	//权限类：机器人不在群里、被禁言、没有主动消息权限。
	//它们是**主动降级失败**最常见的原因（见 lib/message 的三级降级收尾日志）：
	//主动群聊撞上这几个码，就该退到私聊。
	40034101: KindPermission,
	40034105: KindPermission,
	40054002: KindPermission,
	40054003: KindPermission,
	40054016: KindPermission,

	40034024: KindNotFound,
	//消息审核中不算失败：api.sendWithRetry 会等审核结果，走到这里说明审核没过。
	304023: KindAudit,

	40034004: KindServer,
	50055001: KindServer,
	50055006: KindServer,

	22006:    KindParam,
	304061:   KindParam,
	304080:   KindParam,
	305007:   KindParam,
	340069:   KindParam,
	40034006: KindParam,
	40034008: KindParam,
	40034009: KindParam,
	40034010: KindParam,
	40034011: KindParam,
	40034025: KindParam,
	40034028: KindParam,
	40034029: KindParam,
	40034106: KindParam,
	40034108: KindParam,
	40034109: KindParam,
	40034124: KindParam,
	40054005: KindParam,
	40054007: KindParam,
	40054010: KindParam,
	40064004: KindParam,
}

// Classify 错误码分类; 未收录视为未知, 由 IsRetryable 的 extra 兜底。
func Classify(code int) Kind {
	if kind, ok := semantic[code]; ok {
		return kind
	}
	return KindUnknown
}

// IsRetryable 重试决策: 语义分类优先, extra 配置码次之。
func IsRetryable(code int, extra []int) bool {
	switch Classify(code) {
	case KindServer:
		return true
	case KindUnknown:
		for _, c := range extra {
			if c == code {
				return true
			}
		}
	}
	return false
}

// As 从错误链中提取业务错误, 便于调用方按语义分支处理。
func As(err error) (*QQError, bool) {
	var qe *QQError
	if errors.As(err, &qe) {
		return qe, true
	}
	return nil, false
}

// IsPassiveExpired 这个错误是否意味着"被动回复窗口没了，该改走主动通道"。
//
// 这是消息层三级降级的**唯一判据**。做成函数而不是让调用方自己比 Kind，
// 是为了让"哪些码算过期"只有 map 里那一处真相 —— 上游加码时改一行就够，
// 不用去追所有 if。
func IsPassiveExpired(err error) bool {
	qe, ok := As(err)
	return ok && qe.Kind == KindPassiveExpired
}
