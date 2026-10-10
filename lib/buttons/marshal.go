package buttons

import (
	"encoding/json"
	"fmt"
)

var commandNormalizer func(string) string

func SetCommandNormalizer(fn func(string) string) { commandNormalizer = fn }

func GenerateJson(keyboard Keyboard) ([]byte, error) {
	if len(keyboard.Rows) == 0 {
		return make([]byte, 0), nil
	}
	if len(keyboard.Rows) > 5 {
		return make([]byte, 0), fmt.Errorf("Rows must less than 5 lines, but this keyboard has %v", len(keyboard.Rows))
	}
	for k, v := range keyboard.Rows {
		if len(v.List) > 5 {
			return make([]byte, 0), fmt.Errorf("Buttons in one row must less than 5, but row %v has %v", k, len(v.List))
		}
	}
	for i := 0; i < len(keyboard.Rows); i++ {
		for j := 0; j < len(keyboard.Rows[i].List); j++ {
			value := &keyboard.Rows[i].List[j]

			if value.Type == Callback && value.CallbackData == "" {
				return make([]byte, 0), fmt.Errorf("Button %v need CallbackData when ActionType is Callback", value.Id)
			} else if value.Type == Command && value.Msg == "" {
				return make([]byte, 0), fmt.Errorf("Button %v need Msg when ActionType is Command", value.Id)
			} else if value.Type == Link && value.Url == "" {
				return make([]byte, 0), fmt.Errorf("Button %v need Url when ActionType is Link", value.Id)
			} else if !IsVaildActionType(value.Type) {
				return make([]byte, 0), fmt.Errorf("Button %v define a invailed actionType: %v", value.Id, value.Type)
			}

			value.JsonData = actionJson{}
			value.JsonData.Type = int(value.Type)
			switch value.Type {
			case Callback:
				value.JsonData.Data = value.CallbackData
			case Command:
				msg := value.Msg
				if commandNormalizer != nil {
					msg = commandNormalizer(msg)
				}
				value.JsonData.Data = msg
			case Link:
				value.JsonData.Data = value.Url
			}
			value.JsonData.Reply = value.Reply
			value.JsonData.Enter = value.AutoSend
			value.JsonData.Anchor = value.Anchor
			value.JsonData.UnsupportTips = value.UnsupportTips
			value.JsonData.Permission = value.Permission
		}
	}
	raw := keyboardJson{
		Content: keyboard,
	}
	js, err := json.Marshal(raw)
	if err != nil {
		return make([]byte, 0), err
	}
	return js, nil
}
