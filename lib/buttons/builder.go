package buttons

const defaultUnsupportedTip = "当前客户端不支持按钮"

const (
	maxBuilderRows   = 5
	maxBuilderPerRow = 5
)

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

func (b *Builder) Callback(id, label, visited, data string) *Builder {
	return b.add(id, label, visited, Blue, AllUser, nil, func(btn *Button) {
		btn.SetCallbackWithoutHandle(data)
	})
}

func (b *Builder) CallbackFor(id, label, visited, data string, users []string) *Builder {
	return b.add(id, label, visited, Blue, SomeUser, users, func(btn *Button) {
		btn.SetCallbackWithoutHandle(data)
	})
}

func (b *Builder) Command(id, label, visited, content string) *Builder {
	return b.add(id, label, visited, Gray, AllUser, nil, func(btn *Button) {
		btn.SetAutoCommand(content, false, false)
	})
}

func (b *Builder) Primary(id, label, visited, content string) *Builder {
	return b.add(id, label, visited, Blue, AllUser, nil, func(btn *Button) {
		btn.SetAutoCommand(content, false, false)
	})
}

func (b *Builder) Action(id, label, content string) *Builder {
	return b.add(id, label, "已填入", Gray, AllUser, nil, func(btn *Button) {
		btn.SetAutoCommand(content, false, false)
	})
}

func (b *Builder) Link(id, label, visited, url string) *Builder {
	return b.add(id, label, visited, Blue, AllUser, nil, func(btn *Button) {
		btn.SetHref(url)
	})
}

func (b *Builder) LinkPlain(id, label, visited, url string) *Builder {
	return b.add(id, label, visited, Gray, AllUser, nil, func(btn *Button) {
		btn.SetHref(url)
	})
}

func (b *Builder) Build() *Keyboard {
	if b == nil || len(b.kb.Rows) == 0 {
		return nil
	}
	return &b.kb
}

func (b *Builder) add(id, label, visited string, style ButtonStyle, perm AllowedPermission, users []string, apply func(*Button)) *Builder {
	for {
		if b.row >= maxBuilderRows {
			logger.Errorf("按钮板已满, 丢弃按钮 %s", id)
			return b
		}
		if len(b.kb.Rows) <= b.row {
			b.kb.Rows = append(b.kb.Rows, Buttons{})
		}
		if len(b.kb.Rows[b.row].List) < maxBuilderPerRow {
			break
		}
		b.row++
	}
	btn, err := b.kb.AppendButton(b.qualify(id), label, visited, style, b.row)
	if err != nil {
		logger.Errorf("按钮板构造失败(%s): %v", id, err)
		return b
	}
	apply(btn)
	if perm == SomeUser && len(users) > 0 {
		btn.SetUserWhiteList(users)
	} else {
		btn.SetPermission(AllUser)
	}
	btn.SetUnsupportedTip(defaultUnsupportedTip)
	return b
}

func (b *Builder) qualify(id string) string {
	if b.scope == "" {
		return id
	}
	return b.scope + "." + id
}
