package structers

import "strings"

// Searchable 把一条消息里所有"可能有链接的文本"按顺序摊平, 去重。
//
// 为什么要有这一层: QQ 把分享内容放在好几个地方 —— 正文、结构化卡片、卡片的每一个
// 消息元素, 三种来源的字段名和层级都不一样。下游(比如链接解析)如果各扫各的,
// 每加一种卡片形状就要多一层判断, 最后堆成一团。
//
// 这里统一收口: 谁想知道"这条消息里出现过什么文字", 都问这一个函数。
// 加新来源 = 往下面追加一处, 而不是在下游加分支。
//
// 注意转义: 小程序卡片的正文是一段 JSON, 里面的链接是 https:\/\/… 这种转义形态,
// 所以每个来源都会额外给一份反转义后的文本 —— 下游拿到的就是可以直接跑正则的字符串。
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

	// 反转义后的副本跟在原文后面: 原文能匹配到就不必再看它。
	for _, s := range append([]string(nil), out...) {
		if unescaped := UnescapeJSONString(s); unescaped != s {
			put(unescaped)
		}
	}
	return out
}

// UnescapeJSONString 还原 JSON 字符串里的转义: \/ \\ \" \n \r \t。
//
// 不整段 json.Unmarshal 是因为卡片正文经常是残缺或被截断的 JSON, 解不开就什么都不剩;
// 逐字符扫则坏掉的字符不影响其余部分。
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
