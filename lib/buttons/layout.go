package buttons

func HasButton(kb *Keyboard, id string) bool {
	if kb == nil {
		return false
	}
	for _, row := range kb.Rows {
		for _, btn := range row.List {
			if btn.Id == id {
				return true
			}
		}
	}
	return false
}

func MergeTrailing(kb *Keyboard, perRow int, extra ...Button) *Keyboard {
	if kb == nil {
		kb = &Keyboard{}
	}
	origin := flattenRows(kb)
	if len(origin) == 0 {
		if len(extra) > 0 {
			kb.Rows = []Buttons{{List: extra}}
		}
		return kb
	}
	if perRow < 1 {
		perRow = maxBuilderPerRow
	}
	all := make([]Button, 0, len(origin)+len(extra))
	all = append(all, origin...)
	all = append(all, extra...)
	rows := []Buttons{{List: []Button{all[0]}}}
	for rest := all[1:]; len(rest) > 0 && len(rows) < maxBuilderRows; {
		take := min(perRow, len(rest))
		rows = append(rows, Buttons{List: append([]Button(nil), rest[:take]...)})
		rest = rest[take:]
	}
	if used := rowButtonCount(rows); used < len(all) {
		logger.Errorf("按钮板已满, 丢弃 %d 个尾部按钮", len(all)-used)
	}
	kb.Rows = rows
	return kb
}

func flattenRows(kb *Keyboard) []Button {
	var out []Button
	for _, row := range kb.Rows {
		out = append(out, row.List...)
	}
	return out
}

func rowButtonCount(rows []Buttons) int {
	total := 0
	for _, row := range rows {
		total += len(row.List)
	}
	return total
}
