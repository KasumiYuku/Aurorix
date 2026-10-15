package quiz

import (
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

// Deliverer 把一张图交给平台: 返回可直接内嵌的图源与量到过的尺寸; 为空时直接交代字节, 由框架上传。
type Deliverer func(m *context.MessageManager, img Image) (src any, size []int)

// NewSender 造按框架默认方式发的出口: 正文按插件 id 填模板, 图走框架上传; id 为空则不填正文。
func NewSender(id string, d Deliverer) Sender { return frameworkSender{id: id, deliver: d} }

type frameworkSender struct {
	id      string
	deliver Deliverer
}

func (s frameworkSender) Post(m *context.MessageManager, out Outgoing) (string, error) {
	b := m.Msg()
	if out.Mention != "" {
		b.AtQuote(out.Mention, true)
	}
	if out.TemplateID != "" {
		text, err := templates.FillFor(s.id, out.TemplateID, out.Args)
		if err != nil {
			logger.Warnf("卡面模板 %s 填充失败: %v", out.TemplateID, err)
		}
		if text != "" {
			b.Markdown(text)
		}
	}
	if out.Keyboard != nil {
		b.Keyboard(out.Keyboard)
	}
	if out.NoFooter {
		b.NoFooter()
	}
	if len(out.Image.Data) > 0 || out.Image.Key != "" {
		src, size := any(out.Image.Data), []int(nil)
		if s.deliver != nil {
			src, size = s.deliver(m, out.Image)
		}
		b.Image(src, out.Summary, size...)
	}
	return b.SendWithID()
}

func (s frameworkSender) Recall(m *context.MessageManager, messageID string) error {
	if messageID == "" || m.Qapi == nil {
		return nil
	}
	if m.Target == constant.PrivateMessage {
		return m.Qapi.RecallC2CMessage(m.UserId, messageID, true)
	}
	return m.Qapi.RecallGroupMessage(m.GroupId, messageID, true)
}
