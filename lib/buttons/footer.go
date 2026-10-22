package buttons

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

// 框架级「底部按钮」: 给每条 Markdown 消息末尾追加可配置的底部按钮行。
//
// 配置写在 config.json 的 footer_buttons, 或在管理台「设置」页的多行文本框里编辑。
//
// 每个配置行 = 键盘上的一行按钮; 同一行可以放多个按钮, 用逗号隔开;
// 每个按钮写成「标签 | 类型 | 参数 [; 选项]」:
//
//	+1 | counter | 感谢支持，当前 {{count}} 次 ; dedup=user , 帮助 | command | /help
//	文档 | link | https://example.com
//
// 类型: counter(计数) / command(指令) / link(链接) / repeat(复读, 把触发这条回复的原消息填进输入框) / image(图片, 点击把那一段 URL 的图发给点击者); 选项是 k=v, 多个用 ';' 隔开。
//
// 两个必须知道的平台事实:
//   - 按钮上的文字在发送那一刻就写死了 —— 平台没有消息编辑接口, 所以计数只能出现在点击后的回复里,
//     不可能实时刷新按钮上的数字。
//   - 回调按钮的 ID 固定为 aurorix.footer.<序号>, 全部指向同一个分发器; 分发器每次都去读「当前配置」,
//     所以改配置热更后立即生效, 不需要重新注册处理器。
const (
	footerIDPrefix     = "aurorix.footer."
	footerMaxButtons   = 5 // 底部按钮占一整行, 与平台「每行最多 5 个」对齐
	footerOptionSep    = ";"
	footerDefaultReply = "已记录，当前 {{count}} 次。"
)

// FooterScope 标识一组底部按钮的归属。所有作用域共用同一张回调表, 靠槽位段区分:
// 每个作用域占 footerMaxButtons 个连续槽位, 所以不同来源的按钮永远不会串到一起。
type FooterScope int

const (
	// FooterGlobal 全局底部按钮, 挂在每条 Markdown 消息末尾。
	FooterGlobal FooterScope = iota
	// FooterNoticeWelcome 入群欢迎专属按钮。
	FooterNoticeWelcome
	// FooterNoticeFarewell 退群提示专属按钮。
	FooterNoticeFarewell
)

// footerScopes 是全部作用域; 新增一个就在这里加一行, 槽位数与回调注册自动跟上。
var footerScopes = []FooterScope{FooterGlobal, FooterNoticeWelcome, FooterNoticeFarewell}

func (s FooterScope) slotBase() int { return int(s) * footerMaxButtons }

// slug 是计数键里的作用域名; 全局返回空串, 沿用 "footer.<键>" 的旧格式。
func (s FooterScope) slug() string {
	switch s {
	case FooterNoticeWelcome:
		return "notice.welcome"
	case FooterNoticeFarewell:
		return "notice.farewell"
	}
	return ""
}

func (s FooterScope) String() string {
	switch s {
	case FooterNoticeWelcome:
		return "入群欢迎"
	case FooterNoticeFarewell:
		return "退群提示"
	}
	return "全局"
}

// footerSlotOf 把回调槽位号还原成 (作用域, 作用域内序号)。
func footerSlotOf(slot int) (FooterScope, int, bool) {
	if slot < 0 || slot/footerMaxButtons >= len(footerScopes) {
		return FooterGlobal, 0, false
	}
	return FooterScope(slot / footerMaxButtons), slot % footerMaxButtons, true
}

// FooterKind 底部按钮的行为类型。
type FooterKind string

const (
	// FooterCounter 计数: 点击累加一次并按模板回复。
	FooterCounter FooterKind = "counter"
	// FooterCommand 指令: 点击把指令填进输入框, auto=1 时直接发送。
	FooterCommand FooterKind = "command"
	// FooterLink 链接: 点击跳转。
	FooterLink FooterKind = "link"
	// FooterRepeat 复读: 点击把「触发这条回复的那条原消息」填进输入框, auto=1 时直接发送。
	FooterRepeat FooterKind = "repeat"
	// FooterImage 图片: 点击把那一段 URL 的图发给点击者。
	FooterImage FooterKind = "image"
	// FooterCallback 回调: 点击转给指定 id 的回调函数, 由注册它的那一方决定发什么。
	FooterCallback FooterKind = "callback"
)

