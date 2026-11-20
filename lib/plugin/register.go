package plugin

import (
	"cmp"
	"errors"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/templates"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

var regLog = logx.New("plugin")

var GlobalCommands map[string]*Command = make(map[string]*Command)
var aliasCommands = make(map[string]*Command)
var globalPlugins = make(map[string]*Plugin)
var pluginSettings = make(map[string]map[string]any)
var pluginAccess = make(map[string]AccessConfig)
var lock sync.RWMutex = sync.RWMutex{}
var commandCount uint = 0
var pluginStates atomic.Value

type commandIndex struct {
	commands map[string]*Command
	aliases  map[string]*Command
	names    []string
	access   map[string]AccessConfig
}

var registryStore atomic.Value

func registry() *commandIndex {
	if reg, ok := registryStore.Load().(*commandIndex); ok && reg != nil {
		return reg
	}
	return &commandIndex{}
}

func (r *commandIndex) lookup(name string) (*Command, bool) {
	if cmd, ok := r.commands[name]; ok {
		return cmd, true
	}
	cmd, ok := r.aliases[name]
	return cmd, ok
}

func publishRegistryLocked() {
	reg := &commandIndex{
		commands: make(map[string]*Command, len(GlobalCommands)),
		aliases:  make(map[string]*Command, len(aliasCommands)),
		access:   make(map[string]AccessConfig, len(pluginAccess)),
	}
	for k, v := range GlobalCommands {
		reg.commands[k] = v
	}
	for k, v := range aliasCommands {
		reg.aliases[k] = v
	}
	for k, v := range pluginAccess {
		reg.access[k] = v
	}
	names := make([]string, 0, len(GlobalCommands)+len(aliasCommands))
	for name := range GlobalCommands {
		names = append(names, name)
	}
	for name := range aliasCommands {
		names = append(names, name)
	}
	slices.Sort(names)
	reg.names = slices.Compact(names)
	registryStore.Store(reg)
}

func normalizeName(name string) string {
	for _, p := range []string{"/", "#", "!"} {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			return name[len(p):]
		}
	}
	return name
}

func buildIndex(command *Command, pluginId string) {
	command.PluginId = pluginId
	command.Prefix = normalizeName(command.Prefix)
	commandCount++
	if len(command.SubCommand) > 0 {
		command.children = make(map[string]*Command, len(command.SubCommand)*2)
		for _, sub := range command.SubCommand {
			buildIndex(sub, pluginId)
			if _, dup := command.children[sub.Prefix]; dup {
				regLog.Warnf("子指令 %s 重复注册, 被覆盖", sub.Prefix)
			}
			command.children[sub.Prefix] = sub
			for _, alias := range sub.Aliases {
				command.children[normalizeName(alias)] = sub
			}
		}
	}
}

func ensureChildren(command *Command) {
	if command.children != nil || len(command.SubCommand) == 0 {
		return
	}
	command.children = make(map[string]*Command, len(command.SubCommand)*2)
	for _, sub := range command.SubCommand {
		ensureChildren(sub)
		command.children[normalizeName(sub.Prefix)] = sub
		for _, alias := range sub.Aliases {
			command.children[normalizeName(alias)] = sub
		}
	}
}

func canonicalCommandPath(path string) string {
	fields := strings.Fields(path)
	for i, field := range fields {
		fields[i] = normalizeName(field)
	}
	return strings.Join(fields, " ")
}

func Register(plugin *Plugin) {
	lock.Lock()
	defer lock.Unlock()
	globalPlugins[plugin.Id] = plugin
	if plugin.TemplateFS != nil {
		if err := templates.RegisterFS(plugin.Id, plugin.TemplateFS, "templates/markdown"); err != nil {
			regLog.Warnf("插件 %s 模板装载失败: %v", plugin.Id, err)
		}
	}
	if plugin.HTMLTemplateFS != nil {
		if err := templates.RegisterHTMLFS(plugin.Id, plugin.HTMLTemplateFS, "templates/html"); err != nil {
			regLog.Warnf("插件 %s HTML 模板装载失败: %v", plugin.Id, err)
		}
	}
	for _, v := range plugin.Commands {
		buildIndex(v, plugin.Id)
		if existing, ok := GlobalCommands[v.Prefix]; ok {
			regLog.Warnf("指令 %s 与插件 %s 冲突, 由 %s 覆盖", v.Prefix, existing.PluginId, plugin.Id)
		}
		GlobalCommands[v.Prefix] = v
		for _, alias := range v.Aliases {
			key := normalizeName(alias)
			if existing, ok := aliasCommands[key]; ok {
				regLog.Warnf("别名 %s 与插件 %s 冲突, 由 %s 覆盖", alias, existing.PluginId, plugin.Id)
			}
			aliasCommands[key] = v
		}
	}
	publishRegistryLocked()
}

