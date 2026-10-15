package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/KasumiYuku/Aurorix/lib/requests"
)

// 指令面板 = 用户在输入框旁边看到的那个命令菜单。它存在 QQ 服务端, 只能走 OpenAPI。
//
// 官方把「面板/条目数量超限」和「字段显示宽度超限」塞进同一个错误码 30013, 只看码
// 必然误判(21 个汉字按字符数没超, 按显示宽度 43 就超了), 所以面板域自己带一张错误码
// 表, 并把响应里的 limit 一起透出去。
const (
	PanelNameMaxWidth = 14
	PanelDescMaxWidth = 30
	PanelRemarkMaxLen = 255
	PanelMaxItems     = 20
	// PanelMaxPerBot 全账号面板总数上限, 四个场景合计。
	PanelMaxPerBot = 20
)

// PanelScope 面板场景。
type PanelScope string

const (
	PanelScopeC2C     PanelScope = "c2c"
	PanelScopeGroup   PanelScope = "group"
	PanelScopeChannel PanelScope = "channel"
	PanelScopeDM      PanelScope = "dm"
)

// PanelScopes 四个场景, 顺序即前端的标签页顺序("群聊"在前, 因为它是主战场)。
var PanelScopes = []PanelScope{PanelScopeGroup, PanelScopeC2C, PanelScopeChannel, PanelScopeDM}

// PanelScopeLabels 场景中文名, 前后端共用一份口径。
var PanelScopeLabels = map[PanelScope]string{
	PanelScopeGroup:   "群聊",
	PanelScopeC2C:     "私聊",
	PanelScopeChannel: "频道",
	PanelScopeDM:      "频道私信",
}

const (
	PanelItemCommand = "command"
	PanelItemLink    = "link"
)

// PanelTargetAll 目前只支持 all; specific(按群/按人下发)留作以后。
const PanelTargetAll = "all"

// ValidPanelScope 场景白名单。
func ValidPanelScope(scope PanelScope) bool {
	_, ok := PanelScopeLabels[scope]
	return ok
}

// PanelItem 面板里的一条指令。
type PanelItem struct {
	Name      string `json:"name"`
	Desc      string `json:"desc"`
	Type      string `json:"type"`
	OnlyAdmin bool   `json:"only_admin,omitempty"`
	Link      string `json:"link,omitempty"`
}

// Panel 面板内容。items 的顺序就是用户看到的顺序, 服务端没有单独的排序字段。
type Panel struct {
	Items   []PanelItem `json:"items"`
	Remark  string      `json:"remark,omitempty"`
	Version int         `json:"version,omitempty"`
}

