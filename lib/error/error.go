// Package errorx QQ 平台错误语义化: 错误码分类、重试决策与中文提示。
// 业务码语义收敛在一处, 调用方不再手工维护重试白名单。
package errorx

import (
	"errors"
	"time"
)

// Kind 错误语义分类, 决定是否可重试与退避方式。
type Kind int

const (
	KindUnknown    Kind = iota
	KindRateLimit       // 频率超限, 不盲目重试, 应等待窗口
	KindAuth            // 鉴权失败, 重试无意义
	KindPermission      // 权限不足
	KindNotFound        // 目标不存在
	KindAudit           // 消息审核中, 走审计等待机制
	KindServer          // 服务端错误, 短退避可重试
	KindParam           // 参数错误
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

// 业务码中文提示, 未收录的码返回"未知错误"。
var errorHints = map[int]string{
	100016: "secret 输入错误",
	10004:  "appid 输入错误",
	100007: "机器人被封禁或不存在",
	100017: "接口调用超过频率限制",
	11300:  "仅私域机器人可用（link type check failed）",
	11700:  "机器人已取消该能力",
	11253:  "按钮回调权限不足",
	630006: "header appid 获取失败",
	304023: "消息正在审核中（audit）",
}

// Describe 返回错误码的中文描述。
func Describe(code int) string {
	if hint, ok := errorHints[code]; ok {
		return hint
	}
	return "未知错误"
}

// 内置语义表: 已知码直接定类, 不依赖调用方配置。
// 11253/630006 实践可恢复(权限刚配置/网关瞬时), 不入表, 交由 extra 配置决定。
var semantic = map[int]Kind{
	100016: KindAuth,
	10004:  KindAuth,
	100007: KindAuth,
	100017: KindRateLimit,
	11300:  KindPermission,
	11700:  KindPermission,
	304023: KindAudit,
}

// Classify 错误码分类; 未收录视为未知, 由 IsRetryable 的 extra 兜底。
func Classify(code int) Kind {
	if kind, ok := semantic[code]; ok {
		return kind
	}
	return KindUnknown
}

// IsRetryable 重试决策: 语义分类优先, extra 配置码次之。
// 未知码不重试, 避免对永久性错误做无谓重试风暴。
func IsRetryable(code int, extra []int) bool {
	switch Classify(code) {
	case KindServer, KindUnknown:
	default:
		return false
	}
	for _, c := range extra {
		if c == code {
			return true
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