type ConfiguredPlugin struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Fields      []ConfigField  `json:"fields"`
	Values      map[string]any `json:"values"`
}

type AccessRule struct {
	Mode   string                `json:"mode"`
	Users  []string              `json:"users"`
	Groups []string              `json:"groups"`
	Role   constant.RoleRequired `json:"role,omitempty"`
	// Maintenance 这条指令自己的维护开关, 只能加拦, 解不开插件级维护; 默认规则上的维护开关保存时会被清掉。
	Maintenance Maintenance `json:"maintenance"`
	// Deny 是名单拦下人之后怎么回话。静默是默认值, 所以它只表达"要不要回"。
	Deny Deny `json:"deny"`
}

type AccessConfig struct {
	Default     AccessRule            `json:"default"`
	Commands    map[string]AccessRule `json:"commands"`
	Disabled    bool                  `json:"disabled,omitempty"`
	Maintenance Maintenance           `json:"maintenance"`
	// Deny 是插件级的默认回话方式: 指令自己没表态时用它。
	Deny Deny `json:"deny"`
}

type Maintenance struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message,omitempty"`
}

// Deny 名单拦截后的回话方式; Reply 用指针是为了区分"没表态(继承插件级)"与"显式静默"。
type Deny struct {
	Reply *bool `json:"reply,omitempty"`
	// Message 留空时回落到插件级文案, 再回落到 constant.DefaultDeniedMessage。
	Message string `json:"message,omitempty"`
}

func (d Deny) decided() (reply bool, ok bool) {
	if d.Reply == nil {
		return false, false
	}
	return *d.Reply, true
}

type accessState struct {
	disabled    bool
	maintenance bool
	message     string
}

func StateOf(id string) (disabled bool, maintenance string) {
	m, _ := pluginStates.Load().(map[string]accessState)
	state := m[id]
	if state.disabled {
		return true, ""
	}
	if state.maintenance {
		return false, state.message
	}
	return false, ""
}

func Enabled(id string) bool {
	disabled, _ := StateOf(id)
	return !disabled
}

func refreshStatesLocked() {
	m := make(map[string]accessState, len(pluginAccess))
	for id, access := range pluginAccess {
		m[id] = accessState{disabled: access.Disabled, maintenance: access.Maintenance.Enabled, message: access.Maintenance.Message}
	}
	pluginStates.Store(m)
}

type ManagedPlugin struct {
	ConfiguredPlugin
	Commands []string     `json:"commands"`
	Access   AccessConfig `json:"access"`
	Console  string       `json:"console,omitempty"`
}

var denyAllAccessRule = AccessRule{Mode: "whitelist"}

// LoadConfigurations 把每个插件的配置交给它自己校验并生效。
func LoadConfigurations(settings map[string]map[string]any) error {
	lock.Lock()
	var failures []error
	var applied []func()
	for id, registered := range globalPlugins {
		if len(registered.Config) == 0 {
			continue
		}
		values := cloneSettings(settings[id])
		if registered.ValidateConfig != nil {
			if err := registered.ValidateConfig(values); err != nil {
				failures = append(failures, fmt.Errorf("插件 %s 的配置校验未通过, 该插件退回默认值: %w", id, err))
				continue
			}
		}
		if registered.ApplyConfig != nil {
			if err := registered.ApplyConfig(values); err != nil {
				failures = append(failures, fmt.Errorf("插件 %s 的配置生效失败, 该插件退回默认值: %w", id, err))
				continue
			}
		}
		pluginSettings[id] = values
		if registered.OnConfigApplied != nil {
			applied = append(applied, registered.OnConfigApplied)
		}
	}
	lock.Unlock()
	for _, hook := range applied {
		hook()
	}
	return errors.Join(failures...)
}