// FooterSpec 一行配置解析后的结果。
type FooterSpec struct {
	Label   string
	Kind    FooterKind
	Arg     string
	Options map[string]string
	// Key 计数键, 默认取标签; 两个计数按钮想共用计数或标签重复时用 key= 显式指定。
	Key string
}

func (s FooterSpec) option(name, fallback string) string {
	if value, ok := s.Options[name]; ok && value != "" {
		return value
	}
	return fallback
}

// FooterParseResult 解析结果: 合法按钮(展平后顺序即回调序号)、按配置行分好的键盘行、
// 以及逐行错误(带行号)。一行写错不影响其它行。
type FooterParseResult struct {
	Specs  []FooterSpec
	Rows   [][]FooterSpec
	Errors []string
}

// footerReplyPlaceholders 回复模板里允许出现的占位符。
var footerReplyPlaceholders = []string{"count", "user", "group", "label"}

var footerPlaceholderRe = regexp.MustCompile(`\{\{([^}]*)\}\}`)

var footerAllowedOptions = map[FooterKind][]string{
	FooterCounter:  {"key", "dedup", "scope", "style", "visited", "dup"},
	FooterCommand:  {"key", "auto", "style", "visited"},
	FooterLink:     {"key", "style", "visited"},
	FooterRepeat:   {"auto", "style", "visited"},
	FooterImage:    {"style", "visited"},
	FooterCallback: {"style", "visited"},
}

// ParseFooter 解析底部按钮配置。空行与 # 开头的注释行跳过。
//
// 每个配置行对应键盘上的一行按钮: 同一行可以写多个按钮(用逗号隔开), 也可以一行只写一个。
// 单个按钮写「标签 | 类型 | 参数 [; 选项]」, 详见文件头注释。
func ParseFooter(lines []string) FooterParseResult {
	var out FooterParseResult
	seenKeys := make(map[string]int, len(lines))
	for i, raw := range lines {
		lineNo := i + 1
		var row []FooterSpec
		for _, chunk := range splitFooterLine(raw) {
			text := strings.TrimSpace(chunk)
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			if len(out.Specs) >= footerMaxButtons {
				out.Errors = append(out.Errors, fmt.Sprintf("第 %d 行: 底部按钮最多 %d 个, 多出的被忽略", lineNo, footerMaxButtons))
				continue
			}
			spec, err := parseFooterLine(text)
			if err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("第 %d 行: %v", lineNo, err))
				continue
			}
			if spec.Kind == FooterCounter {
				if first, dup := seenKeys[spec.Key]; dup {
					out.Errors = append(out.Errors, fmt.Sprintf(
						"第 %d 行: 计数键 %q 与第 %d 行重复, 用 key= 区分(或删掉一个), 该按钮被忽略", lineNo, spec.Key, first))
					continue
				}
				seenKeys[spec.Key] = lineNo
			}
			row = append(row, spec)
			out.Specs = append(out.Specs, spec)
		}
		if len(row) > 0 {
			out.Rows = append(out.Rows, row)
		}
	}
	return out
}

// splitFooterLine 把一行拆成若干按钮: 用半角逗号分隔。
// 只有逗号后面那截「看起来像一个按钮」(带 | 且第二段是合法类型)时才真的拆,
// 否则逗号算文案的一部分 —— 这样「感谢支持, 当前 {{count}} 次」不会被拆坏。
func splitFooterLine(line string) []string {
	if !strings.Contains(line, ",") {
		return []string{line}
	}
	chunks := strings.Split(line, ",")
	out := []string{chunks[0]}
	for _, chunk := range chunks[1:] {
		if footerLooksLikeButton(chunk) {
			out = append(out, chunk)
			continue
		}
		out[len(out)-1] += "," + chunk
	}
	return out
}