// PanelRecord 服务端返回的面板记录(含归属信息)。
type PanelRecord struct {
	PanelID      string     `json:"panel_id"`
	Scope        PanelScope `json:"scope"`
	TargetType   string     `json:"target_type"`
	Panel        Panel      `json:"panel"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	Version      int        `json:"version"`
	UserOpenIDs  []string   `json:"user_openids,omitempty"`
	GroupOpenIDs []string   `json:"group_openids,omitempty"`
}

type panelPage struct {
	Records    []PanelRecord `json:"records"`
	NextCursor string        `json:"next_cursor"`
	IsEnd      bool          `json:"is_end"`
}

// PanelDisplayWidth 按显示宽度计量: 非 ASCII 记 2。
//
// 这是官方统计口径, 不是字符数。desc="解析哔哩哔哩视频，转换为MP3或FLAC格式音频" 只有
// 21 个字符, 宽度 43, 超上限 30, 服务端直接拒。
func PanelDisplayWidth(s string) int {
	width := 0
	for _, r := range s {
		if r < 128 {
			width++
		} else {
			width += 2
		}
	}
	return width
}

// ValidatePanel 提交前的本地校验, 规则与 QQ 服务端一致。
//
// 有意义的地方在于报错能说清是哪个字段: 交给服务端只会回一句 30013。
func ValidatePanel(p Panel) error {
	if len(p.Items) > PanelMaxItems {
		return fmt.Errorf("指令项 %d 条, 超过上限 %d", len(p.Items), PanelMaxItems)
	}
	if n := utf8.RuneCountInString(p.Remark); n > PanelRemarkMaxLen {
		return fmt.Errorf("备注 %d 个字符, 超过上限 %d", n, PanelRemarkMaxLen)
	}
	for i, item := range p.Items {
		where := fmt.Sprintf("第 %d 项", i+1)
		if strings.TrimSpace(item.Name) == "" {
			return fmt.Errorf("%s 的名字不能为空", where)
		}
		if strings.TrimSpace(item.Desc) == "" {
			return fmt.Errorf("%s 的描述不能为空", where)
		}
		if w := PanelDisplayWidth(item.Name); w > PanelNameMaxWidth {
			return fmt.Errorf("%s 名字「%s」显示宽度 %d, 超过上限 %d(中文与全角标点各计 2)", where, item.Name, w, PanelNameMaxWidth)
		}
		if w := PanelDisplayWidth(item.Desc); w > PanelDescMaxWidth {
			return fmt.Errorf("%s 描述「%s」显示宽度 %d, 超过上限 %d(中文与全角标点各计 2)", where, item.Desc, w, PanelDescMaxWidth)
		}
		switch item.Type {
		case PanelItemCommand:
		case PanelItemLink:
			if !strings.HasPrefix(item.Link, "https://") {
				return fmt.Errorf("%s 是链接类型, link 必须以 https:// 开头(当前 %q)", where, item.Link)
			}
		default:
			return fmt.Errorf("%s 的类型 %q 不认识, 只能是 %s 或 %s", where, item.Type, PanelItemCommand, PanelItemLink)
		}
	}
	return nil
}

// PanelError 面板接口的业务错误。HTTP 是状态码, Code 是 QQ 业务码, Limit 是服务端回传的真实上限。
type PanelError struct {
	HTTP    int    `json:"http"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Limit   int    `json:"limit"`
	Reason  string `json:"reason"`
}

func (e *PanelError) Error() string {
	switch {
	case e.Reason != "" && e.Message != "":
		return fmt.Sprintf("%s: %s", e.Reason, e.Message)
	case e.Reason != "":
		return e.Reason
	case e.Message != "":
		return fmt.Sprintf("面板接口错误码 %d: %s", e.Code, e.Message)
	default:
		return fmt.Sprintf("面板接口返回 HTTP %d", e.HTTP)
	}
}

// panelHints 面板域错误码。官方文档写 40030013, 实测返回 30013 —— 同一件事两种写法, 统一按低位查。
var panelHints = map[int]string{
	30006: "面板不存在(可能已在别处被删除)",
	30008: "链接格式错误",
	30009: "该面板正在操作中, 请稍后重试",
	30011: "场景参数不合法",
	30012: "目标类型不合法",
	30013: "数量或字段宽度超出限制",
	30015: "指令项类型不合法",
	30016: "必填字段缺失",
	30018: "当前场景不支持该操作",
	30020: "内容存在安全风险",
	30021: "全局面板不支持指定关联",
}

func normPanelCode(code int) int {
	if code >= 40000000 {
		return code % 100000
	}
	return code
}

func describePanelCode(code, limit int) string {
	hint, ok := panelHints[code]
	if !ok {
		return ""
	}
	if code == 30013 && limit > 0 {
		return fmt.Sprintf("%s(服务端上限 %d)", hint, limit)
	}
	return hint
}

