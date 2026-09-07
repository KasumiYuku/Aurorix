package buttons

import (
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/context"
)

type RenderData struct {
	Label   string      `json:"label"`
	Visited string      `json:"visited_label"`
	Style   ButtonStyle `json:"style"`
}

type Permission struct {
	Type           AllowedPermission `json:"type"`
	SpecifyUserIds []string          `json:"specify_user_ids,omitempty"`
}

type Action struct {
	Type          ButtonAction `json:"-"`
	Url           string       `json:"-"` // 跳转目标
	CallbackData  string       `json:"-"` // 回调数据
	Msg           string       `json:"-"` // 自动消息
	Reply         bool         `json:"-"` // 是否携带引用
	AutoSend      bool         `json:"-"` // 是否自动发送指令
	Anchor        bool         `json:"-"` // 唤起图片选择
	Permission    Permission   `json:"-"` // 权限设置
	UnsupportTips string       `json:"-"` // 不支持此功能时的提示
}

type actionJson struct {
	Type          int        `json:"type"`
	Data          string     `json:"data"`
	Reply         bool       `json:"reply,omitempty"`
	Anchor        bool       `json:"anchor,omitempty"`
	UnsupportTips string     `json:"unsupport_tips,omitempty"`
	Permission    Permission `json:"permission"`
}

type Button struct {
	Id         string     `json:"id"`
	RenderData RenderData `json:"render_data"`
	Action     `json:"-"`
	JsonData   actionJson `json:"action"`
}

type Buttons struct {
	List []Button `json:"buttons"`
}

type Keyboard struct {
	Rows []Buttons `json:"rows"`
}

type keyboardJson struct {
	Content Keyboard `json:"content"`
}

func (k *Keyboard) Marshal() ([]byte, error) {
	return GenerateJson(*k)
}

func (k *Keyboard) AppendButton(id string, label string, visited string, style ButtonStyle, row int) (*Button, error) {
	if row > 4 || row < 0 {
		return nil, fmt.Errorf("行号需在 0~4 之间，收到 %d", row)
	}
	button := Button{
		RenderData: RenderData{
			Label:   label,
			Visited: visited,
			Style:   style,
		},
		Id: id,
	}
	// 补全长度
	if len(k.Rows) <= row {
		for len(k.Rows) != row+1 {
			k.Rows = append(k.Rows, Buttons{})
		}
	}
	if len(k.Rows[row].List) == 5 {
		return nil, fmt.Errorf("The row %v is full, can't append new button", row)
	}
	k.Rows[row].List = append(k.Rows[row].List, button)
	return &k.Rows[row].List[len(k.Rows[row].List)-1], nil
}

func (button *Button) SetAutoCommand(content string, autoSend bool, anchor bool) *Button {
	button.Action.Anchor = anchor
	button.Action.Msg = content
	button.Action.Type = Command
	button.Action.AutoSend = autoSend
	return button
}

func (button *Button) SetHref(url string) *Button {
	button.Action.Type = Link
	button.Action.Url = url
	return button
}

func (button *Button) SetCallback(data string, handle CallbackButtonHandleFunc) *Button {
	// 储存回调数据
	button.Action.CallbackData = data
	button.Action.Type = Callback
	// 添加回调函数
	CallbackFuncMapLock.Lock()
	defer CallbackFuncMapLock.Unlock()
	CallbackFuncMap[button.Id] = handle
	return button
}

func (button *Button) SetCallbackWithoutHandle(data string) *Button {
	// 储存回调数据
	button.Action.CallbackData = data
	button.Action.Type = Callback
	return button
}

func (button *Button) SetPermission(required AllowedPermission) *Button {
	button.Permission.Type = required
	return button
}

func (button *Button) SetUserWhiteList(users []string) *Button {
	button.Permission.Type = SomeUser
	button.Permission.SpecifyUserIds = users
	return button
}

func (button *Button) SetUnsupportedTip(tip string) *Button {
	button.Action.UnsupportTips = tip
	return button
}

type CallbackButtonHandleFunc func(*context.CallbackContext) error