func footerLooksLikeButton(chunk string) bool {
	parts := strings.Split(chunk, "|")
	if len(parts) < 2 {
		return false
	}
	_, err := parseFooterKind(strings.TrimSpace(parts[1]))
	return err == nil
}

func parseFooterLine(text string) (FooterSpec, error) {
	parts := strings.Split(text, "|")
	if len(parts) < 2 {
		return FooterSpec{}, errors.New("至少要写「标签 | 类型」两段")
	}
	if len(parts) > 4 {
		return FooterSpec{}, fmt.Errorf("最多 4 段「标签 | 类型 | 参数 | 选项」, 现在有 %d 段(文案里不要出现 | 符号)", len(parts))
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if parts[0] == "" {
		return FooterSpec{}, errors.New("标签不能为空")
	}
	kind, err := parseFooterKind(parts[1])
	if err != nil {
		return FooterSpec{}, err
	}
	spec := FooterSpec{Label: parts[0], Kind: kind}
	optionText := ""
	switch len(parts) {
	case 4:
		spec.Arg, optionText = parts[2], parts[3]
	case 3:
		// 参数后面可以直接跟 "; k=v" 形式的选项, 省掉一个竖线
		spec.Arg, optionText = splitInlineFooterOptions(parts[2])
	}
	if spec.Options, err = parseFooterOptions(optionText); err != nil {
		return FooterSpec{}, err
	}
	spec.Key = spec.option("key", spec.Label)
	if err := spec.validate(); err != nil {
		return FooterSpec{}, err
	}
	return spec, nil
}

func parseFooterKind(raw string) (FooterKind, error) {
	switch raw {
	case "counter", "计数", "点赞":
		return FooterCounter, nil
	case "command", "指令":
		return FooterCommand, nil
	case "link", "链接":
		return FooterLink, nil
	case "repeat", "复读", "回填":
		return FooterRepeat, nil
	case "image", "图片", "图":
		return FooterImage, nil
	case "callback", "回调":
		return FooterCallback, nil
	}
	return "", fmt.Errorf("类型 %q 不认识, 只能是 counter(计数) / command(指令) / link(链接) / repeat(复读) / image(图片) / callback(回调)", raw)
}

func parseFooterOptions(text string) (map[string]string, error) {
	options := make(map[string]string)
	for _, pair := range strings.Split(text, footerOptionSep) {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("选项 %q 不是 k=v 形式, 例如 dedup=user", pair)
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if name == "" {
			return nil, fmt.Errorf("选项 %q 缺少名字", pair)
		}
		options[name] = value
	}
	return options, nil
}

// splitInlineFooterOptions 拆开「参数 ; k=v ; k2=v2」。
// 只有分号后面的片段**都是** k=v 形状时才当选项 —— 否则整段都算参数(文案里本来就可能带分号)。
func splitInlineFooterOptions(raw string) (arg, options string) {
	chunks := strings.Split(raw, footerOptionSep)
	if len(chunks) < 2 {
		return raw, ""
	}
	for _, chunk := range chunks[1:] {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		name, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return raw, ""
		}
	}
	return strings.TrimSpace(chunks[0]), strings.Join(chunks[1:], footerOptionSep)
}