func newPanelError(httpCode int, body []byte) *PanelError {
	e := &PanelError{HTTP: httpCode}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Limit   int    `json:"limit"`
		Data    struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Limit   int    `json:"limit"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &resp) == nil {
		e.Code, e.Message, e.Limit = resp.Code, resp.Message, resp.Limit
		if e.Code == 0 && resp.Data.Code != 0 {
			e.Code, e.Message, e.Limit = resp.Data.Code, resp.Data.Message, resp.Data.Limit
		}
	}
	e.Reason = describePanelCode(normPanelCode(e.Code), e.Limit)
	return e
}

// PanelAPI 指令面板域。
type PanelAPI struct {
	api *BotAPI
}

func (p *PanelAPI) call(method, url string, body requests.Body) ([]byte, error) {
	raw, err := p.api.do(method, url, body)
	if err != nil {
		var se *requests.StatusError
		if errors.As(err, &se) {
			return nil, newPanelError(se.Code, se.Body)
		}
		return nil, err
	}
	return raw, nil
}

// List 拉某个场景的全部面板(内部翻页, 上游单页最多 50)。
func (p *PanelAPI) List(scope PanelScope) ([]PanelRecord, error) {
	if !ValidPanelScope(scope) {
		return nil, fmt.Errorf("场景 %q 不认识", scope)
	}
	var out []PanelRecord
	cursor := ""
	for page := 0; page < 40; page++ {
		query := url.Values{}
		query.Set("scope", string(scope))
		query.Set("limit", "50")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		raw, err := p.call("GET", p.api.ProxyAPI+"/v2/panels?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp panelPage
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("解析面板列表失败: %w", err)
		}
		out = append(out, resp.Records...)
		if resp.IsEnd || resp.NextCursor == "" {
			return out, nil
		}
		cursor = resp.NextCursor
	}
	return out, nil
}

// Count 全账号面板总数(跨四个场景合计)。新建前用它挡上限。
func (p *PanelAPI) Count() (int, error) {
	total := 0
	for _, scope := range PanelScopes {
		records, err := p.List(scope)
		if err != nil {
			return 0, err
		}
		total += len(records)
	}
	return total, nil
}

// Get 取单个面板详情。
func (p *PanelAPI) Get(panelID string) (*PanelRecord, error) {
	raw, err := p.call("GET", p.api.ProxyAPI+"/v2/panels/"+url.PathEscape(panelID), nil)
	if err != nil {
		return nil, err
	}
	var record PanelRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("解析面板详情失败: %w", err)
	}
	return &record, nil
}

// Create 新建面板, 返回服务端分配的 panel_id。内容整份提交, 没有按条目增删的接口。
func (p *PanelAPI) Create(scope PanelScope, panel Panel) (string, error) {
	if !ValidPanelScope(scope) {
		return "", fmt.Errorf("场景 %q 不认识", scope)
	}
	if err := ValidatePanel(panel); err != nil {
		return "", err
	}
	body := map[string]any{"scope": string(scope), "target_type": PanelTargetAll, "panel": panel}
	raw, err := p.call("POST", p.api.ProxyAPI+"/v2/panels", requests.JSON(body))
	if err != nil {
		return "", err
	}
	var resp struct {
		PanelID string `json:"panel_id"`
		Data    struct {
			PanelID string `json:"panel_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("解析新建结果失败: %w", err)
	}
	if resp.PanelID != "" {
		return resp.PanelID, nil
	}
	return resp.Data.PanelID, nil
}

// Update 整份覆盖面板内容, 返回服务端的新版本号。
//
// 这是不可撤销的远端写: 调用方负责先留快照。
func (p *PanelAPI) Update(panelID string, panel Panel) (int, error) {
	if err := ValidatePanel(panel); err != nil {
		return 0, err
	}
	panel.Version = 0
	raw, err := p.call("PUT", p.api.ProxyAPI+"/v2/panels/"+url.PathEscape(panelID),
		requests.JSON(map[string]any{"panel": panel}))
	if err != nil {
		return 0, err
	}
	var resp struct {
		Version int `json:"version"`
		Data    struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return 0, fmt.Errorf("解析保存结果失败: %w", err)
	}
	if resp.Version != 0 {
		return resp.Version, nil
	}
	return resp.Data.Version, nil
}

// Delete 删除面板。同样不可撤销。
func (p *PanelAPI) Delete(panelID string) error {
	_, err := p.call("DELETE", p.api.ProxyAPI+"/v2/panels/"+url.PathEscape(panelID), nil)
	return err
}
