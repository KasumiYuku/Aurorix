// Package notice 处理群成员变动: 入群欢迎与退群提示, 文案在框架设置页配置。
package notice

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	errorx "github.com/KasumiYuku/Aurorix/lib/error"
	"github.com/KasumiYuku/Aurorix/lib/event"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/structers"
)

var log = logx.New("notice")

const (
	avatarQlogoSize  = 140
	avatarShowPx     = 40
	codeNoPermission = 40034105
)

// noticeContent 是某个方向(入群/退群)要用的全部素材。
type noticeContent struct {
	enabled bool
	lines   []string
	extra   string
	scope   buttons.FooterScope
}

type handler struct {
	gate *gate
	cfg  func() config.AppConfig
}

// Install 注册群成员变动处理器; 启动时调用一次。
func Install() {
	h := handler{gate: newGate(), cfg: config.Current}
	event.On(constant.GROUP_MEMBER_ADD, h.member)
	event.On(constant.GROUP_MEMBER_REMOVE, h.member)
	event.On(constant.GROUP_MSG_RECEIVE, h.receive)
	event.On(constant.GROUP_MSG_REJECT, h.receive)
}

func (h handler) receive(_ *event.Context, ev *event.ReceiveEvent) {
	if ev == nil || ev.GroupID == "" {
		return
	}
	h.gate.set(ev.GroupID, ev.Enabled)
}

func (h handler) member(ctx *event.Context, ev *event.MemberEvent) {
	if ctx == nil || ev == nil || ev.GroupID == "" || ev.UserID == "" {
		return
	}
	if c := h.content(ev.Added); c.enabled {
		h.announce(ctx, ev, c)
	}
}

func (h handler) content(added bool) noticeContent {
	cfg := h.cfg()
	if added {
		return noticeContent{
			enabled: cfg.WelcomeOn(),
			lines:   cfg.WelcomeMessages,
			extra:   cfg.WelcomeExtra,
			scope:   buttons.FooterNoticeWelcome,
		}
	}
	return noticeContent{
		enabled: cfg.FarewellOn(),
		lines:   cfg.FarewellMessages,
		extra:   cfg.FarewellExtra,
		scope:   buttons.FooterNoticeFarewell,
	}
}

func (h handler) announce(ctx *event.Context, ev *event.MemberEvent, c noticeContent) {
	text := pickLine(c.lines)
	if text == "" {
		return
	}
	avatar, _ := avatarURL(ctx, ev.UserID)
	msg := ctx.Msg().Markdown(compose(avatar, ev.UserID, ev.Added, text, c.extra)).NoFooter()
	if kb := buttons.FooterKeyboard(c.scope, ""); kb != nil {
		msg.Keyboard(kb)
	}
	if ev.Added && ctx.CanReply() {
		if err := msg.Send(); err != nil {
			log.Errorf("发送入群欢迎失败: %v", err)
		}
		return
	}
	h.push(ctx, ev.GroupID, msg)
}

func (h handler) push(ctx *event.Context, groupID string, msg *context.MsgBuilder) {
	if allow, known := h.gate.allow(ctx.Qapi, groupID); known && !allow {
		log.Infof("群 %s 未开启主动消息, 跳过成员通知", groupID)
		return
	}
	if err := msg.Initiative().Send(); err != nil {
		if qe, ok := errorx.As(err); ok && qe.Code == codeNoPermission {
			h.gate.set(groupID, false)
			log.Infof("群 %s 未授权主动消息, 跳过成员通知: %v", groupID, err)
			return
		}
		log.Errorf("发送群成员通知失败: %v", err)
	}
}

// compose 拼出最终正文: 头一行是圆形头像和 @(退群没有 @), 文案另起一行, 附加内容再另起一行。
// 附加内容原样落进正文 —— 换行、引用块、内联指令标签都由它自己决定, 框架不替它排版。
func compose(avatar, userID string, added bool, text, extra string) string {
	if text == "" {
		return ""
	}
	head := ""
	if avatar != "" {
		head = fmt.Sprintf("![cornerRadius=100 #%dpx #%dpx](%s)", avatarShowPx, avatarShowPx, avatar)
	}
	if added {
		if head != "" {
			head += " "
		}
		head += "<@" + userID + ">"
	}
	var b strings.Builder
	if head != "" {
		b.WriteString("##")
		b.WriteString(head)
		b.WriteString("\n")
	}
	b.WriteString(text)
	if extra != "" {
		b.WriteString("\n")
		b.WriteString(extra)
	}
	return b.String()
}

func avatarURL(ctx *event.Context, userID string) (string, bool) {
	if ctx.Qapi == nil || ctx.Qapi.AppID == "" || userID == "" {
		return "", false
	}
	return structers.AvatarURLSize(ctx.Qapi.AppID, userID, avatarQlogoSize), true
}

func pickLine(pool []string) string {
	lines := make([]string, 0, len(pool))
	for _, line := range pool {
		if s := strings.TrimSpace(line); s != "" {
			lines = append(lines, s)
		}
	}
	switch len(lines) {
	case 0:
		return ""
	case 1:
		return lines[0]
	}
	return lines[rand.IntN(len(lines))]
}
