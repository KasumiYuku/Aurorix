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
var pluginDisabled atomic.Value

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
	names := make([]string, 0, len(GlobalCommands))
	for name := range GlobalCommands {
		names = append(names, name)
	}
	slices.Sort(names)
	reg.names = names
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
	Mode   string   `json:"mode"`
	Users  []string `json:"users"`
	Groups []string `json:"groups"`
}

type AccessConfig struct {
	Default  AccessRule            `json:"default"`
	Commands map[string]AccessRule `json:"commands"`
	Disabled bool                  `json:"disabled,omitempty"`
}

// Enabled 插件是否启用; 停用后指令与定时任务不再响应。
func Enabled(id string) bool {
	if m, ok := pluginDisabled.Load().(map[string]bool); ok {
		return !m[id]
	}
	return true
}

func refreshDisabledLocked() {
	m := make(map[string]bool, len(pluginAccess))
	for id, access := range pluginAccess {
		m[id] = access.Disabled
	}
	pluginDisabled.Store(m)
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
	defer lock.Unlock()
	var failures []error
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
			if rule.Mode == "off" {
				delete(access.Commands, path)
			}
		}
		pluginAccess[id] = access
	}
	refreshDisabledLocked()
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
		result = append(result, ManagedPlugin{ConfiguredPlugin: base, Commands: commands, Access: cloneAccessConfig(pluginAccess[id]), Console: console})
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
	for path, rule := range prepared.Commands {
		if !validCommands[path] {
			return AccessConfig{}, fmt.Errorf("unknown command path: %s", path)
		}
		if err := validateAccessRule(rule); err != nil {
			return AccessConfig{}, fmt.Errorf("command %s: %w", path, err)
		}
		if rule.Mode == "off" {
			delete(prepared.Commands, path)
		}
	}
	return prepared, nil
}

func ApplyAccessConfiguration(id string, access AccessConfig) {
	lock.Lock()
	defer lock.Unlock()
	pluginAccess[id] = cloneAccessConfig(access)
	refreshDisabledLocked()
	publishRegistryLocked()
}

// CanUse 访问控制判定, 快照读零锁。
func CanUse(pluginID, commandPath, userID, groupID string) bool {
	reg := registry()
	access := reg.access[pluginID]
	rule, overridden := access.Commands[canonicalCommandPath(commandPath)]
	if !overridden {
		rule = access.Default
	}
	userMatched := contains(rule.Users, userID)
	groupMatched := contains(rule.Groups, groupID)
	switch rule.Mode {
	case "whitelist":
		return userMatched || groupMatched
	case "blacklist":
		return !userMatched && !groupMatched
	default:
		return true
	}
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
		if cmd, ok := reg.commands[cand]; ok {
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
	commands := make(map[string]AccessRule, len(access.Commands))
	for path, rule := range access.Commands {
		commands[canonicalCommandPath(path)] = normalizeAccessRule(rule)
	}
	access.Commands = commands
	return access
}

func normalizeAccessRule(rule AccessRule) AccessRule {
	rule.Mode = strings.ToLower(strings.TrimSpace(rule.Mode))
	if rule.Mode == "" {
		rule.Mode = "off"
	}
	rule.Users = cleanIDs(rule.Users)
	rule.Groups = cleanIDs(rule.Groups)
	return rule
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
	return nil
}

func contains(values []string, target string) bool {
	if target == "" {
		return false
	}
	return slices.Contains(values, target)
}

func cloneAccessConfig(source AccessConfig) AccessConfig {
	result := AccessConfig{
		Default:  cloneAccessRule(source.Default),
		Commands: make(map[string]AccessRule, len(source.Commands)),
		Disabled: source.Disabled,
	}
	for path, rule := range source.Commands {
		result.Commands[path] = cloneAccessRule(rule)
	}
	return result
}

func cloneAccessRule(source AccessRule) AccessRule {
	return AccessRule{Mode: source.Mode, Users: append([]string(nil), source.Users...), Groups: append([]string(nil), source.Groups...)}
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
		if field.Type == "password" && (!exists || value == "") {
			value, exists = current[field.Key]
		}
		if !exists {
			if field.Type == "boolean" {
				value = false
			} else {
				value = ""
			}
		}
		if field.Type == "boolean" {
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("field %s must be a boolean", field.Key)
			}
		} else if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("field %s must be a string", field.Key)
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

func ApplyConfiguration(id string, settings map[string]any) error {
	lock.Lock()
	defer lock.Unlock()
	registered, ok := globalPlugins[id]
	if !ok {
		return fmt.Errorf("plugin %s is not registered", id)
	}
	if registered.ApplyConfig != nil {
		if err := registered.ApplyConfig(cloneSettings(settings)); err != nil {
			return err
		}
	}
	pluginSettings[id] = cloneSettings(settings)
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

// 根据前缀获取Command指针
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

// GetCommandCount 获取总指令数
func GetCommandCount() uint {
	return commandCount
}

func defaultCommandHandle(_ *context.MessageContext) error {
	return nil
}