// LoadAccessConfigurations 与 LoadConfigurations 同一条口径: 一条规则写错不该带走整台机器人。
func LoadAccessConfigurations(configs map[string]AccessConfig) error {
	lock.Lock()
	defer lock.Unlock()
	var failures []error
	for id, registered := range globalPlugins {
		access := normalizeAccessConfig(configs[id])
		if err := validateAccessRule(access.Default); err != nil {
			failures = append(failures, fmt.Errorf("插件 %s 的默认访问规则无效, 该插件已按「全部拒绝」处理: %w", id, err))
			access = AccessConfig{Default: denyAllAccessRule, Disabled: access.Disabled}
		}
		validCommands := commandPathSet(registered)
		for path, rule := range access.Commands {
			if !validCommands[path] {
				failures = append(failures, fmt.Errorf("插件 %s 的访问规则指向了不存在的指令 %s, 已忽略该条", id, path))
				delete(access.Commands, path)
				continue
			}
			if err := validateAccessRule(rule); err != nil {
				failures = append(failures, fmt.Errorf("插件 %s 的指令 %s 访问规则无效, 已忽略该条: %w", id, path, err))
				delete(access.Commands, path)
				continue
			}
			if !ruleActive(rule) {
				delete(access.Commands, path)
			}
		}
		pluginAccess[id] = access
	}
	refreshStatesLocked()
	publishRegistryLocked()
	return errors.Join(failures...)
}

func ConfiguredPlugins() []ConfiguredPlugin {
	lock.RLock()
	defer lock.RUnlock()
	result := make([]ConfiguredPlugin, 0)
	for id, registered := range globalPlugins {
		if len(registered.Config) == 0 {
			continue
		}
		values := cloneSettings(pluginSettings[id])
		for _, field := range registered.Config {
			if field.Type == "password" {
				_, configured := values[field.Key].(string)
				values[field.Key] = configured && values[field.Key] != ""
				continue
			}
			if _, ok := values[field.Key]; !ok && field.Default != nil {
				values[field.Key] = field.Default
			}
		}
		name := registered.Name
		if name == "" {
			name = id
		}
		result = append(result, ConfiguredPlugin{ID: id, Name: name, Description: registered.Description, Fields: registered.Config, Values: values})
	}
	slices.SortFunc(result, func(a, b ConfiguredPlugin) int { return cmp.Compare(a.ID, b.ID) })
	return result
}

func ManagedPlugins() []ManagedPlugin {
	configured := ConfiguredPlugins()
	byID := make(map[string]ConfiguredPlugin, len(configured))
	for _, item := range configured {
		byID[item.ID] = item
	}

	lock.RLock()
	defer lock.RUnlock()
	result := make([]ManagedPlugin, 0, len(globalPlugins))
	for id, registered := range globalPlugins {
		base, ok := byID[id]
		if !ok {
			name := registered.Name
			if name == "" {
				name = id
			}
			base = ConfiguredPlugin{ID: id, Name: name, Description: registered.Description, Fields: []ConfigField{}, Values: map[string]any{}}
		}
		commands := make([]string, 0)
		for _, command := range registered.Commands {
			collectCommandPaths(command, command.Prefix, &commands)
		}
		slices.Sort(commands)
		console := ""
		if registered.WebUI != nil && registered.WebUI.SPA != nil {
			console = "/" + id + "/"
		}
		result = append(result, ManagedPlugin{ConfiguredPlugin: base, Commands: commands, Access: normalizeAccessConfig(pluginAccess[id]), Console: console})
	}
	slices.SortFunc(result, func(a, b ManagedPlugin) int { return cmp.Compare(a.ID, b.ID) })
	return result
}

func ManagedPluginByID(id string) (ManagedPlugin, bool) {
	for _, managed := range ManagedPlugins() {
		if managed.ID == id {
			return managed, true
		}
	}
	return ManagedPlugin{}, false
}

