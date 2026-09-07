package plugin

import (
	"cmp"
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
)

var regLog = logx.New("plugin")

var GlobalCommands map[string]*Command = make(map[string]*Command)
var aliasCommands = make(map[string]*Command)
var globalPlugins = make(map[string]*Plugin)
var pluginSettings = make(map[string]map[string]any)
var pluginAccess = make(map[string]AccessConfig)
var lock sync.RWMutex = sync.RWMutex{}
var commandCount uint = 0
var pluginDisabled atomic.Value // map[string]bool 快照: 停用插件集合, 读路径无锁

// commandIndex 注册表只读快照, 热路径经 atomic 读取零锁。
type commandIndex struct {
	commands map[string]*Command
	aliases  map[string]*Command
	names    []string // 规范名有序, 粘合二分
	access   map[string]AccessConfig
}

var registryStore atomic.Value // *commandIndex

// publishRegistryLocked 重建注册表快照并发布, 热路径读零锁。须持写锁。
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

// normalizeName 剥离注册名携带的前缀符号, 统一以规范名入注册表。
// 注册先于配置加载, 运行时前缀集不可读, 故用固定符号表。
func normalizeName(name string) string {
	for _, p := range []string{"/", "#", "!"} {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			return name[len(p):]
		}
	}
	return name
}

// buildIndex 递归填充名称/别名索引与子指令表。
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

func Register(plugin *Plugin) {
	lock.Lock()
	defer lock.Unlock()
	globalPlugins[plugin.Id] = plugin
	if plugin.TemplateFS != nil {
		if err := templates.RegisterFS(plugin.Id, plugin.TemplateFS, "templates/markdown"); err != nil {
			regLog.Warnf("插件 %s 模板装载失败: %v", plugin.Id, err)
		}
	}
	for _, v := range plugin.Commands {
		if v.Handle == nil {
			v.Handle = defaultCommandHandle
		}
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
	Disabled bool                  `json:"disabled,omitempty"` // 停用整个插件
}

// Enabled 插件是否启用; 停用后指令与定时任务不再响应。
func Enabled(id string) bool {
	if m, ok := pluginDisabled.Load().(map[string]bool); ok {
		return !m[id]
	}
	return true
}

// refreshDisabledLocked 从 pluginAccess 重建停用快照, 须持写锁。
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
}

func LoadConfigurations(settings map[string]map[string]any) error {
	lock.Lock()
	defer lock.Unlock()
	for id, registered := range globalPlugins {
		if len(registered.Config) == 0 {
			continue
		}
		values := cloneSettings(settings[id])
		if registered.ValidateConfig != nil {
			if err := registered.ValidateConfig(values); err != nil {
				return fmt.Errorf("validate configuration for plugin %s: %w", id, err)
			}
		}
		if registered.ApplyConfig != nil {
			if err := registered.ApplyConfig(values); err != nil {
				return fmt.Errorf("apply configuration for plugin %s: %w", id, err)
			}
		}
		pluginSettings[id] = values
	}
	return nil
}

func LoadAccessConfigurations(configs map[string]AccessConfig) error {
	lock.Lock()
	defer lock.Unlock()
	for id, registered := range globalPlugins {
		access := normalizeAccessConfig(configs[id])
		if err := validateAccessRule(access.Default); err != nil {
			return fmt.Errorf("validate access for plugin %s default rule: %w", id, err)
		}
		validCommands := commandPathSet(registered)
		for path, rule := range access.Commands {
			if !validCommands[path] {
				return fmt.Errorf("validate access for plugin %s: unknown command path %s", id, path)
			}
			if err := validateAccessRule(rule); err != nil {
				return fmt.Errorf("validate access for plugin %s command %s: %w", id, path, err)
			}
			if rule.Mode == "off" {
				delete(access.Commands, path)
			}
		}
		pluginAccess[id] = access
	}
	refreshDisabledLocked()
	publishRegistryLocked()
	return nil
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
		result = append(result, ManagedPlugin{ConfiguredPlugin: base, Commands: commands, Access: cloneAccessConfig(pluginAccess[id])})
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
	reg := registryStore.Load().(*commandIndex)
	access := reg.access[pluginID]
	rule, overridden := access.Commands[commandPath]
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

// hasPrefixSymbol 词元是否携带已启用的前缀符号。
func hasPrefixSymbol(token string) bool {
	for _, p := range constant.PrefixChars() {
		if p != "" && strings.HasPrefix(token, p) && len(token) > len(p) {
			return true
		}
	}
	return false
}

// isExactSymbol 词元是否为孤立的已启用符号(如单独一个 "/")。
func isExactSymbol(token string) bool {
	for _, p := range constant.PrefixChars() {
		if p != "" && token == p {
			return true
		}
	}
	return false
}

// matchCommandName 按快照解析词元为根指令: 原词优先, 再逐前缀剥离; allowBare=false 时无符号裸词失配。
func matchCommandName(token string, allowBare bool) (*Command, bool) {
	if !allowBare && !hasPrefixSymbol(token) {
		return nil, false
	}
	reg := registryStore.Load().(*commandIndex)
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

// gluedCommand 带符号词元的粘合匹配: /echoilove you → echo + "ilove you"。
// 仅当词元以已启用符号开头时尝试, 普通口语词不受影响。
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
	reg := registryStore.Load().(*commandIndex)
	// 找最大的规范名前缀: lower_bound 前驱是字典序最大的候选
	idx := sort.SearchStrings(reg.names, name)
	if idx == 0 {
		return nil, "", false
	}
	cand := reg.names[idx-1]
	if !strings.HasPrefix(name, cand) || len(name) <= len(cand) {
		return nil, "", false
	}
	cmd, ok := reg.commands[cand]
	if !ok {
		return nil, "", false
	}
	return cmd, name[len(cand):], true
}

// ResolveRoot 解析首词元为根指令, 三态: 精确/符号孤立/粘合。
// 返回根指令与根指令之后的剩余词元(含粘合残留)。
func ResolveRoot(tokens []string) (*Command, []string, bool) {
	if len(tokens) == 0 {
		return nil, nil, false
	}
	first := tokens[0]
	if isExactSymbol(first) {
		// "/ echo hi": 符号孤立成词, 指令名在下一词元
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
		commands[strings.TrimSpace(path)] = normalizeAccessRule(rule)
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

// NormalizeCommandMsg 按当前启用的前缀符号重写按钮命令文本;
// 首词不是已注册指令时按自定义文字原样透传。
func NormalizeCommandMsg(msg string) string {
	tokens := strings.Fields(msg)
	if len(tokens) < 1 {
		return msg
	}
	if _, ok := MatchCommand(tokens[0]); !ok {
		return msg
	}
	name := canonicalName(tokens[0])
	prefix := ""
	for _, p := range constant.PrefixChars() {
		if p != "" {
			prefix = p
			break
		}
	}
	if prefix != "" {
		tokens[0] = prefix + name
	}
	return strings.Join(tokens, " ")
}

// canonicalName 剥离词元携带的前缀符号, 得到规范名。
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

// 兜底处理函数
func defaultCommandHandle(_ *context.MessageContext) error {
	return nil
}