func (s FooterSpec) validate() error {
	allowed := footerAllowedOptions[s.Kind]
	for _, name := range sortedKeys(s.Options) {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("类型 %s 不支持选项 %q(可用: %s)", s.Kind, name, strings.Join(allowed, ", "))
		}
	}
	if style, ok := s.Options["style"]; ok && style != "blue" && style != "gray" {
		return fmt.Errorf("style 只能是 blue 或 gray, 收到 %q", style)
	}
	switch s.Kind {
	case FooterCounter:
		if scope, ok := s.Options["scope"]; ok && scope != "group" && scope != "global" {
			return fmt.Errorf("scope 只能是 group 或 global, 收到 %q", scope)
		}
		if dedup, ok := s.Options["dedup"]; ok && dedup != "user" && dedup != "group" && dedup != "none" {
			return fmt.Errorf("dedup 只能是 user / group / none, 收到 %q", dedup)
		}
		if err := checkFooterPlaceholders("回复模板", s.Arg); err != nil {
			return err
		}
		if dup := s.Options["dup"]; dup != "" {
			if err := checkFooterPlaceholders("dup 文案", dup); err != nil {
				return err
			}
		}
	case FooterCommand:
		if s.Arg == "" {
			return errors.New("command 要在第 3 段写指令内容, 例如 /help")
		}
		if auto, ok := s.Options["auto"]; ok && auto != "0" && auto != "1" {
			return fmt.Errorf("auto 只能是 0 或 1, 收到 %q", auto)
		}
	case FooterLink:
		target, err := url.Parse(s.Arg)
		if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
			return fmt.Errorf("link 的第 3 段要是 http(s) 链接, 收到 %q", s.Arg)
		}
	case FooterRepeat:
		if s.Arg != "" {
			return errors.New("repeat(复读) 不用写第 3 段: 它填的就是触发这条回复的原消息, 把这段删掉")
		}
		if auto, ok := s.Options["auto"]; ok && auto != "0" && auto != "1" {
			return fmt.Errorf("auto 只能是 0 或 1, 收到 %q", auto)
		}
	case FooterImage:
		target, err := url.Parse(s.Arg)
		if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
			return fmt.Errorf("image 的第 3 段要是 http(s) 图片链接, 收到 %q", s.Arg)
		}
	case FooterCallback:
		if s.Arg == "" {
			return errors.New("callback(回调) 要在第 3 段写回调 id, 例如 okamikko.noat.guide")
		}
		if _, ok := GetCallbackFunc(s.Arg); !ok {
			return fmt.Errorf("callback(回调) 指向的 %q 没有注册, 点了不会有反应", s.Arg)
		}
	}
	return nil
}