func PrepareAccessConfiguration(id string, access AccessConfig) (AccessConfig, error) {
	lock.RLock()
	registered, ok := globalPlugins[id]
	lock.RUnlock()
	if !ok {
		return AccessConfig{}, fmt.Errorf("plugin %s is not registered", id)
	}

	validCommands := commandPathSet(registered)
	prepared := normalizeAccessConfig(access)
	if err := validateAccessRule(prepared.Default); err != nil {
		return AccessConfig{}, fmt.Errorf("default rule: %w", err)
	}
	if err := checkWhitelistNotEmpty("默认规则", prepared.Default); err != nil {
		return AccessConfig{}, err
	}
	for path, rule := range prepared.Commands {
		if !validCommands[path] {
			return AccessConfig{}, fmt.Errorf("unknown command path: %s", path)
		}
		if err := validateAccessRule(rule); err != nil {
			return AccessConfig{}, fmt.Errorf("command %s: %w", path, err)
		}
		if err := checkWhitelistNotEmpty("指令 "+path, rule); err != nil {
			return AccessConfig{}, err
		}
		if !ruleActive(rule) {
			delete(prepared.Commands, path)
		}
	}
	return prepared, nil
}

func checkWhitelistNotEmpty(where string, rule AccessRule) error {
	if rule.Mode != "whitelist" || len(rule.Users) > 0 || len(rule.Groups) > 0 {
		return nil
	}
	return fmt.Errorf("%s 选了白名单但名单是空的, 保存后所有人都用不了 —— 白名单至少要有一个群或一个用户", where)
}

func ApplyAccessConfiguration(id string, access AccessConfig) {
	lock.Lock()
	defer lock.Unlock()
	pluginAccess[id] = cloneAccessConfig(access)
	refreshStatesLocked()
	publishRegistryLocked()
}

type AccessQuery struct {
	PluginID    string
	CommandPath string
	UserID      string
	// ConversationID 会话 ID: 群 openid / 用户 openid / 子频道 ID / 频道 ID。
	ConversationID string
	Role           constant.RoleRequired
}

type Verdict struct {
	Allowed     bool
	Maintenance string
	// Denied 非空表示名单拦下了这条指令, 且调用方要求回一句。空表示静默。
	Denied string
}

// Check 一次判定给出完整结论: 放不放行、被维护拦下时回什么话、被名单拦下要不要回话。维护两层只能加拦不能解拦, 对管理员与主人一律豁免。
func Check(q AccessQuery) Verdict {
	access := registry().access[q.PluginID]
	if access.Disabled {
		return Verdict{}
	}
	exempt := constant.RoleAdmin.CanUse(q.Role)
	if access.Maintenance.Enabled && !exempt {
		return Verdict{Maintenance: access.Maintenance.Message}
	}
	rule, overridden := access.Commands[canonicalCommandPath(q.CommandPath)]
	if !overridden {
		rule = access.Default
	}
	if overridden && rule.Maintenance.Enabled && !exempt {
		return Verdict{Maintenance: commandMaintenanceMessage(rule.Maintenance, access.Maintenance)}
	}
	if !rule.Role.CanUse(q.Role) {
		return Verdict{}
	}
	userMatched := contains(rule.Users, q.UserID)
	groupMatched := contains(rule.Groups, q.ConversationID)
	allowed := true
	switch rule.Mode {
	case "whitelist":
		allowed = userMatched || groupMatched
	case "blacklist":
		allowed = !userMatched && !groupMatched
	}
	if allowed {
		return Verdict{Allowed: true}
	}
	return Verdict{Denied: denyMessage(rule.Deny, access.Deny)}
}

func CanUse(q AccessQuery) bool {
	return Check(q).Allowed
}

func commandMaintenanceMessage(command, plugin Maintenance) string {
	if command.Message != "" {
		return command.Message
	}
	if plugin.Message != "" {
		return plugin.Message
	}
	return constant.DefaultMaintenanceMessage
}

func denyMessage(command, plugin Deny) string {
	reply, decided := command.decided()
	if !decided {
		reply, _ = plugin.decided()
	}
	if !reply {
		return ""
	}
	if command.Message != "" {
		return command.Message
	}
	if plugin.Message != "" {
		return plugin.Message
	}
	return constant.DefaultDeniedMessage
}

