package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/api"
	"github.com/KasumiYuku/Aurorix/lib/config"
	"github.com/KasumiYuku/Aurorix/lib/logx"
)

// 指令面板管理。面板住在 QQ 服务端: 写是整份覆盖、删除不可撤销, 所以凡是会改到远端
// 的动作都先在本地留一份快照 —— 这是它跟官方那套界面的主要区别。
const menuSnapshotKeep = 20

var menuLog = logx.New("menu")

type menuSnapshot struct {
	File       string         `json:"file"`
	PanelID    string         `json:"panel_id"`
	Scope      api.PanelScope `json:"scope"`
	TargetType string         `json:"target_type"`
	Panel      api.Panel      `json:"panel"`
	Version    int            `json:"version"`
	Action     string         `json:"action"`
	SavedAt    string         `json:"saved_at"`
}

// panelStore 是同步要用的那几个动作; 抽成接口是为了能在测试里替身。
type panelStore interface {
	Get(panelID string) (*api.PanelRecord, error)
	Create(scope api.PanelScope, panel api.Panel) (string, error)
	Update(panelID string, panel api.Panel) (int, error)
	Count() (int, error)
}

// syncTarget 一个目标场景: 给了 panel_id 就是覆盖它, 没给就是新建一份。
type syncTarget struct {
	Scope   api.PanelScope `json:"scope"`
	PanelID string         `json:"panel_id"`
}

// syncResult 每个目标单独一条结果。
type syncResult struct {
	Scope    api.PanelScope `json:"scope"`
	PanelID  string         `json:"panel_id,omitempty"`
	Mode     string         `json:"mode,omitempty"`
	Snapshot string         `json:"snapshot,omitempty"`
	Error    string         `json:"error,omitempty"`
}

