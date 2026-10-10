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
	"sync/atomic"

	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

// 框架级「底部按钮」: 给每条 Markdown 消息末尾追加一行可配置的按钮。
//
// 配置写在 config.json 的 footer_buttons, 或在管理台「设置」页里一行一个地编辑:
//
//	标签 | 类型 | 参数 | 选项
//
//	+1   | counter | 感谢支持，当前 {{count}} 次 ; dedup=user
//	帮助 | command | /help
//	文档 | link    | https://example.com
//
// 类型: counter(计数) / command(指令) / link(链接); 选项是 k=v, 多个用 ';' 隔开。
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

// FooterKind 底部按钮的行为类型。
type FooterKind string

const (
	// FooterCounter 计数: 点击累加一次并按模板回复。
	FooterCounter FooterKind = "counter"
	// FooterCommand 指令: 点击把指令填进输入框, auto=1 时直接发送。
	FooterCommand FooterKind = "command"
	// FooterLink 链接: 点击跳转。
	FooterLink FooterKind = "link"
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

// FooterParseResult 解析结果: 合法行与逐行错误(带行号)。一行写错不影响其它行。
type FooterParseResult struct {
	Specs  []FooterSpec
	Errors []string
}

// footerReplyPlaceholders 回复模板里允许出现的占位符。
var footerReplyPlaceholders = []string{"count", "user", "group", "label"}

var footerPlaceholderRe = regexp.MustCompile(`\{\{([^}]*)\}\}`)

var footerAllowedOptions = map[FooterKind][]string{
	FooterCounter: {"key", "dedup", "scope", "style", "visited", "dup"},
	FooterCommand: {"key", "auto", "style", "visited"},
	FooterLink:    {"key", "style", "visited"},
}

// ParseFooter 解析底部按钮配置, 一行一个按钮。空行与 # 开头的注释行跳过。
func ParseFooter(lines []string) FooterParseResult {
	var out FooterParseResult
	seenKeys := make(map[string]int, len(lines))
	for i, raw := range lines {
		lineNo := i + 1
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		spec, err := parseFooterLine(text)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("第 %d 行: %v", lineNo, err))
			continue
		}
		if len(out.Specs) >= footerMaxButtons {
			out.Errors = append(out.Errors, fmt.Sprintf("第 %d 行: 底部按钮最多 %d 个, 该行被忽略", lineNo, footerMaxButtons))
			continue
		}
		if spec.Kind == FooterCounter {
			if first, dup := seenKeys[spec.Key]; dup {
				out.Errors = append(out.Errors, fmt.Sprintf(
					"第 %d 行: 计数键 %q 与第 %d 行重复, 用 key= 区分(或删掉一个), 该行被忽略", lineNo, spec.Key, first))
				continue
			}
			seenKeys[spec.Key] = lineNo
		}
		out.Specs = append(out.Specs, spec)
	}
	return out
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
	}
	return "", fmt.Errorf("类型 %q 不认识, 只能是 counter(计数) / command(指令) / link(链接)", raw)
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

// FooterButtons 按解析结果构造底部按钮行; 空配置返回 nil。
func FooterButtons(specs []FooterSpec) []Button {
	out := make([]Button, 0, len(specs))
	for i, spec := range specs {
		btn := Button{
			Id: footerButtonID(i),
			RenderData: RenderData{
				Label:   spec.Label,
				Visited: spec.option("visited", footerDefaultVisited(spec.Kind)),
				Style:   footerStyle(spec.option("style", "blue")),
			},
		}
		switch spec.Kind {
		case FooterCounter:
			btn.SetCallbackWithoutHandle(strconv.Itoa(i))
		case FooterCommand:
			btn.SetAutoCommand(spec.Arg, spec.option("auto", "0") == "1", false)
		case FooterLink:
			btn.SetHref(spec.Arg)
		}
		btn.SetPermission(AllUser)
		btn.SetUnsupportedTip(defaultUnsupportedTip)
		out = append(out, btn)
	}
	return out
}

func footerButtonID(index int) string { return footerIDPrefix + strconv.Itoa(index) }

func footerDefaultVisited(kind FooterKind) string {
	switch kind {
	case FooterCounter:
		return "已投"
	case FooterCommand:
		return "已填入"
	}
	return ""
}

