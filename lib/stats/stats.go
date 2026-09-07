// Package stats 管理台统计: 群/私聊用户/每日计数, SQLite 持久化, 重启不丢。
package stats

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/storage"
)

// 批量落库水位与周期: 高频消息不逐条写库, 累积后合并; 写失败保留内存重试。
const (
	flushThreshold = 128
	flushInterval  = 10 * time.Second
	keepDays       = 90 // 每日计数保留天数, 超期滚动清理
)

var (
	statsMu     sync.Mutex
	groups      = make(map[string]*groupStat)
	peers       = make(map[string]uint64)
	daily       = make(map[string]*dayStat)
	dirty       atomic.Bool // 存在未落库数据
	flushTicker *time.Ticker
	stopCh      chan struct{}
)

type groupStat struct {
	name string
	msg  uint64
}

type dayStat struct {
	recv   uint64
	sent   uint64
	button uint64
}

// Group 记录一条群消息; name 非空时顺带更新群名缓存。
func Group(groupID, name string) {
	statsMu.Lock()
	g := groups[groupID]
	if g == nil {
		g = &groupStat{}
		groups[groupID] = g
	}
	if name != "" {
		g.name = name
	}
	g.msg++
	statsMu.Unlock()
	dirty.Store(true)
	maybeFlush()
}

// Peer 记录一条私聊消息。
func Peer(peerID string) {
	statsMu.Lock()
	peers[peerID]++
	statsMu.Unlock()
	dirty.Store(true)
	maybeFlush()
}

// Recv/Sent/Button 记录今日收发与按钮计数。
func Recv()   { dayAdd(recvKind) }
func Sent()   { dayAdd(sentKind) }
func Button() { dayAdd(buttonKind) }

type dayKind int

const (
	recvKind dayKind = iota
	sentKind
	buttonKind
)

func dayAdd(kind dayKind) {
	statsMu.Lock()
	d := todayOf(time.Now())
	cur := daily[d]
	if cur == nil {
		cur = &dayStat{}
		daily[d] = cur
	}
	switch kind {
	case recvKind:
		cur.recv++
	case sentKind:
		cur.sent++
	case buttonKind:
		cur.button++
	}
	statsMu.Unlock()
	dirty.Store(true)
	maybeFlush()
}

func todayOf(t time.Time) string { return t.Format("2006-01-02") }

// maybeFlush 内存积压超水位时立即落库, 否则交给定时器。
func maybeFlush() {
	statsMu.Lock()
	pending := len(groups) + len(peers) + len(daily)
	statsMu.Unlock()
	if pending >= flushThreshold {
		Flush()
	}
}

// 表结构与装载/落库实现不依赖外部状态, 全部走 storage.DB 单连接。

// Start 装载历史数据并启动后台 flush 循环。
func Start() error {
	if err := load(); err != nil {
		return err
	}
	statsMu.Lock()
	defer statsMu.Unlock()
	if flushTicker != nil {
		return nil
	}
	flushTicker = time.NewTicker(flushInterval)
	stopCh = make(chan struct{})
	go loop()
	return nil
}

func loop() {
	for {
		select {
		case <-flushTicker.C:
			Flush()
		case <-stopCh:
			return
		}
	}
}

// Stop 停止后台 flush 并兜底落库 (进程退出前调用)。
func Stop() {
	statsMu.Lock()
	if flushTicker != nil {
		flushTicker.Stop()
		flushTicker = nil
		close(stopCh)
		stopCh = nil
	}
	statsMu.Unlock()
	Flush()
}

