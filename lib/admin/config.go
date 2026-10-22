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

type coreField struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Desc    string   `json:"desc,omitempty"`
	Kind    string   `json:"kind"`
	Options []string `json:"options,omitempty"`
	Hot     bool     `json:"hot"`
	Restart bool     `json:"restart"`
	Value   any      `json:"value"`
	Set     bool     `json:"set"`
	// Placeholder 输入框里的示例提示(灰色占位文字), 前端直接渲染
	Placeholder string `json:"placeholder,omitempty"`
}

var coreSpecs = []coreField{
	{Key: "port", Label: "监听端口", Kind: "number", Restart: true},
	{Key: "listen_addr", Label: "HTTP 监听地址", Desc: "管理台与 webhook 回调的绑定地址; 留空按协议取默认值(websocket 只绑 127.0.0.1, webhook 绑 0.0.0.0)。改完要重启", Kind: "text", Restart: true},
	{Key: "appid", Label: "机器人 AppID", Kind: "text", Restart: true},
	{Key: "secret", Label: "AppSecret", Desc: "不会回显, 留空保持原值", Kind: "secret", Restart: true},
	{Key: "proxy", Label: "API 代理地址", Kind: "text", Restart: true},
	{Key: "gateway_url", Label: "自定义网关地址", Desc: "填 wss:// 地址可接 webhook 转 websocket 中转站, 留空走官方网关; 仅 websocket 模式生效", Kind: "text", Restart: true},
	{Key: "database", Label: "SQLite 数据库文件", Kind: "text", Restart: true},
	{Key: "protocol", Label: "协议模式", Kind: "select", Options: []string{"webhook", "websocket"}, Restart: true},
	{Key: "intents", Label: "WebSocket 订阅事件", Desc: "至少选一个；全不选时启动使用默认订阅", Kind: "multiselect", Options: gateway.IntentEvents(), Restart: true},
	{Key: "prefixes", Label: "指令前缀符号", Desc: "多选；取消「无前缀」后 confirm 等无前缀指令需带符号触发", Kind: "multiselect", Options: []string{"!", "/", "#", "无前缀"}, Restart: true},
	{Key: "admin_password", Label: "管理密码", Desc: "留空保持原值; 修改后现有会话立即失效", Kind: "secret", Hot: true},
	{Key: "log_level", Label: "控制台日志级别", Kind: "select", Options: []string{"debug", "info", "warn", "error"}, Hot: true},
	{Key: "global_markdown", Label: "全局 Markdown 回复", Kind: "bool", Hot: true},
	{Key: "image_limits_enabled", Label: "限制 Markdown 图片尺寸", Desc: "开: 正文里每张 markdown 图片宽度顶到上限、高度按比例算出后封顶, 只缩不放; 超长图会被压进框里(比例会走样, 点开仍是原图), 换来的是一屏装得下、不再刷屏。关: 原样发出。**省略即开启**", Kind: "bool", Hot: true},
	{Key: "image_max_width", Label: "图片最大宽度(px)", Desc: "聊天气泡宽度; 350 约等于主流手机满宽, 小于它的图不放大。只能填 16..4096, 留空用默认值", Placeholder: "350", Kind: "number", Hot: true},
	{Key: "image_max_height", Label: "图片最大高度(px)", Desc: "单张图在聊天窗里最多占多高; 高度超了就截到这个值, 宽度照样顶满。只能填 16..4096, 留空用默认值", Placeholder: "438", Kind: "number", Hot: true},
	{Key: "retry_when", Label: "消息重试错误码", Desc: "每行一个业务错误码", Kind: "intlist", Hot: true},
	{Key: "upload_threshold", Label: "分片上传阈值(字节)", Kind: "number", Hot: true},
	{Key: "footer_buttons_enabled", Label: "启用底部按钮", Desc: "关掉后保留下面的配置, 但不再往消息里追加", Kind: "bool", Hot: true},
	{Key: "footer_buttons", Label: "底部按钮", Desc: "每个 Markdown 消息末尾追加的底部按钮。一行 = 键盘上一行, 同一行用逗号分隔多个按钮; 每个按钮写 标签|类型|参数, 类型有 counter(计数)/command(指令)/link(链接)/repeat(复读: 把触发这条回复的原消息填进用户输入框)/image(图片: 点击把那一段 URL 的图发出来)/callback(回调: 点击转给参数里那个回调 id, 发什么由注册它的插件决定), 选项跟在参数后: ; k=v。style 可选 0 灰线 / 1 蓝线 / 2 灰底 / 3 蓝底 / 4 文字(不写就是这个)", Placeholder: "+1 | repeat , 文档 | link | https://github.com/KasumiYuku/Aurorix\n帮助 | command | /help ; style=1", Kind: "textlist", Hot: true},
	{Key: "welcome_enabled", Label: "新人进群欢迎", Desc: "收到新人进群事件时发欢迎。能用事件 ID 被动回复, 不占主动消息额度、不受群授权限制。", Kind: "bool", Hot: true},
	{Key: "welcome_messages", Label: "新人进群文案", Desc: "一行一个候选, 发送时随机挑一行; 留空则不发。消息第一行是 ## + 圆形头像 + @进群成员, 文案从第二行开始。", Placeholder: "欢迎来到这里，请随意一点，也请温柔一点。\n欢迎加入，愿你在这里玩得开心。", Kind: "textlist", Hot: true},
	{Key: "farewell_enabled", Label: "成员退群提示", Desc: "收到成员退群事件时发提示。退群不支持被动回复, 只能主动推送; 群需开启「允许主动消息」才会发出。", Kind: "bool", Hot: true},
	{Key: "farewell_messages", Label: "成员退群文案", Desc: "一行一个候选; 留空则不发。退群的人已不在群里、@ 解析不到, 所以只有头像 + 文案。", Placeholder: "有人离开了群聊，祝之后一切顺利。\n短暂同行也很好，下次有缘再见。", Kind: "textlist", Hot: true},
	{Key: "welcome_extra", Label: "新人进群附加内容", Desc: "原样追加在欢迎文案后面, 支持换行、引用块(行首写 >)和内联指令标签。标签里的指令照人话写, 平台要求的 urlencode 由框架代劳 —— 自己先编一遍反而会双重编码。留空则不加", Placeholder: "> <qqbot-cmd-input text=\"/p\" show=\"pgr菜单\" /> <qqbot-cmd-input text=\"/a\" show=\"arc菜单\" />", Kind: "textarea", Hot: true},
	{Key: "welcome_buttons", Label: "新人进群底部按钮", Desc: "语法与上面「底部按钮」完全一致; 只挂在这条欢迎消息上, 不跟全局按钮混, 计数也各算各的。留空则不挂键盘", Placeholder: "帮助 | command | /help ; style=1", Kind: "textlist", Hot: true},
	{Key: "farewell_extra", Label: "成员退群附加内容", Desc: "同「新人进群附加内容」, 追加在退群文案后面, 同样不用自己编码。留空则不加", Kind: "textarea", Hot: true},
	{Key: "farewell_buttons", Label: "成员退群底部按钮", Desc: "语法同「底部按钮」; 只挂在这条退群提示上。留空则不挂键盘", Kind: "textlist", Hot: true},
}

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
	case "listen_addr":
		field.Value = cfg.ListenAddr
	case "appid":
		field.Value = cfg.AppId
	case "secret":
		field.Value, field.Set = "", cfg.AppSecret != ""
	case "proxy":
		field.Value = cfg.ProxyAPI
	case "gateway_url":
		field.Value = cfg.GatewayURL
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
	case "image_limits_enabled":
		field.Value = cfg.ImageBox().On()
	case "image_max_width":
		field.Value = cfg.ImageMaxWidth
	case "image_max_height":
		field.Value = cfg.ImageMaxHeight
	case "retry_when":
		field.Value = cfg.RetryWhen
	case "upload_threshold":
		field.Value = cfg.UploadThreshold
	case "footer_buttons":
		field.Value = cfg.FooterButtons
	case "footer_buttons_enabled":
		// 开关省略时视为启用, 所以 nil 在这里也要显示成打开
		field.Value = cfg.FooterEnabled()
	case "welcome_enabled":
		field.Value = cfg.WelcomeOn()
	case "welcome_messages":
		field.Value = cfg.WelcomeMessages
	case "farewell_enabled":
		field.Value = cfg.FarewellOn()
	case "farewell_messages":
		field.Value = cfg.FarewellMessages
	case "welcome_extra":
		field.Value = cfg.WelcomeExtra
	case "welcome_buttons":
		field.Value = cfg.WelcomeButtons
	case "farewell_extra":
		field.Value = cfg.FarewellExtra
	case "farewell_buttons":
		field.Value = cfg.FarewellButtons
	}
}

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
		if deps.Client != nil && hasAny(hotChanged, "global_markdown", "retry_when", "upload_threshold") {
			deps.Client.SetMessageOptions(next.GlobalMarkdown, next.RetryWhen, next.UploadThreshold)
		}
		if deps.Client != nil {
			deps.Client.SetImageBox(next.ImageBox())
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
