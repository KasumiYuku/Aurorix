package buttons

type AllowedPermission int

const (
	SomeUser AllowedPermission = iota
	Admin
	AllUser
)

type ButtonAction int

const (
	Link ButtonAction = iota
	Callback
	Command
)

func IsVaildActionType(actionType ButtonAction) bool {
	return int(Link) >= 0 && int(actionType) <= int(Command)
}

// ButtonStyle 是 QQ 按钮的 render_style 取值。
type ButtonStyle int

const (
	Gray ButtonStyle = iota
	Blue
	GraySolid
	BlueSolid
	BlueText
)
