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

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/contract"
	"github.com/KasumiYuku/Aurorix/lib/message"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

const (
	footerIDPrefix     = "aurorix.footer."
	footerMaxButtons   = 5
	footerOptionSep    = ";"
	footerDefaultReply = "已记录，当前 {{count}} 次。"
)

// FooterScope 标识一组底部按钮的归属; 每个作用域占 footerMaxButtons 个连续槽位, 不同来源的按钮不会串到一起。
type FooterScope int

const (
	// FooterGlobal 全局底部按钮, 挂在每条 Markdown 消息末尾; 场景没单独配时用它。
	FooterGlobal FooterScope = iota
	// FooterNoticeWelcome 入群欢迎专属按钮。
	FooterNoticeWelcome
	// FooterNoticeFarewell 退群提示专属按钮。
	FooterNoticeFarewell
	// FooterSceneGroup 等四个场景的专属按钮: 配了就用它, 没配才回落到 FooterGlobal。
	FooterSceneGroup
	FooterSceneC2C
	FooterSceneChannel
	FooterSceneDM
)

var footerScopes = []FooterScope{
	FooterGlobal, FooterNoticeWelcome, FooterNoticeFarewell,
	FooterSceneGroup, FooterSceneC2C, FooterSceneChannel, FooterSceneDM,
}

// FooterScopeFor 会话场景对应的按钮作用域; 计数与回调按作用域分段, 所以同一个按钮在群和频道里各算各的。
func FooterScopeFor(scene constant.Scene) (FooterScope, bool) {
	switch scene {
	case constant.SceneGroup:
		return FooterSceneGroup, true
	case constant.SceneC2C:
		return FooterSceneC2C, true
	case constant.SceneChannel:
		return FooterSceneChannel, true
	case constant.SceneDirect:
		return FooterSceneDM, true
	}
	return FooterGlobal, false
}

func (s FooterScope) slotBase() int { return int(s) * footerMaxButtons }

func (s FooterScope) slug() string {
	switch s {
	case FooterNoticeWelcome:
		return "notice.welcome"
	case FooterNoticeFarewell:
		return "notice.farewell"
	case FooterSceneGroup:
		return "scene.group"
	case FooterSceneC2C:
		return "scene.c2c"
	case FooterSceneChannel:
		return "scene.channel"
	case FooterSceneDM:
		return "scene.dm"
	}
	return ""
}

func (s FooterScope) String() string {
	switch s {
	case FooterNoticeWelcome:
		return "入群欢迎"
	case FooterNoticeFarewell:
		return "退群提示"
	case FooterSceneGroup:
		return "群聊"
	case FooterSceneC2C:
		return "单聊"
	case FooterSceneChannel:
		return "频道"
	case FooterSceneDM:
		return "频道私信"
	}
	return "全局"
}

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

// FooterParseResult 解析结果: 合法按钮、按配置行分好的键盘行、逐行错误(带行号); 一行写错不影响其它行。
type FooterParseResult struct {
	Specs  []FooterSpec
	Rows   [][]FooterSpec
	Errors []string
}

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

// ParseFooter 解析底部按钮配置, 空行与 # 开头的注释行跳过; 每个配置行对应键盘上的一行按钮。
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

// FooterMerge 给消息原有键盘追加底部按钮行; 场景配了自己的用它的, 没配用全局那套, 键盘满时放弃放不下的行并记警告。
func FooterMerge(kb contract.CanMarshal, originalInput string, scene constant.Scene) contract.CanMarshal {
	scope := FooterGlobal
	if scene, ok := FooterScopeFor(scene); ok {
		if result, _ := currentFooterResult(scene); len(result.Specs) > 0 {
			scope = scene
		}
	}
	result, errs := currentFooterResult(scope)
	logFooterErrors(errs)
	if len(result.Specs) == 0 {
		return kb
	}
	rows := footerButtonRows(result, scope.slotBase(), originalInput)
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

// FooterKeyboard 构造某个作用域专有的按钮板, 该作用域没配时返回 nil; 给入群欢迎这类不该跟全局底部按钮混在一起的消息用。
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

type footerCache struct {
	ready  bool
	raw    string
	result FooterParseResult
}

// EnableFooter 打开框架级底部按钮: 注入配置来源、注册回调分发器、挂消息钩子; source 每次返回当前配置, 热更立即生效。
func EnableFooter(source func() []string) {
	SetFooterSource(FooterGlobal, source)
	message.SetFooterHook(FooterMerge)
}

// SetFooterSource 给某个作用域注入配置来源, 传 nil 表示停用该作用域。
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
		return nil
	}
}

func footerCountKey(scope FooterScope, spec FooterSpec) string {
	if slug := scope.slug(); slug != "" {
		return "footer." + slug + "." + spec.Key
	}
	return "footer." + spec.Key
}

func footerImage(ctx *context.CallbackContext, spec FooterSpec) error {
	return ctx.Msg().Image(spec.Arg, spec.Label).Send()
}

func footerCount(ctx *context.CallbackContext, spec FooterSpec, scope FooterScope) error {
	key := footerCountKey(scope, spec)
	bucket, bucketID := ctx.TargetStorage, ctx.Target.ID
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
		dup, err = footerMarkOnce(ctx.TargetStorage, key)
	default:
		if ctx.UserStorage != nil && ctx.Target.User != "" {
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
		if _, err := bucket.Get(key, &count); err != nil {
			return err
		}
	} else if count, err = bucket.Incr(key, 1); err != nil {
		return err
	}
	return footerReply(ctx, spec, count, dup)
}

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
		"group": ctx.Target.ID,
		"label": spec.Label,
	})
}

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
		case name == "user" && ctx.Target.User != "":
			builder = builder.At(ctx.Target.Mention())
		case name == "user":
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
