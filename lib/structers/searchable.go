package structers

import "strings"

// Searchable 把一条消息里所有可能有链接的文本按顺序摊平去重。正文、结构化卡片、卡片的每个消息元素字段名与层级都不一样, 收在这里下游才不用各扫各的; 每个来源额外附一份反转义后的副本。
func Searchable(content string, ark *ArkData, elements []MsgElement) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 8)
	put := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	put(content)
	if ark != nil {
		put(ark.Prompt)
		for _, u := range ark.URLs() {
			put(u)
		}
		put(string(ark.Raw))
	}
	for _, el := range FlattenElements(elements) {
		put(el.Content)
		if el.ArkData != nil {
			put(el.ArkData.Prompt)
			for _, u := range el.ArkData.URLs() {
				put(u)
			}
			put(string(el.ArkData.Raw))
		}
	}

	for _, s := range append([]string(nil), out...) {
		if unescaped := UnescapeJSONString(s); unescaped != s {
			put(unescaped)
		}
	}
	return out
}

// UnescapeJSONString 还原 JSON 字符串里的 \/ \\ \" \n \r \t; 不整段 json.Unmarshal, 因为卡片正文经常是残缺或被截断的 JSON。
func UnescapeJSONString(s string) string {
	at := strings.IndexByte(s, '\\')
	if at < 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	out.WriteString(s[:at])
	for i := at; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case '/', '\\', '"':
			out.WriteByte(s[i+1])
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		default:
			out.WriteByte(s[i])
			continue
		}
		i++
	}
	return out.String()
}