func registerMenuRoutes(apiMux *http.ServeMux, deps Deps) {
	if deps.Client == nil {
		return
	}
	panels := deps.Client.Panel

	apiMux.HandleFunc("GET /admin/api/menu/meta", func(w http.ResponseWriter, r *http.Request) {
		scopes := make([]H, 0, len(api.PanelScopes))
		for _, scope := range api.PanelScopes {
			scopes = append(scopes, H{"scope": scope, "label": api.PanelScopeLabels[scope]})
		}
		writeJSON(w, http.StatusOK, H{
			"scopes": scopes,
			"limits": H{
				"name_width":   api.PanelNameMaxWidth,
				"desc_width":   api.PanelDescMaxWidth,
				"remark_len":   api.PanelRemarkMaxLen,
				"max_items":    api.PanelMaxItems,
				"max_per_bot":  api.PanelMaxPerBot,
				"item_types":   []string{api.PanelItemCommand, api.PanelItemLink},
				"target_type":  api.PanelTargetAll,
				"display_note": "长度按显示宽度算: 中文与全角标点各计 2",
			},
		})
	})

	apiMux.HandleFunc("GET /admin/api/menu/panels", func(w http.ResponseWriter, r *http.Request) {
		scopes := make([]H, 0, len(api.PanelScopes))
		total := 0
		for _, scope := range api.PanelScopes {
			records, err := panels.List(scope)
			if err != nil {
				writePanelError(w, err)
				return
			}
			if records == nil {
				records = []api.PanelRecord{}
			}
			total += len(records)
			scopes = append(scopes, H{
				"scope": scope, "label": api.PanelScopeLabels[scope], "panels": records,
			})
		}
		writeJSON(w, http.StatusOK, H{
			"scopes": scopes, "total": total, "max_per_bot": api.PanelMaxPerBot,
		})
	})

	apiMux.HandleFunc("POST /admin/api/menu/panels", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scope api.PanelScope `json:"scope"`
			Panel api.Panel      `json:"panel"`
		}
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		if !api.ValidPanelScope(input.Scope) {
			writeJSON(w, http.StatusBadRequest, H{"error": fmt.Sprintf("场景 %q 不认识", input.Scope)})
			return
		}
		count, err := panels.Count()
		if err != nil {
			writePanelError(w, err)
			return
		}
		if count >= api.PanelMaxPerBot {
			writeJSON(w, http.StatusConflict, H{
				"error": fmt.Sprintf("该机器人已有 %d 个面板, 达到上限 %d(四个场景合计)。请先删掉不用的面板再新建",
					count, api.PanelMaxPerBot),
			})
			return
		}
		panelID, err := panels.Create(input.Scope, input.Panel)
		if err != nil {
			writePanelError(w, err)
			return
		}
		menuLog.Infof("新建指令面板: 场景=%s id=%s 条目=%d", input.Scope, panelID, len(input.Panel.Items))
		writeJSON(w, http.StatusOK, H{"ok": true, "panel_id": panelID})
	})

	apiMux.HandleFunc("PUT /admin/api/menu/panels/{id}", func(w http.ResponseWriter, r *http.Request) {
		panelID := r.PathValue("id")
		var input struct {
			Panel api.Panel `json:"panel"`
		}
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		if err := api.ValidatePanel(input.Panel); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
			return
		}
		current, err := panels.Get(panelID)
		if err != nil {
			writePanelError(w, err)
			return
		}
		snap, snapErr := saveMenuSnapshot("update", current)
		version, err := panels.Update(panelID, input.Panel)
		if err != nil {
			writePanelError(w, err)
			return
		}
		menuLog.Infof("覆盖指令面板: id=%s 条目 %d -> %d 版本=%d", panelID,
			len(current.Panel.Items), len(input.Panel.Items), version)
		result := H{"ok": true, "version": version}
		addSnapshotNotice(result, snap, snapErr)
		writeJSON(w, http.StatusOK, result)
	})

	apiMux.HandleFunc("DELETE /admin/api/menu/panels/{id}", func(w http.ResponseWriter, r *http.Request) {
		panelID := r.PathValue("id")
		current, err := panels.Get(panelID)
		if err != nil {
			writePanelError(w, err)
			return
		}
		snap, snapErr := saveMenuSnapshot("delete", current)
		if err := panels.Delete(panelID); err != nil {
			writePanelError(w, err)
			return
		}
		menuLog.Warnf("删除指令面板: 场景=%s id=%s", current.Scope, panelID)
		result := H{"ok": true}
		addSnapshotNotice(result, snap, snapErr)
		writeJSON(w, http.StatusOK, result)
	})
	apiMux.HandleFunc("POST /admin/api/menu/sync", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			From    string       `json:"from"`
			Targets []syncTarget `json:"targets"`
		}
		if err := readJSON(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, H{"error": "请求格式无效"})
			return
		}
		if strings.TrimSpace(input.From) == "" {
			writeJSON(w, http.StatusBadRequest, H{"error": "没说要同步哪一份面板"})
			return
		}
		results, err := syncPanels(panels, input.From, input.Targets)
		if err != nil {
			writePanelError(w, err)
			return
		}
		done := 0
		for _, res := range results {
			if res.Error == "" {
				done++
			}
		}
		menuLog.Infof("同步指令面板: 源=%s 目标=%d 成功=%d 失败=%d", input.From, len(results), done, len(results)-done)
		writeJSON(w, http.StatusOK, H{"ok": true, "from": input.From, "results": results})
	})

	apiMux.HandleFunc("GET /admin/api/menu/snapshots", func(w http.ResponseWriter, r *http.Request) {
		snaps, err := listMenuSnapshots()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, H{"error": "读取快照目录失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, H{"snapshots": snaps, "dir": menuSnapshotDir()})
	})

	apiMux.HandleFunc("POST /admin/api/menu/snapshots/{file}/restore", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("file")
		if strings.ContainsAny(name, `/\`) || name == "" {
			writeJSON(w, http.StatusBadRequest, H{"error": "快照名不合法"})
			return
		}
		snap, err := readMenuSnapshot(name)
		if err != nil {
			writeJSON(w, http.StatusNotFound, H{"error": "快照不存在或已损坏: " + err.Error()})
			return
		}
		mode := "overwrite"
		panelID := snap.PanelID
		if _, err := panels.Get(panelID); err != nil {
			var pe *api.PanelError
			if !errors.As(err, &pe) || normCode(pe.Code) != 30006 {
				writePanelError(w, err)
				return
			}
			// 面板已被删除: 服务端会重新分配 id, 所以叫"重建"而不是"还原"。
			created, createErr := panels.Create(snap.Scope, snap.Panel)
			if createErr != nil {
				writePanelError(w, createErr)
				return
			}
			panelID, mode = created, "recreated"
		} else if _, err := panels.Update(panelID, snap.Panel); err != nil {
			writePanelError(w, err)
			return
		}
		menuLog.Warnf("回滚指令面板: 快照=%s 场景=%s id=%s 模式=%s", name, snap.Scope, panelID, mode)
		writeJSON(w, http.StatusOK, H{"ok": true, "panel_id": panelID, "scope": snap.Scope, "mode": mode})
	})
}

// normCode 面板域错误码有两种写法(文档 40030013 / 实测 30013), 比较前统一按低位。
func normCode(code int) int {
	if code >= 40000000 {
		return code % 100000
	}
	return code
}

func addSnapshotNotice(result H, snap *menuSnapshot, err error) {
	if err != nil {
		result["snapshot_error"] = err.Error()
		return
	}
	if snap != nil {
		result["snapshot"] = snap.File
	}
}

// writePanelError 把面板域错误映射成合适的 HTTP 状态 + 人话原因。
func writePanelError(w http.ResponseWriter, err error) {
	var pe *api.PanelError
	if !errors.As(err, &pe) {
		writeJSON(w, http.StatusBadRequest, H{"error": err.Error()})
		return
	}
	payload := H{"error": pe.Error(), "code": pe.Code, "reason": pe.Reason, "limit": pe.Limit}
	switch normCode(pe.Code) {
	case 30006:
		writeJSON(w, http.StatusNotFound, payload)
	case 30009:
		writeJSON(w, http.StatusConflict, payload)
	case 30011, 30012, 30013, 30015, 30016, 30018, 30008, 30020, 30021:
		writeJSON(w, http.StatusBadRequest, payload)
	default:
		if pe.HTTP == http.StatusTooManyRequests {
			writeJSON(w, http.StatusTooManyRequests, payload)
			return
		}
		writeJSON(w, http.StatusBadGateway, payload)
	}
}

// menuSnapshotDir 快照放在数据库旁边。派生而不是新增配置项: 数据都该在一起。
func menuSnapshotDir() string {
	dir := filepath.Dir(config.Current().Database)
	if dir == "." || dir == "" {
		dir = "data"
	}
	return filepath.Join(dir, "menu-snapshots")
}

func saveMenuSnapshot(action string, record *api.PanelRecord) (*menuSnapshot, error) {
	dir := menuSnapshotDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		menuLog.Warnf("指令面板快照目录建不出来(%s), 这次改动没有留档: %v", dir, err)
		return nil, err
	}
	now := time.Now()
	snap := &menuSnapshot{
		File:       uniqueSnapshotFile(dir, fmt.Sprintf("%s-%s-%s", now.Format("20060102-150405"), record.Scope, sanitizeName(record.PanelID))),
		PanelID:    record.PanelID,
		Scope:      record.Scope,
		TargetType: record.TargetType,
		Panel:      record.Panel,
		Version:    record.Version,
		Action:     action,
		SavedAt:    now.Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, snap.File), data, 0o644); err != nil {
		menuLog.Warnf("指令面板快照写不进去(%s): %v", snap.File, err)
		return nil, err
	}
	pruneMenuSnapshots(record.Scope, record.PanelID)
	return snap, nil
}

func readMenuSnapshot(name string) (*menuSnapshot, error) {
	data, err := os.ReadFile(filepath.Join(menuSnapshotDir(), name))
	if err != nil {
		return nil, err
	}
	var snap menuSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	snap.File = name
	return &snap, nil
}

func listMenuSnapshots() ([]menuSnapshot, error) {
	dir := menuSnapshotDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []menuSnapshot{}, nil
		}
		return nil, err
	}
	out := make([]menuSnapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		snap, err := readMenuSnapshot(entry.Name())
		if err != nil {
			continue
		}
		out = append(out, *snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File > out[j].File })
	return out, nil
}

// uniqueSnapshotFile 时间戳只精确到秒, 同一秒里连着改两次会撞名把上一份盖掉 —— 所以撞了就往后编号。
func uniqueSnapshotFile(dir, base string) string {
	name := base + ".json"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
		name = fmt.Sprintf("%s-%d.json", base, i)
	}
}

// pruneMenuSnapshots 同一个面板只留最近 menuSnapshotKeep 份, 免得目录无限长。
//
// 按快照内容里的场景与 id 匹配, 不看文件名: 文件名可能带 -2 之类的避让后缀。
func pruneMenuSnapshots(scope api.PanelScope, panelID string) {
	all, err := listMenuSnapshots()
	if err != nil {
		return
	}
	kept := 0
	for _, snap := range all {
		if snap.Scope != scope || snap.PanelID != panelID {
			continue
		}
		kept++
		if kept > menuSnapshotKeep {
			_ = os.Remove(filepath.Join(menuSnapshotDir(), snap.File))
		}
	}
}

// sanitizeName 面板 id 目前形如 p_xxx, 仍然过一遍, 免得它哪天带上路径分隔符。
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// syncPanels 把一份面板同步到其他场景: 给了 panel_id 就覆盖它, 没给就新建一份。
func syncPanels(store panelStore, from string, targets []syncTarget) ([]syncResult, error) {
	src, err := store.Get(from)
	if err != nil {
		return nil, err
	}
	if !editablePanel(src) {
		return nil, errors.New("这份面板只对指定群或指定人生效, 不能作为同步的来源")
	}
	if len(targets) == 0 {
		return nil, errors.New("没有选要同步到哪个场景")
	}
	fresh := 0
	for _, t := range targets {
		if !api.ValidPanelScope(t.Scope) {
			return nil, fmt.Errorf("场景 %q 不认识", t.Scope)
		}
		if t.PanelID == "" {
			fresh++
		}
	}
	if fresh > 0 {
		count, err := store.Count()
		if err != nil {
			return nil, err
		}
		if count+fresh > api.PanelMaxPerBot {
			return nil, fmt.Errorf("要新建 %d 份, 现在已有 %d 份、上限 %d —— 先删掉不用的面板", fresh, count, api.PanelMaxPerBot)
		}
	}

	content := api.Panel{Items: append([]api.PanelItem(nil), src.Panel.Items...), Remark: src.Panel.Remark}
	out := make([]syncResult, 0, len(targets))
	for _, t := range targets {
		out = append(out, syncOne(store, t, content))
	}
	return out, nil
}

// syncOne 做一个目标; 失败只记在这条上, 不影响其余。
func syncOne(store panelStore, target syncTarget, content api.Panel) syncResult {
	res := syncResult{Scope: target.Scope, PanelID: target.PanelID}
	if target.PanelID == "" {
		id, err := store.Create(target.Scope, content)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.PanelID, res.Mode = id, "created"
		return res
	}
	current, err := store.Get(target.PanelID)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if !editablePanel(current) {
		res.Error = "那份也只对指定群或指定人生效, 没动它"
		return res
	}
	if current.Scope != target.Scope {
		res.Error = "那份面板不在要同步的场景里"
		return res
	}
	snap, snapErr := saveMenuSnapshot("sync", current)
	if _, err := store.Update(target.PanelID, content); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Mode = "overwritten"
	if snapErr == nil && snap != nil {
		res.Snapshot = snap.File
	}
	return res
}

// editablePanel 只对指定群/指定人生效的面板在服务端是只读的。
func editablePanel(record *api.PanelRecord) bool {
	return record != nil && (record.TargetType == "" || record.TargetType == api.PanelTargetAll)
}