func footerStyle(name string) ButtonStyle {
	if name == "gray" {
		return Gray
	}
	return Blue
}

// FooterMerge 是给 message.SetFooterHook 用的合并函数: 给消息原有键盘追加一行底部按钮。
// 插件自己排好的行不动; 键盘已满时放弃底部按钮并记警告, 保证插件 UI 不被破坏。
func FooterMerge(kb contract.CanMarshal) contract.CanMarshal {
	specs, errs := currentFooterSpecs()
	logFooterErrors(errs)
	if len(specs) == 0 {
		return kb
	}
	row := FooterButtons(specs)
	if kb == nil {
		fresh := &Keyboard{}
		if !AppendRow(fresh, row...) {
			return nil
		}
		return fresh
	}
	existing, ok := kb.(*Keyboard)
	if !ok {
		logger.Warnf("消息上挂的是非 Keyboard 实现的按钮板, 放弃追加底部按钮")
		return kb
	}
	if !AppendRow(existing, row...) {
		logger.Warnf("按钮板已满 %d 行, 放弃追加底部按钮(插件按钮优先)", maxBuilderRows)
	}
	return existing
}

var (
	footerSource atomic.Pointer[func() []string]
	footerOnce   sync.Once
	footerCache  struct {
		sync.Mutex
		ready bool
		raw   string
		specs []FooterSpec
	}
)

// EnableFooter 打开框架级底部按钮: 注入配置来源、注册回调分发器、挂上消息钩子。
// source 每次调用都应返回「当前」配置(通常是 config.Current().FooterButtons), 这样热更立即生效。
func EnableFooter(source func() []string) {
	footerSource.Store(&source)
	InstallFooterHandlers()
	message.SetFooterHook(FooterMerge)
}

// DisableFooter 关掉底部按钮, 主要给测试与运行期停用。
func DisableFooter() {
	footerSource.Store(nil)
	message.SetFooterHook(nil)
}

// currentFooterSpecs 读一次配置并解析; 同一批配置只解析一次, 错误也只报一次, 避免逐条消息刷屏。
func currentFooterSpecs() ([]FooterSpec, []string) {
	src := footerSource.Load()
	if src == nil {
		return nil, nil
	}
	raw := strings.Join((*src)(), "\n")
	footerCache.Lock()
	defer footerCache.Unlock()
	if footerCache.ready && raw == footerCache.raw {
		return footerCache.specs, nil
	}
	result := ParseFooter(strings.Split(raw, "\n"))
	footerCache.ready, footerCache.raw, footerCache.specs = true, raw, result.Specs
	return result.Specs, result.Errors
}

func logFooterErrors(errs []string) {
	for _, e := range errs {
		logger.Warnf("底部按钮配置: %s", e)
	}
}

// InstallFooterHandlers 注册底部按钮的回调分发器; 重复调用只生效一次。
func InstallFooterHandlers() {
	footerOnce.Do(func() {
		for i := 0; i < footerMaxButtons; i++ {
			RegisterCallbackFunc(footerButtonID(i), handleFooterClick)
		}
	})
}

// handleFooterClick 分发底部按钮的点击。
// 回调数据只带「序号」, 真正的行为每次从当前配置里取 —— 所以改配置不需要重新注册处理器。
func handleFooterClick(ctx *context.CallbackContext) error {
	index, err := strconv.Atoi(strings.TrimSpace(ctx.Data))
	if err != nil {
		return fmt.Errorf("底部按钮回调数据非法: %q", ctx.Data)
	}
	specs, errs := currentFooterSpecs()
	logFooterErrors(errs)
	if index < 0 || index >= len(specs) {
		return fmt.Errorf("底部按钮 #%d 已不在配置里(配置改过?), 忽略本次点击", index)
	}
	spec := specs[index]
	if spec.Kind != FooterCounter {
		// command / link 是纯客户端行为, 不会走到回调分发
		return nil
	}
	return footerCount(ctx, spec)
}

// footerCount 计数按钮: 按去重规则决定算不算一次, 然后按模板回复。
func footerCount(ctx *context.CallbackContext, spec FooterSpec) error {
	key := "footer." + spec.Key
	bucket, bucketID := ctx.GroupStorage, ctx.GroupId
	if scope := spec.option("scope", "group"); scope == "global" || bucketID == "" {
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