func hasPrefixSymbol(token string) bool {
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(token, p) && len(token) > len(p) {
			return true
		}
	}
	return false
}

func isExactSymbol(token string) bool {
	for _, p := range constant.PrefixChars() {
		if p != "" && token == p {
			return true
		}
	}
	return false
}

func matchCommandName(token string, allowBare bool) (*Command, bool) {
	if !allowBare && !hasPrefixSymbol(token) {
		return nil, false
	}
	reg := registry()
	if cmd, ok := reg.commands[token]; ok {
		return cmd, true
	}
	if cmd, ok := reg.aliases[token]; ok {
		return cmd, true
	}
	for _, p := range constant.PrefixChars() {
		if p == "" || !strings.HasPrefix(token, p) || len(token) <= len(p) {
			continue
		}
		if cmd, ok := reg.commands[token[len(p):]]; ok {
			return cmd, true
		}
		if cmd, ok := reg.aliases[token[len(p):]]; ok {
			return cmd, true
		}
	}
	return nil, false
}

// MatchCommand 兼容入口: 按当前无前缀开关解析单个词元。
func MatchCommand(token string) (*Command, bool) {
	return matchCommandName(token, constant.HasBarePrefix())
}

func gluedCommand(token string) (*Command, string, bool) {
	prefix := ""
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(token, p) && len(token) > len(p) {
			prefix = p
			break
		}
	}
	if prefix == "" {
		return nil, "", false
	}
	name := token[len(prefix):]
	if name == "" {
		return nil, "", false
	}
	reg := registry()
	idx := sort.SearchStrings(reg.names, name)
	for i := idx - 1; i >= 0; i-- {
		cand := reg.names[i]
		if !strings.HasPrefix(name, cand) {
			continue
		}
		if len(name) <= len(cand) {
			continue
		}
		if cmd, ok := reg.lookup(cand); ok {
			return cmd, name[len(cand):], true
		}
	}
	return nil, "", false
}

// ResolveRoot 解析首词元为根指令, 三态: 精确/符号孤立/粘合。
func ResolveRoot(tokens []string) (*Command, []string, bool) {
	if len(tokens) == 0 {
		return nil, nil, false
	}
	first := tokens[0]
	if isExactSymbol(first) {
		if len(tokens) < 2 {
			return nil, nil, false
		}
		cmd, ok := matchCommandName(tokens[1], true)
		if !ok {
			return nil, nil, false
		}
		return cmd, tokens[2:], true
	}
	if cmd, ok := matchCommandName(first, constant.HasBarePrefix()); ok {
		return cmd, tokens[1:], true
	}
	if cmd, rest, ok := gluedCommand(first); ok {
		tail := make([]string, 0, len(tokens))
		if rest != "" {
			tail = append(tail, rest)
		}
		tail = append(tail, tokens[1:]...)
		return cmd, tail, true
	}
	return nil, nil, false
}

// ResolveCommandPath 原始消息解析出的规范路径, 供访问控制展示。
func ResolveCommandPath(root *Command, raw string) string {
	tokens := strings.Fields(raw)
	if len(tokens) < 2 {
		return root.Prefix
	}
	_, path, _ := Resolve(root, tokens[1:])
	return path
}

// Resolve 沿子指令树走到叶子, tokens 为首词元之后的剩余词元。
func Resolve(root *Command, tokens []string) (*Command, string, []string) {
	ensureChildren(root)
	path := root.Prefix
	current := root
	i := 0
	for i < len(tokens) && len(current.children) > 0 {
		var next *Command
		if sub, ok := current.children[tokens[i]]; ok {
			next = sub
		} else {
			for _, p := range constant.PrefixChars() {
				if p == "" || !strings.HasPrefix(tokens[i], p) || len(tokens[i]) <= len(p) {
					continue
				}
				if sub, ok := current.children[tokens[i][len(p):]]; ok {
					next = sub
					break
				}
			}
		}
		if next == nil {
			break
		}
		path += " " + next.Prefix
		current = next
		i++
	}
	return current, path, tokens[i:]
}

