package bind

import (
	"embed"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/templates"
)

//go:embed templates/markdown/*.md
var templateFS embed.FS

type bindArgs struct {
	UID string `kong:"arg,name='uid',help='要绑定的 UID'"`
}

type confirmArgs struct {
	UID  string `kong:"arg,name='uid',help='要绑定的 UID'"`
	Code string `kong:"arg,name='code',help='绑定验证码'"`
}

func init() {
	commands := make([]*plugin.Command, 0)
	commands = append(commands, &plugin.Command{
		Prefix:   "bind",
		Aliases:  []string{"绑定"},
		Role:     constant.RoleMember,
		Describe: "开始绑定流程",
		Args:     &bindArgs{},
		Handle:   startBind,
		SubCommand: []*plugin.Command{
			{
				Prefix:   "confirm",
				Role:     constant.RoleMember,
				Describe: "确认绑定验证码",
				Args:     &confirmArgs{},
				Handle:   confirmCode,
			},
		},
	})
	self := &plugin.Plugin{}
	self.Commands = commands
	self.Id = "bind"
	self.TemplateFS = templateFS
	plugin.Register(self)
}

func startBind(ctx *context.MessageContext) error {
	a := ctx.Parsed.(*bindArgs)
	msg := ctx.UnsafeMarkdownTemplate("BindGuide", &templates.Args{"uid": a.UID})
	k := &buttons.Keyboard{}
	btn, _ := k.AppendButton("2", "点击我输入UID", "请输入UID", buttons.Blue, 0)
	btn.SetAutoCommand(fmt.Sprintf("/bind confirm %v ", a.UID), false, false).SetUserWhiteList(append(make([]string, 0), ctx.UserId)).SetUnsupportedTip("不支持按钮")
	msg.Keyboard(k)
	return msg.Send()
}

func confirmCode(ctx *context.MessageContext) error {
	a := ctx.Parsed.(*confirmArgs)
	msg := ctx.UnsafeMarkdownTemplate("BindConfirm", &templates.Args{"uid": a.UID, "code": a.Code})
	k := &buttons.Keyboard{}
	btn, _ := k.AppendButton("confirm", "确认无误", "已确认", buttons.Blue, 0)
	btn.SetCallback(fmt.Sprintf("%v %v", a.UID, a.Code), cb).SetUserWhiteList(append(make([]string, 0), ctx.UserId)).SetUnsupportedTip("不支持按钮")
	msg.Keyboard(k)
	return msg.Send()
}

func cb(ctx *context.CallbackContext) error {
	return ctx.Text(fmt.Sprintf("已模拟绑定: %v", ctx.Data)).Send()
}
