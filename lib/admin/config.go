package admin

import (
	"encoding/json"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/gateway"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// coreField 设置页字段元信息, Kind: text|secret|number|bool|intlist|strlist|select|note。
type coreField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Desc    string   `json:"desc,omitempty"`
	Kind    string   `json:"kind"`
	Options []string `json:"options,omitempty"`
	Hot     bool     `json:"hot"`     // 保存即热更
	Restart bool     `json:"restart"` // 需重启生效
	Value   any      `json:"value"`
	Set     bool     `json:"set"` // secret 类: 是否已配置
}

var coreSpecs = []coreField{
	{Key: "port", Label: "监听端口", Kind: "number", Restart: true},
	{Key: "appid", Label: "机器人 AppID", Kind: "text", Restart: true},
	{Key: "secret", Label: "AppSecret", Desc: "不会回显, 留空保持原值", Kind: "secret", Restart: true},
	{Key: "proxy", Label: "API 代理地址", Kind: "text", Restart: true},
	{Key: "database", Label: "SQLite 数据库文件", Kind: "text", Restart: true},
	{Key: "protocol", Label: "协议模式", Kind: "select", Options: []string{"webhook", "websocket"}, Restart: true},
	{Key: "intents", Label: "WebSocket 订阅事件", Desc: "至少选一个；全不选时启动使用默认订阅", Kind: "multiselect", Options: gateway.IntentEvents(), Restart: true},
	{Key: "prefixes", Label: "指令前缀符号", Desc: "多选；取消「无前缀」后 confirm 等无前缀指令需带符号触发", Kind: "multiselect", Options: []string{"!", "/", "#", "无前缀"}, Restart: true},
	{Key: "admin_password", Label: "管理密码", Desc: "留空保持原值; 修改后现有会话立即失效", Kind: "secret", Hot: true},
	{Key: "log_level", Label: "控制台日志级别", Kind: "select", Options: []string{"debug", "info", "warn", "error"}, Hot: true},
	{Key: "global_markdown", Label: "全局 Markdown 回复", Kind: "bool", Hot: true},
	{Key: "markdown_verify_image", Label: "Markdown 图片校验", Desc: "检测 markdown 模板内的图片 URL 是否需走图床", Kind: "bool", Hot: true},
	{Key: "retry_when", Label: "消息重试错误码", Desc: "每行一个业务错误码", Kind: "intlist", Hot: true},
	{Key: "upload_threshold", Label: "分片上传阈值(字节)", Kind: "number", Hot: true},
}

// handleGetConfig 设置页视图: 全部核心字段 + 当前值。
func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Current()
	fields := make([]coreField, 0, len(coreSpecs))
	for _, spec := range coreSpecs {
		field := spec
		applyCoreValue(&field, cfg)
		fields = append(fields, field)
	}
	writeJSON(w, http.StatusOK, H{"fields": fields})
}

func applyCoreValue(field *coreField, cfg config.AppConfig) {
	switch field.Key {
	case "port":
		field.Value = cfg.Port
	case "appid":
		field.Value = cfg.AppId
	case "secret":
		field.Value, field.Set = "", cfg.AppSecret != ""
	case "proxy":
		field.Value = cfg.ProxyAPI
	case "database":
		field.Value = cfg.Database
	case "admin_password":
		field.Value, field.Set = "", cfg.AdminPassword != ""
	case "protocol":
		field.Value = cfg.Protocol
	case "intents":
		field.Value = cfg.Intents
	case "prefixes":
		field.Value = displayPrefixes(cfg.Prefixes)
	case "log_level":
		field.Value = cfg.LogLevel
	case "global_markdown":
		field.Value = cfg.GlobalMarkdown
	case "markdown_verify_image":
		field.Value = cfg.MarkdownVerifyImage
	case "retry_when":
		field.Value = cfg.RetryWhen
	case "upload_threshold":
		field.Value = cfg.UploadThreshold
	}
}

