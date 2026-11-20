package admin

import (
	"encoding/json"
	"fmt"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/gateway"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

type coreField struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Desc      string   `json:"desc,omitempty"`
	Kind      string   `json:"kind"`
	Options   []string `json:"options,omitempty"`
	Hot       bool     `json:"hot"`
	Restart   bool     `json:"restart"`
	Value     any      `json:"value"`
	Set       bool     `json:"set"`
	Dedicated bool     `json:"dedicated,omitempty"`
	// Placeholder 输入框里的示例提示(灰色占位文字), 前端直接渲染
	Placeholder string `json:"placeholder,omitempty"`
	// Group 所属功能域, 取值必须是 configGroups 里的 ID; 一屏 36 项靠它分区
	Group string `json:"group"`
	// Unit 数值单位(px / 字节), 只跟在输入框后面显示, 不参与校验
	Unit string `json:"unit,omitempty"`
}

type configGroup struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Desc  string `json:"desc,omitempty"`
}

var configGroups = []configGroup{
	{ID: "maintenance", Label: "维护开关", Desc: "先把 Bot 停下来, 再慢慢排查; 只有这个页面能关掉它。"},
	{ID: "connection", Label: "连接与协议", Desc: "改了要重启进程才生效。"},
	{ID: "console", Label: "管理台", Desc: "本控制台的登录口令, 保存即生效, 旧会话立即失效。"},
	{ID: "logging", Label: "日志", Desc: "控制台输出的最低级别, 保存即生效, 不用重启。"},
	{ID: "message", Label: "消息与图片", Desc: "消息怎么发出去: Markdown、图片尺寸、重试错误码与分片上传。"},
	{ID: "footer", Label: "底部按钮", Desc: "每条消息末尾追加的按钮板, 群聊/单聊/频道/私信可以各写一套。"},
	{ID: "member", Label: "入退群", Desc: "有人进群、有人退群时发什么; 文案留空就不发。"},
}
var coreSpecs = []coreField{
	{Group: "maintenance", Key: "bot_maintenance", Label: "Bot 全局维护", Desc: "打开后所有人(含管理员与主人)都触发不了任何功能, 一律回下面这句话; 只能在本控制台关掉。", Kind: "bool", Hot: true, Dedicated: true},
	{Group: "maintenance", Key: "bot_maintenance_message", Label: "全局维护提示语", Desc: "留空用框架默认话术", Placeholder: constant.DefaultMaintenanceMessage, Kind: "textarea", Hot: true, Dedicated: true},
	{Group: "connection", Key: "port", Label: "监听端口", Kind: "number", Restart: true},
	{Group: "connection", Key: "listen_addr", Label: "HTTP 监听地址", Desc: "管理台与 webhook 回调的绑定地址; 留空按协议取默认值(websocket 只绑 127.0.0.1, webhook 绑 0.0.0.0)。改完要重启", Kind: "text", Restart: true},
	{Group: "connection", Key: "appid", Label: "机器人 AppID", Kind: "text", Restart: true},
	{Group: "connection", Key: "secret", Label: "AppSecret", Desc: "不会回显, 留空保持原值", Kind: "secret", Restart: true},
	{Group: "connection", Key: "proxy", Label: "API 代理地址", Kind: "text", Restart: true},
	{Group: "connection", Key: "gateway_url", Label: "自定义网关地址", Desc: "填 wss:// 地址可接 webhook 转 websocket 中转站, 留空走官方网关; 仅 websocket 模式生效", Kind: "text", Restart: true},
	{Group: "connection", Key: "database", Label: "PostgreSQL 连接串", Kind: "text", Restart: true},
	{Group: "connection", Key: "data_dir", Label: "数据目录", Desc: "本地产物的落盘目录(指令面板快照等), 默认 data", Kind: "text", Restart: true},
	{Group: "connection", Key: "protocol", Label: "协议模式", Kind: "select", Options: []string{"webhook", "websocket"}, Restart: true},
	{Group: "connection", Key: "intents", Label: "订阅事件", Desc: "至少选一个；全不选时启动使用默认订阅。频道事件(GUILD_*/CHANNEL_*/AT_MESSAGE_CREATE/MESSAGE_CREATE/DIRECT_MESSAGE_CREATE)要求协议为 websocket —— 平台要求机器人连着 WebSocket 才能往频道发消息；MESSAGE_CREATE 仅私域机器人有权限，公域只能收 @ 消息", Kind: "multiselect", Options: gateway.IntentEvents(), Restart: true},
	{Group: "connection", Key: "prefixes", Label: "指令前缀符号", Desc: "多选；取消「无前缀」后 confirm 等无前缀指令需带符号触发", Kind: "multiselect", Options: []string{"!", "/", "#", "无前缀"}, Restart: true},
	{Group: "console", Key: "admin_password", Label: "管理密码", Desc: "留空保持原值; 修改后现有会话立即失效", Kind: "secret", Hot: true},
	{Group: "logging", Key: "log_level", Label: "控制台日志级别", Kind: "select", Options: []string{"debug", "info", "warn", "error"}, Hot: true},
	{Group: "message", Key: "global_markdown", Label: "全局 Markdown 回复", Kind: "bool", Hot: true},
	{Group: "message", Key: "image_limits_enabled", Label: "限制 Markdown 图片尺寸", Desc: "开: 正文里每张 markdown 图片宽度顶到上限、高度按比例算出后封顶, 只缩不放; 超长图会被压进框里(比例会走样, 点开仍是原图), 换来的是一屏装得下、不再刷屏。关: 原样发出。**省略即开启**", Kind: "bool", Hot: true},
	{Group: "message", Key: "image_max_width", Label: "图片最大宽度(px)", Desc: "聊天气泡宽度; 350 约等于主流手机满宽, 小于它的图不放大。只能填 16..4096, 留空用默认值", Placeholder: "350", Kind: "number", Unit: "px", Hot: true},
	{Group: "message", Key: "image_max_height", Label: "图片最大高度(px)", Desc: "单张图在聊天窗里最多占多高; 高度超了就截到这个值, 宽度照样顶满。只能填 16..4096, 留空用默认值", Placeholder: "438", Kind: "number", Unit: "px", Hot: true},
	{Group: "message", Key: "retry_when", Label: "消息重试错误码", Desc: "每行一个业务错误码", Kind: "intlist", Hot: true},
	{Group: "message", Key: "upload_threshold", Label: "分片上传阈值(字节)", Kind: "number", Unit: "字节", Hot: true},
	{Group: "message", Key: "keyboard_font_size", Label: "按钮字号", Desc: "所有按钮板(含底部按钮)的缺省字号, 对应报文里的 keyboard.content.style.font_size。选「默认」= 不发这个字段, 用客户端默认大小; 选 small = 小号按钮。它只是缺省值: 插件自己指定过字号的键盘照旧", Kind: "select", Options: []string{"", "small"}, Hot: true},
	{Group: "footer", Key: "footer_buttons_enabled", Label: "启用底部按钮", Desc: "关掉后保留下面的配置, 但不再往消息里追加(含各场景专属按钮)", Kind: "bool", Hot: true},
	{Group: "footer", Key: "footer_buttons", Label: "底部按钮", Desc: "每个 Markdown 消息末尾追加的底部按钮。一行 = 键盘上一行, 同一行用逗号分隔多个按钮; 每个按钮写 标签|类型|参数, 类型有 counter(计数)/command(指令)/link(链接)/repeat(复读: 把触发这条回复的原消息填进用户输入框)/image(图片: 点击把那一段 URL 的图发出来)/callback(回调: 点击转给参数里那个回调 id, 发什么由注册它的插件决定), 选项跟在参数后: ; k=v。style 可选 0 灰线 / 1 蓝线 / 2 灰底 / 3 蓝底 / 4 文字(不写就是这个)", Placeholder: "+1 | repeat , 文档 | link | https://github.com/KasumiYuku/Aurorix\n帮助 | command | /help ; style=1", Kind: "textlist", Hot: true},
	{Group: "footer", Key: "footer_buttons_group", Label: "群聊底部按钮", Desc: "只挂在群消息上; 留空则沿用上面的「底部按钮」。语法与「底部按钮」完全一致", Placeholder: "+1 | repeat , 帮助 | command | /help", Kind: "textlist", Hot: true},
	{Group: "footer", Key: "footer_buttons_c2c", Label: "单聊底部按钮", Desc: "只挂在好友单聊消息上; 留空则沿用上面的「底部按钮」。语法同上", Kind: "textlist", Hot: true},
	{Group: "footer", Key: "footer_buttons_channel", Label: "频道底部按钮", Desc: "只挂在子频道消息上; 留空则沿用上面的「底部按钮」。频道里「复读」按钮能不能用取决于平台, 配之前建议先在测试频道点一次", Kind: "textlist", Hot: true},
	{Group: "footer", Key: "footer_buttons_dm", Label: "频道私信底部按钮", Desc: "只挂在频道私信里; 留空则沿用上面的「底部按钮」。语法同上", Kind: "textlist", Hot: true},
	{Group: "member", Key: "welcome_enabled", Label: "新人进群欢迎", Desc: "收到新人进群事件时发欢迎。能用事件 ID 被动回复, 不占主动消息额度、不受群授权限制。", Kind: "bool", Hot: true},
	{Group: "member", Key: "welcome_messages", Label: "新人进群文案", Desc: "一行一个候选, 发送时随机挑一行; 留空则不发。消息第一行是 ## + 圆形头像 + @进群成员, 文案从第二行开始。", Placeholder: "欢迎来到这里，请随意一点，也请温柔一点。\n欢迎加入，愿你在这里玩得开心。", Kind: "textlist", Hot: true},
	{Group: "member", Key: "farewell_enabled", Label: "成员退群提示", Desc: "收到成员退群事件时发提示。退群不支持被动回复, 只能主动推送; 群需开启「允许主动消息」才会发出。", Kind: "bool", Hot: true},
	{Group: "member", Key: "farewell_messages", Label: "成员退群文案", Desc: "一行一个候选; 留空则不发。退群的人已不在群里、@ 解析不到, 所以只有头像 + 文案。", Placeholder: "有人离开了群聊，祝之后一切顺利。\n短暂同行也很好，下次有缘再见。", Kind: "textlist", Hot: true},
	{Group: "member", Key: "welcome_extra", Label: "新人进群附加内容", Desc: "原样追加在欢迎文案后面, 支持换行、引用块(行首写 >)和内联指令标签。标签里的指令照人话写, 平台要求的 urlencode 由框架代劳 —— 自己先编一遍反而会双重编码。留空则不加", Placeholder: "> <qqbot-cmd-input text=\"/p\" show=\"pgr菜单\" /> <qqbot-cmd-input text=\"/a\" show=\"arc菜单\" />", Kind: "textarea", Hot: true},
	{Group: "member", Key: "welcome_buttons", Label: "新人进群底部按钮", Desc: "语法与上面「底部按钮」完全一致; 只挂在这条欢迎消息上, 不跟全局按钮混, 计数也各算各的。留空则不挂键盘", Placeholder: "帮助 | command | /help ; style=1", Kind: "textlist", Hot: true},
	{Group: "member", Key: "farewell_extra", Label: "成员退群附加内容", Desc: "同「新人进群附加内容」, 追加在退群文案后面, 同样不用自己编码。留空则不加", Kind: "textarea", Hot: true},
	{Group: "member", Key: "farewell_buttons", Label: "成员退群底部按钮", Desc: "语法同「底部按钮」; 只挂在这条退群提示上。留空则不挂键盘", Kind: "textlist", Hot: true},
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.Current()
	fields := make([]coreField, 0, len(coreSpecs))
	for _, spec := range coreSpecs {
		field := spec
		applyCoreValue(&field, cfg)
		fields = append(fields, field)
	}
	writeJSON(w, http.StatusOK, H{"fields": fields, "groups": configGroups})
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
	case "data_dir":
		field.Value = cfg.DataDir
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
	case "bot_maintenance":
		field.Value = cfg.BotMaintenance
	case "bot_maintenance_message":
		field.Value = cfg.BotMaintenanceText
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
	case "footer_buttons_group":
		field.Value = cfg.FooterButtonsGroup
	case "footer_buttons_c2c":
		field.Value = cfg.FooterButtonsC2C
	case "footer_buttons_channel":
		field.Value = cfg.FooterButtonsChannel
	case "footer_buttons_dm":
		field.Value = cfg.FooterButtonsDM
	case "keyboard_font_size":
		field.Value = cfg.KeyboardFontSize
	case "footer_buttons_enabled":
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
		if err := checkChannelNeedsWebSocket(config.Current(), overrides); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
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

func checkChannelNeedsWebSocket(current config.AppConfig, overrides map[string]json.RawMessage) error {
	protocol := current.Protocol
	if raw, ok := overrides["protocol"]; ok {
		if err := json.Unmarshal(raw, &protocol); err != nil {
			return fmt.Errorf("protocol 必须为字符串")
		}
	}
	events := current.Intents
	if raw, ok := overrides["intents"]; ok {
		if err := json.Unmarshal(raw, &events); err != nil {
			return fmt.Errorf("intents 格式无效")
		}
	}
	if protocol != "websocket" && gateway.RequiresWebSocket(events) {
		return fmt.Errorf("订阅了频道事件, 协议必须改成 websocket: 平台要求机器人连着 WebSocket 才能往频道发消息")
	}
	return nil
}