// Flush 把内存累计值合并写库; 失败保留内存, 下次重试。
func Flush() {
	if !dirty.Load() {
		return
	}
	db, err := storage.DB()
	if err != nil {
		return
	}

	statsMu.Lock()
	defer statsMu.Unlock()

	tx, err := db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	for id, g := range groups {
		if _, err := tx.Exec(
			`INSERT INTO stats_groups (group_id, name, msg_count) VALUES (?, ?, ?)
			 ON CONFLICT(group_id) DO UPDATE SET name=excluded.name, msg_count=excluded.msg_count`,
			id, g.name, g.msg); err != nil {
			return
		}
	}
	for id, c := range peers {
		if _, err := tx.Exec(
			`INSERT INTO stats_peers (peer_id, msg_count) VALUES (?, ?)
			 ON CONFLICT(peer_id) DO UPDATE SET msg_count=excluded.msg_count`,
			id, c); err != nil {
			return
		}
	}
	for d, s := range daily {
		if _, err := tx.Exec(
			`INSERT INTO stats_daily (day, recv, sent, button) VALUES (?, ?, ?, ?)
			 ON CONFLICT(day) DO UPDATE SET recv=excluded.recv, sent=excluded.sent, button=excluded.button`,
			d, s.recv, s.sent, s.button); err != nil {
			return
		}
	}
	// 滚动清理超期日数据
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	if _, err := tx.Exec(`DELETE FROM stats_daily WHERE day < ?`, cutoff); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}
	dirty.Store(false)
}

func load() error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	if err := ensureSchema(db); err != nil {
		return err
	}

	statsMu.Lock()
	defer statsMu.Unlock()

	rows, err := db.Query(`SELECT group_id, name, msg_count FROM stats_groups`)
	if err != nil {
		return fmt.Errorf("load groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var msg uint64
		if err := rows.Scan(&id, &name, &msg); err != nil {
			return err
		}
		groups[id] = &groupStat{name: name, msg: msg}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`SELECT peer_id, msg_count FROM stats_peers`)
	if err != nil {
		return fmt.Errorf("load peers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var msg uint64
		if err := rows.Scan(&id, &msg); err != nil {
			return err
		}
		peers[id] = msg
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`SELECT day, recv, sent, button FROM stats_daily`)
	if err != nil {
		return fmt.Errorf("load daily: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var recv, sent, button uint64
		if err := rows.Scan(&d, &recv, &sent, &button); err != nil {
			return err
		}
		daily[d] = &dayStat{recv: recv, sent: sent, button: button}
	}
	return rows.Err()
}

func ensureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS stats_groups (
			group_id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			msg_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS stats_peers (
			peer_id TEXT PRIMARY KEY,
			msg_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS stats_daily (
			day TEXT PRIMARY KEY,
			recv INTEGER NOT NULL DEFAULT 0,
			sent INTEGER NOT NULL DEFAULT 0,
			button INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("ensure stats schema: %w", err)
		}
	}
	return nil
}

// DayStat 单日计数快照。
type DayStat struct {
	Recv   uint64 `json:"recv"`
	Sent   uint64 `json:"sent"`
	Button uint64 `json:"button"`
}

// GroupRow 活跃群排行行。
type GroupRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Msg  uint64 `json:"msg"`
}

// View 统计快照, 供管理台展示。
type View struct {
	Groups    int64      `json:"groups"`     // 见过的群总数
	Peers     int64      `json:"peers"`      // 私聊过的用户总数
	TotalRecv uint64     `json:"total_recv"` // 累计收
	TotalSent uint64     `json:"total_sent"` // 累计发
	Today     DayStat    `json:"today"`      // 今日活跃
	TopGroups []GroupRow `json:"top_groups"` // 活跃群 TOP10
}

// Snapshot 组装当前统计快照。
func Snapshot() View {
	statsMu.Lock()
	defer statsMu.Unlock()

	var totalRecv, totalSent uint64
	for _, d := range daily {
		totalRecv += d.recv
		totalSent += d.sent
	}
	today := daily[todayOf(time.Now())]
	if today == nil {
		today = &dayStat{}
	}

	top := make([]GroupRow, 0, len(groups))
	for id, g := range groups {
		top = append(top, GroupRow{ID: id, Name: g.name, Msg: g.msg})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].Msg > top[j].Msg })
	if len(top) > 10 {
		top = top[:10]
	}

	return View{
		Groups:    int64(len(groups)),
		Peers:     int64(len(peers)),
		TotalRecv: totalRecv,
		TotalSent: totalSent,
		Today: DayStat{
			Recv:   today.recv,
			Sent:   today.sent,
			Button: today.button,
		},
		TopGroups: top,
	}
}