func collectCommandPaths(command *Command, path string, result *[]string) {
	*result = append(*result, path)
	for _, subcommand := range command.SubCommand {
		collectCommandPaths(subcommand, path+" "+subcommand.Prefix, result)
	}
}

func commandPathSet(registered *Plugin) map[string]bool {
	result := make(map[string]bool)
	for _, command := range registered.Commands {
		paths := make([]string, 0)
		collectCommandPaths(command, command.Prefix, &paths)
		for _, path := range paths {
			result[path] = true
		}
	}
	return result
}

func normalizeAccessConfig(access AccessConfig) AccessConfig {
	access.Default = normalizeAccessRule(access.Default)
	access.Default.Maintenance = Maintenance{}
	commands := make(map[string]AccessRule, len(access.Commands))
	for path, rule := range access.Commands {
		commands[canonicalCommandPath(path)] = normalizeAccessRule(rule)
	}
	access.Commands = commands
	access.Maintenance = normalizeMaintenance(access.Maintenance)
	access.Deny = normalizeDeny(access.Deny)
	return access
}

func normalizeMaintenance(m Maintenance) Maintenance {
	m.Message = strings.TrimSpace(m.Message)
	if m.Enabled && m.Message == "" {
		m.Message = constant.DefaultMaintenanceMessage
	}
	if !m.Enabled {
		m.Message = ""
	}
	return m
}

func normalizeAccessRule(rule AccessRule) AccessRule {
	rule.Mode = strings.ToLower(strings.TrimSpace(rule.Mode))
	if rule.Mode == "" {
		rule.Mode = "off"
	}
	rule.Role = constant.RoleRequired(strings.ToLower(strings.TrimSpace(string(rule.Role))))
	rule.Users = cleanIDs(rule.Users)
	rule.Groups = cleanIDs(rule.Groups)
	rule.Maintenance = normalizeCommandMaintenance(rule.Maintenance)
	if rule.Mode == "off" {
		rule.Deny = Deny{}
	} else {
		rule.Deny = normalizeDeny(rule.Deny)
	}
	return rule
}

func normalizeCommandMaintenance(m Maintenance) Maintenance {
	if !m.Enabled {
		return Maintenance{}
	}
	return Maintenance{Enabled: true, Message: strings.TrimSpace(m.Message)}
}

const maxDenyMessage = 300

func normalizeDeny(d Deny) Deny {
	d.Message = truncateRunes(strings.TrimSpace(d.Message), maxDenyMessage)
	if d.Message != "" && d.Reply == nil {
		reply := true
		d.Reply = &reply
	}
	if d.Reply != nil && !*d.Reply {
		d.Message = ""
	}
	return d
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func ruleActive(rule AccessRule) bool {
	return (rule.Mode != "" && rule.Mode != "off") || rule.Role != "" || rule.Maintenance.Enabled
}

// HasListenerRule 报告默认规则是否给消息监听这类入口加了约束; 没有约束时总线连身份都不必去查。
func HasListenerRule(id string) bool {
	access := registry().access[id]
	return access.Maintenance.Enabled || ruleActive(access.Default)
}

func cleanIDs(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return result
}

func validateAccessRule(rule AccessRule) error {
	if rule.Mode != "off" && rule.Mode != "whitelist" && rule.Mode != "blacklist" {
		return fmt.Errorf("mode must be off, whitelist, or blacklist")
	}
	switch rule.Role {
	case "", constant.RoleOwner, constant.RoleAdmin, constant.RoleMember:
		return nil
	}
	return fmt.Errorf("role must be empty, owner, admin, or member")
}

func contains(values []string, target string) bool {
	if target == "" {
		return false
	}
	return slices.Contains(values, target)
}

func cloneAccessConfig(source AccessConfig) AccessConfig {
	result := AccessConfig{
		Default:     cloneAccessRule(source.Default),
		Commands:    make(map[string]AccessRule, len(source.Commands)),
		Disabled:    source.Disabled,
		Maintenance: source.Maintenance,
		Deny:        cloneDeny(source.Deny),
	}
	for path, rule := range source.Commands {
		result.Commands[path] = cloneAccessRule(rule)
	}
	return result
}

func cloneAccessRule(source AccessRule) AccessRule {
	return AccessRule{
		Mode:        source.Mode,
		Users:       append([]string(nil), source.Users...),
		Groups:      append([]string(nil), source.Groups...),
		Role:        source.Role,
		Maintenance: source.Maintenance,
		Deny:        cloneDeny(source.Deny),
	}
}

func cloneDeny(source Deny) Deny {
	if source.Reply == nil {
		return source
	}
	reply := *source.Reply
	source.Reply = &reply
	return source
}

func PrepareConfiguration(id string, input map[string]any) (map[string]any, error) {
	lock.RLock()
	defer lock.RUnlock()
	registered, ok := globalPlugins[id]
	if !ok || len(registered.Config) == 0 {
		return nil, fmt.Errorf("plugin %s has no configurable options", id)
	}
	current := pluginSettings[id]
	prepared := make(map[string]any, len(registered.Config))
	for _, field := range registered.Config {
		value, exists := input[field.Key]
		if field.Type == "password" {
			if s, ok := value.(string); !ok || s == "" {
				value, exists = current[field.Key]
			}
		}
		if !exists {
			if field.Type == "boolean" {
				value = false
			} else {
				value = ""
			}
		}
		switch field.Type {
		case "boolean":
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("field %s must be a boolean", field.Key)
			}
		case "number":
			normalized, ok := normalizeNumber(value)
			if !ok {
				return nil, fmt.Errorf("field %s must be a number", field.Key)
			}
			value = normalized
		default:
			if _, ok := value.(string); !ok {
				return nil, fmt.Errorf("field %s must be a string", field.Key)
			}
		}
		prepared[field.Key] = value
	}
	if registered.ValidateConfig != nil {
		if err := registered.ValidateConfig(cloneSettings(prepared)); err != nil {
			return nil, err
		}
	}
	return prepared, nil
}

