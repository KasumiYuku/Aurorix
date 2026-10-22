package buttons

const defaultUnsupportedTip = "当前客户端不支持按钮"

const (
	maxBuilderRows   = 5
	maxBuilderPerRow = 5
)

type Opt func(*Button)

func Style(style ButtonStyle) Opt { return func(b *Button) { b.RenderData.Style = style } }

func Visited(label string) Opt { return func(b *Button) { b.RenderData.Visited = label } }

func Only(users ...string) Opt { return func(b *Button) { b.SetUserWhiteList(users) } }

func AutoSend() Opt { return func(b *Button) { b.Action.AutoSend = true } }

func Tip(text string) Opt { return func(b *Button) { b.Action.UnsupportTips = text } }

type Builder struct {
	kb    Keyboard
	scope string
	row   int
}

func New() *Builder { return &Builder{} }

func (b *Builder) Scope(pluginID string) *Builder {
	b.scope = pluginID
	return b
}

func (b *Builder) Row() *Builder {
	b.row++
	return b
}

func (b *Builder) Link(id, label, url string, opts ...Opt) *Builder {
	return b.add(id, label, Blue, func(btn *Button) { btn.SetHref(url) }, opts)
}

func (b *Builder) Callback(id, label, data string, opts ...Opt) *Builder {
	return b.add(id, label, Blue, func(btn *Button) { btn.SetCallback(data, nil) }, opts)
}

func (b *Builder) Command(id, label, content string, opts ...Opt) *Builder {
	return b.add(id, label, Gray, func(btn *Button) { btn.SetAutoCommand(content, false, false) }, opts)
}

func (b *Builder) Build() *Keyboard {
	if b == nil || len(b.kb.Rows) == 0 {
		return nil
	}
	return &b.kb
}

func (b *Builder) add(id, label string, style ButtonStyle, apply func(*Button), opts []Opt) *Builder {
	row, ok := b.slot(id)
	if !ok {
		return b
	}
	btn, err := b.kb.AppendButton(Qualify(b.scope, id), label, label, style, row)
	if err != nil {
		logger.Errorf("按钮板构造失败(%s): %v", id, err)
		return b
	}
	btn.SetUserWhiteList(nil)
	btn.SetUnsupportedTip(defaultUnsupportedTip)
	apply(btn)
	for _, opt := range opts {
		opt(btn)
	}
	return b
}

func (b *Builder) slot(id string) (int, bool) {
	for b.row < maxBuilderRows {
		if len(b.kb.Rows) <= b.row {
			b.kb.Rows = append(b.kb.Rows, Buttons{})
		}
		if len(b.kb.Rows[b.row].List) < maxBuilderPerRow {
			return b.row, true
		}
		b.row++
	}
	logger.Errorf("按钮板已满, 丢弃按钮 %s", id)
	return 0, false
}

func Qualify(scope, id string) string {
	if scope == "" {
		return id
	}
	return scope + "." + id
}