// checkFooterPlaceholders 拦住写错的占位符 —— 否则它会当普通文本静默发出去, 很难查。
func checkFooterPlaceholders(where, tmpl string) error {
	for _, m := range footerPlaceholderRe.FindAllStringSubmatch(tmpl, -1) {
		name := strings.TrimSpace(m[1])
		if !slices.Contains(footerReplyPlaceholders, name) {
			return fmt.Errorf("%s 里的占位符 %q 不认识(可用: {{count}} {{user}} {{group}} {{label}})", where, m[0])
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// footerButton 把一条配置变成按钮; ok=false 表示这条在当前消息上放不了(目前只有「没有原消息的复读」)。
// index 是它在整份配置里的序号, 也是回调按钮携带的序号。
func footerButton(spec FooterSpec, index int, originalInput string) (Button, bool) {
	btn := Button{
		Id: footerButtonID(index),
		RenderData: RenderData{
			Label:   spec.Label,
			Visited: spec.option("visited", footerDefaultVisited(spec.Kind)),
			Style:   footerStyle(spec.option("style", "")),
		},
	}
	switch spec.Kind {
	case FooterCounter:
		btn.SetCallback(strconv.Itoa(index), nil)
	case FooterCommand:
		btn.SetAutoCommand(spec.Arg, spec.option("auto", "0") == "1", false)
	case FooterLink:
		btn.SetHref(spec.Arg)
	case FooterRepeat:
		if strings.TrimSpace(originalInput) == "" {
			return Button{}, false
		}
		btn.SetAutoCommand(originalInput, spec.option("auto", "0") == "1", false)
	case FooterImage:
		btn.SetCallback(strconv.Itoa(index), nil)
	case FooterCallback:
		// data 是平台必填字段; 按 id 路由的回调不靠它传参, 就填回调 id 本身。
		btn.Id = spec.Arg
		btn.SetCallback(spec.Arg, nil)
	}
	btn.SetPermission(AllUser)
	btn.SetUnsupportedTip(defaultUnsupportedTip)
	return btn, true
}

// FooterButtons 按解析结果构造一整排全局底部按钮; 放不了的按钮会被跳过。空配置返回 nil。
func FooterButtons(specs []FooterSpec, originalInput string) []Button {
	out := make([]Button, 0, len(specs))
	for i, spec := range specs {
		if btn, ok := footerButton(spec, i, originalInput); ok {
			out = append(out, btn)
		}
	}
	return out
}

// footerButtonRows 按配置行分组构造按钮; 放不了的按钮从所在行去掉, 空行不出现。
func footerButtonRows(result FooterParseResult, base int, originalInput string) [][]Button {
	var out [][]Button
	flat := 0
	for _, row := range result.Rows {
		list := make([]Button, 0, len(row))
		for range row {
			if btn, ok := footerButton(result.Specs[flat], base+flat, originalInput); ok {
				list = append(list, btn)
			}
			flat++
		}
		if len(list) > 0 {
			out = append(out, list)
		}
	}
	return out
}

func footerButtonID(index int) string { return footerIDPrefix + strconv.Itoa(index) }

func footerDefaultVisited(kind FooterKind) string {
	switch kind {
	case FooterCounter:
		return "已投"
	case FooterCommand, FooterRepeat:
		return "已填入"
	case FooterImage, FooterCallback:
		return "已发送"
	}
	return ""
}

// footerStyle 认数字(与 QQ 的 render_style 一致)和 gray/blue 两个别名; 认不出就用文字样式。
func footerStyle(name string) ButtonStyle {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "0", "gray", "grey":
		return Gray
	case "1", "blue":
		return Blue
	case "2":
		return GraySolid
	case "3":
		return BlueSolid
	}
	return BlueText
}

// FooterMerge 是给 message.SetFooterHook 用的合并函数: 按配置行给消息原有键盘追加底部按钮行。
// originalInput 是触发这条消息的原消息文本(主动推送时为空); 复读按钮靠它把原文填回用户输入框。
// 插件自己排好的行不动; 键盘已满时放弃放不下的底部行并记警告, 保证插件 UI 不被破坏。
func FooterMerge(kb contract.CanMarshal, originalInput string) contract.CanMarshal {
	result, errs := currentFooterResult(FooterGlobal)
	logFooterErrors(errs)
	if len(result.Specs) == 0 {
		return kb
	}
	rows := footerButtonRows(result, FooterGlobal.slotBase(), originalInput)
	if len(rows) == 0 {
		return kb
	}
	if kb == nil {
		fresh := &Keyboard{}
		if !appendFooterRows(fresh, rows) {
			return nil
		}
		return fresh
	}
	existing, ok := kb.(*Keyboard)
	if !ok {
		logger.Warnf("消息上挂的是非 Keyboard 实现的按钮板, 放弃追加底部按钮")
		return kb
	}
	appendFooterRows(existing, rows)
	return existing
}

// FooterKeyboard 构造某个作用域专有的按钮板; 该作用域没配按钮时返回 nil。
// 用于不该跟全局底部按钮混在一起的消息, 例如入群欢迎。
func FooterKeyboard(scope FooterScope, originalInput string) *Keyboard {
	result, errs := currentFooterResult(scope)
	logFooterErrors(errs)
	rows := footerButtonRows(result, scope.slotBase(), originalInput)
	if len(rows) == 0 {
		return nil
	}
	kb := &Keyboard{}
	if !appendFooterRows(kb, rows) {
		return nil
	}
	return kb
}

// appendFooterRows 把按钮行逐行追加到键盘末尾; 某行装不下时停止并返回 false。
func appendFooterRows(kb *Keyboard, rows [][]Button) bool {
	for _, row := range rows {
		if !AppendRow(kb, row...) {
			logger.Warnf("按钮板已满 %d 行, 放弃追加底部按钮(插件按钮优先)", maxBuilderRows)
			return false
		}
	}
	return true
}

var (
	footerMu      sync.RWMutex
	footerSources = map[FooterScope]func() []string{}
	footerCaches  = map[FooterScope]*footerCache{}
	footerOnce    sync.Once
)

// footerCache 记住某个作用域上一次解析的结果; 配置没变就不重复解析, 错误也只报一次。
type footerCache struct {
	ready  bool
	raw    string
	result FooterParseResult
}

// EnableFooter 打开框架级底部按钮: 注入配置来源、注册回调分发器、挂上消息钩子。
// source 每次调用都应返回「当前」配置(通常是 config.Current().FooterButtons), 这样热更立即生效。
func EnableFooter(source func() []string) {
	SetFooterSource(FooterGlobal, source)
	message.SetFooterHook(FooterMerge)
}

// SetFooterSource 给某个作用域注入配置来源; 传 nil 表示停用该作用域。
// source 每次调用都应返回「当前」配置, 所以热更立即生效, 不必重启。
func SetFooterSource(scope FooterScope, source func() []string) {
	footerMu.Lock()
	if source == nil {
		delete(footerSources, scope)
	} else {
		footerSources[scope] = source
	}
	delete(footerCaches, scope)
	footerMu.Unlock()
	InstallFooterHandlers()
}

// DisableFooter 关掉框架级底部按钮, 主要给测试与运行期停用。
func DisableFooter() {
	SetFooterSource(FooterGlobal, nil)
	message.SetFooterHook(nil)
}

// currentFooterResult 读一次某个作用域的配置并解析; 同一批配置只解析一次, 错误也只报一次, 避免逐条消息刷屏。
func currentFooterResult(scope FooterScope) (FooterParseResult, []string) {
	footerMu.Lock()
	defer footerMu.Unlock()
	src := footerSources[scope]
	if src == nil {
		return FooterParseResult{}, nil
	}
	raw := strings.Join(src(), "\n")
	cache := footerCaches[scope]
	if cache == nil {
		cache = &footerCache{}
		footerCaches[scope] = cache
	}
	if cache.ready && raw == cache.raw {
		return cache.result, nil
	}
	result := ParseFooter(strings.Split(raw, "\n"))
	cache.ready, cache.raw, cache.result = true, raw, result
	return result, result.Errors
}

// currentFooterSpecs 只取展平后的按钮列表(回调分发用), 顺序即作用域内序号。
func currentFooterSpecs(scope FooterScope) ([]FooterSpec, []string) {
	result, errs := currentFooterResult(scope)
	return result.Specs, errs
}

func logFooterErrors(errs []string) {
	for _, e := range errs {
		logger.Warnf("底部按钮配置: %s", e)
	}
}

// InstallFooterHandlers 注册全部作用域的回调分发器; 重复调用只生效一次。
func InstallFooterHandlers() {
	footerOnce.Do(func() {
		for _, scope := range footerScopes {
			for i := 0; i < footerMaxButtons; i++ {
				RegisterCallbackFunc(footerButtonID(scope.slotBase()+i), handleFooterClick)
			}
		}
	})
}

// handleFooterClick 分发底部按钮的点击。
// 回调数据只带「槽位号」, 作用域与真正的行为每次从当前配置里取 —— 所以改配置不需要重新注册处理器。
func handleFooterClick(ctx *context.CallbackContext) error {
	slot, err := strconv.Atoi(strings.TrimSpace(ctx.Data))
	if err != nil {
		return fmt.Errorf("底部按钮回调数据非法: %q", ctx.Data)
	}
	scope, index, ok := footerSlotOf(slot)
	if !ok {
		return fmt.Errorf("底部按钮槽位 %d 不属于任何作用域", slot)
	}
	specs, errs := currentFooterSpecs(scope)
	logFooterErrors(errs)
	if index >= len(specs) {
		return fmt.Errorf("%s底部按钮 #%d 已不在配置里(配置改过?), 忽略本次点击", scope, index)
	}
	spec := specs[index]
	switch spec.Kind {
	case FooterCounter:
		return footerCount(ctx, spec, scope)
	case FooterImage:
		return footerImage(ctx, spec)
	default:
		// command / link 是纯客户端行为, 不会走到回调分发
		return nil
	}
}

// footerCountKey 各作用域的计数键互不影响; 全局沿用旧键名, 已有计数不用迁移。
func footerCountKey(scope FooterScope, spec FooterSpec) string {
	if slug := scope.slug(); slug != "" {
		return "footer." + slug + "." + spec.Key
	}
	return "footer." + spec.Key
}

// footerImage 图片按钮: 点击后把配置里那段 URL 的图发给点击者。
// 不 @ 点击者 —— 图是同一张, 群里谁点的没区别。
func footerImage(ctx *context.CallbackContext, spec FooterSpec) error {
	return ctx.Msg().Image(spec.Arg, spec.Label).Send()
}

// footerCount 计数按钮: 按去重规则决定算不算一次, 然后按模板回复。
func footerCount(ctx *context.CallbackContext, spec FooterSpec, scope FooterScope) error {
	key := footerCountKey(scope, spec)
	bucket, bucketID := ctx.GroupStorage, ctx.GroupId
	if bucketScope := spec.option("scope", "group"); bucketScope == "global" || bucketID == "" {
		bucket, bucketID = ctx.GlobalStorage, "global"
	}
	if bucket == nil {
		return errors.New("底部按钮计数失败: 没有可用的存储命名空间")
	}

	dup := false
	var err error
	switch spec.option("dedup", "user") {
	case "none":
	case "group":
		dup, err = footerMarkOnce(ctx.GroupStorage, key)
	default: // user
		if ctx.UserStorage != nil && ctx.UserId != "" {
			dup, err = footerMarkOnce(ctx.UserStorage, key+"."+bucketID)
		} else {
			logger.Warnf("底部按钮 %q 想按用户去重, 但本次回调没有用户标识, 按不去重处理", spec.Label)
		}
	}
	if err != nil {
		return err
	}

	count := int64(0)
	if dup {
		// 重复点击也要给反馈: 读当前值(键还不存在时按 0 算)
		if _, err := bucket.Get(key, &count); err != nil {
			return err
		}
	} else if count, err = bucket.Incr(key, 1); err != nil {
		return err
	}
	return footerReply(ctx, spec, count, dup)
}

// footerMarkOnce 见过该键返回 true; 第一次见到落标记并返回 false。
func footerMarkOnce(store *storage.Store, key string) (bool, error) {
	if store == nil {
		return false, nil
	}
	seen, err := store.Has(key)
	if err != nil {
		return false, err
	}
	if seen {
		return true, nil
	}
	if err := store.Set(key, true); err != nil {
		return false, err
	}
	return false, nil
}

// footerReply 渲染回复模板并发送: 重复点击走 dup= 文案, 没配就沿用主模板。
func footerReply(ctx *context.CallbackContext, spec FooterSpec, count int64, dup bool) error {
	tmpl := spec.Arg
	if dup {
		if d := spec.option("dup", ""); d != "" {
			tmpl = d
		}
	}
	if strings.TrimSpace(tmpl) == "" {
		tmpl = footerDefaultReply
	}
	return sendFooterReply(ctx, tmpl, map[string]string{
		"count": strconv.FormatInt(count, 10),
		"group": ctx.GroupId,
		"label": spec.Label,
	})
}

// sendFooterReply 把模板拆成「纯文本 / @某人」两种片段依次发送: {{user}} 渲染成真实艾特,
// 其余占位符是纯文本。
func sendFooterReply(ctx *context.CallbackContext, tmpl string, values map[string]string) error {
	builder := ctx.Msg()
	rest := tmpl
	for {
		loc := footerPlaceholderRe.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		if seg := rest[:loc[0]]; seg != "" {
			builder = builder.Text(seg)
		}
		switch name := strings.TrimSpace(rest[loc[2]:loc[3]]); {
		case name == "user" && ctx.UserId != "":
			builder = builder.At(ctx.UserId)
		case name == "user":
			// 没有用户标识就不渲染这个占位符
		default:
			builder = builder.Text(values[name])
		}
		rest = rest[loc[1]:]
	}
	if rest != "" || builder == nil {
		builder = builder.Text(rest)
	}
	return builder.Send()
}