// handlePutConfig 保存核心配置: 稀疏补丁, 校验后写盘并热更。
func handlePutConfig(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Values map[string]json.RawMessage `json:"values"`
		}
		if err := readJSON(r, &input); err != nil || input.Values == nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		specByKey := make(map[string]coreField, len(coreSpecs))
		for _, spec := range coreSpecs {
			specByKey[spec.Key] = spec
		}
		overrides := make(map[string]json.RawMessage, len(input.Values))
		restartChanged := make([]string, 0)
		hotChanged := make([]string, 0)

		for key, raw := range input.Values {
			spec, ok := specByKey[key]
			if !ok {
				writeJSON(w, http.StatusBadRequest, H{"error": "不支持修改的配置项: " + key})
				return
			}
			if string(raw) == "null" {
				continue
			}
			// secret 类留空 = 保留旧值
			if spec.Kind == "secret" {
				var v string
				if err := json.Unmarshal(raw, &v); err != nil {
					writeJSON(w, http.StatusBadRequest, H{"error": key + " 必须为字符串"})
					return
				}
				if v == "" {
					continue
				}
			}
			// 数组类前端按行提交为字符串数组, 数字列表在此转换
			if key == "retry_when" {
				converted, err := toIntSlice(raw)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, H{"error": "retry_when 每行需为整数"})
					return
				}
				raw, _ = json.Marshal(converted)
			}
			if key == "intents" {
				var selected []string
				if err := json.Unmarshal(raw, &selected); err != nil {
					writeJSON(w, http.StatusBadRequest, H{"error": "intents 格式无效"})
					return
				}
				for _, event := range selected {
					if !slices.Contains(gateway.IntentEvents(), event) {
						writeJSON(w, http.StatusBadRequest, H{"error": "未知订阅事件: " + event})
						return
					}
				}
			}
			if key == "prefixes" {
				var selected []string
				if err := json.Unmarshal(raw, &selected); err != nil {
					writeJSON(w, http.StatusBadRequest, H{"error": "prefixes 格式无效"})
					return
				}
				converted := make([]string, 0, len(selected))
				for _, p := range selected {
					if p == "无前缀" {
						converted = append(converted, "")
					} else if slices.Contains([]string{"!", "/", "#"}, p) {
						converted = append(converted, p)
					} else {
						writeJSON(w, http.StatusBadRequest, H{"error": "未知前缀符号: " + p})
						return
					}
				}
				raw, _ = json.Marshal(converted)
			}
			overrides[key] = raw
			if spec.Hot {
				hotChanged = append(hotChanged, key)
			}
			if spec.Restart {
				restartChanged = append(restartChanged, key)
			}
		}
		if len(overrides) == 0 {
			writeJSON(w, http.StatusOK, H{"ok": true, "restart_needed": false})
			return
		}
		if err := config.UpdateCore(overrides); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		next := config.Current()
		if deps.Client != nil && hasAny(hotChanged, "global_markdown", "markdown_verify_image", "retry_when", "upload_threshold") {
			deps.Client.SetMessageOptions(next.GlobalMarkdown, next.MarkdownVerifyImage, next.RetryWhen, next.UploadThreshold)
		}
		if hasAny(hotChanged, "log_level") {
			logx.SetConsoleLevel(next.LogLevel)
		}
		writeJSON(w, http.StatusOK, H{
			"ok":             true,
			"restart_needed": len(restartChanged) > 0,
			"restart_fields": restartChanged,
			"hot_fields":     hotChanged,
		})
	}
}

func toIntSlice(raw json.RawMessage) ([]int, error) {
	var rawList []any
	if err := json.Unmarshal(raw, &rawList); err != nil {
		return nil, err
	}
	out := make([]int, 0, len(rawList))
	for _, item := range rawList {
		switch v := item.(type) {
		case float64:
			out = append(out, int(v))
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		default:
			return nil, strconv.ErrSyntax
		}
	}
	return out, nil
}

func hasAny(haystack []string, needles ...string) bool {
	for _, n := range needles {
		if slices.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// displayPrefixes 配置值转设置页展示值: 空串以「无前缀」呈现。
func displayPrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p == "" {
			out = append(out, "无前缀")
		} else {
			out = append(out, p)
		}
	}
	return out
}