func normalizeNumber(value any) (any, bool) {
	switch v := value.(type) {
	case nil:
		return "", true
	case float64, int, int64:
		return v, true
	case string:
		return v, true
	}
	return nil, false
}

func ApplyConfiguration(id string, settings map[string]any) error {
	lock.Lock()
	registered, ok := globalPlugins[id]
	if !ok {
		lock.Unlock()
		return fmt.Errorf("plugin %s is not registered", id)
	}
	if registered.ApplyConfig != nil {
		if err := registered.ApplyConfig(cloneSettings(settings)); err != nil {
			lock.Unlock()
			return err
		}
	}
	pluginSettings[id] = cloneSettings(settings)
	hook := registered.OnConfigApplied
	lock.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func cloneSettings(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

// RegisteredCount 已注册插件数 (含无配置项的)。
func RegisteredCount() int {
	lock.RLock()
	defer lock.RUnlock()
	return len(globalPlugins)
}

func GetCommand(prefix string) (*Command, bool) {
	lock.RLock()
	defer lock.RUnlock()
	cmd, ok := GlobalCommands[prefix]
	return cmd, ok
}

// NormalizeCommandMsg 按当前启用的前缀符号重写按钮命令文本
func NormalizeCommandMsg(msg string) string {
	token, start, end := firstToken(msg)
	if token == "" {
		return msg
	}
	if _, ok := MatchCommand(token); !ok {
		return msg
	}
	prefix := ""
	for _, p := range constant.PrefixChars() {
		if p != "" {
			prefix = p
			break
		}
	}
	if prefix == "" {
		return msg
	}
	return msg[:start] + prefix + canonicalName(token) + msg[end:]
}

func firstToken(s string) (token string, start, end int) {
	start = strings.IndexFunc(s, func(r rune) bool { return !unicode.IsSpace(r) })
	if start < 0 {
		return "", 0, 0
	}
	if i := strings.IndexFunc(s[start:], unicode.IsSpace); i < 0 {
		return s[start:], start, len(s)
	} else {
		return s[start : start+i], start, start + i
	}
}

func canonicalName(token string) string {
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(token, p) && len(token) > len(p) {
			return token[len(p):]
		}
	}
	return token
}

func GetCommandCount() uint {
	return commandCount
}

func defaultCommandHandle(_ *context.MessageContext) error {
	return nil
}
