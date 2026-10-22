// Package stats 管理台统计: 计数在内存累加, 单写者按增量落库, 消息热路径不碰 DB。
package stats

import (
	"database/sql"
	"sort"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/storage"
)

const (
	flushInterval = 5 * time.Second
	flushRows     = 4096
	keepDays      = 90
)

const (
	groupUpsert = `INSERT INTO stats_groups (group_id, name, msg_count) VALUES (?, ?, ?)
		ON CONFLICT(group_id) DO UPDATE SET
			msg_count = stats_groups.msg_count + excluded.msg_count,
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE stats_groups.name END`
	peerUpsert = `INSERT INTO stats_peers (peer_id, msg_count) VALUES (?, ?)
		ON CONFLICT(peer_id) DO UPDATE SET msg_count = stats_peers.msg_count + excluded.msg_count`
	dayUpsert = `INSERT INTO stats_daily (day, recv, sent, button) VALUES (?, ?, ?, ?)
		ON CONFLICT(day) DO UPDATE SET
			recv = stats_daily.recv + excluded.recv,
			sent = stats_daily.sent + excluded.sent,
			button = stats_daily.button + excluded.button`
)

var (
	statsMu sync.Mutex
	flushMu sync.Mutex

	groups = make(map[string]*groupStat)
	peers  = make(map[string]*peerStat)
	daily  = make(map[string]*dayStat)

	dirtyGroups = make(map[string]struct{})
	dirtyPeers  = make(map[string]struct{})
	dirtyDays   = make(map[string]struct{})

	wake   = make(chan struct{}, 1)
	wg     sync.WaitGroup
	stopCh chan struct{}

	lastTrim string
)

type groupStat struct {
	name    string
	msg     uint64
	pending int64
}

type peerStat struct {
	msg     uint64
	pending int64
}

type dayStat struct {
	recv, sent, button    uint64
	dRecv, dSent, dButton int64
}

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
	g.pending++
	dirtyGroups[groupID] = struct{}{}
	rows := len(dirtyGroups) + len(dirtyPeers) + len(dirtyDays)
	statsMu.Unlock()
	if rows >= flushRows {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func Peer(peerID string) {
	statsMu.Lock()
	p := peers[peerID]
	if p == nil {
		p = &peerStat{}
		peers[peerID] = p
	}
	p.msg++
	p.pending++
	dirtyPeers[peerID] = struct{}{}
	rows := len(dirtyGroups) + len(dirtyPeers) + len(dirtyDays)
	statsMu.Unlock()
	if rows >= flushRows {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

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
	day := todayOf(time.Now())
	statsMu.Lock()
	d := daily[day]
	if d == nil {
		d = &dayStat{}
		daily[day] = d
	}
	switch kind {
	case recvKind:
		d.recv++
		d.dRecv++
	case sentKind:
		d.sent++
		d.dSent++
	case buttonKind:
		d.button++
		d.dButton++
	}
	dirtyDays[day] = struct{}{}
	rows := len(dirtyGroups) + len(dirtyPeers) + len(dirtyDays)
	statsMu.Unlock()
	if rows >= flushRows {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func todayOf(t time.Time) string { return t.Format("2006-01-02") }

// Start 装载历史数据并启动唯一那个写库协程。
func Start() error {
	if err := load(); err != nil {
		return err
	}
	statsMu.Lock()
	defer statsMu.Unlock()
	if stopCh != nil {
		return nil
	}
	stopCh = make(chan struct{})
	wg.Add(1)
	go loop(stopCh)
	return nil
}

func loop(stop chan struct{}) {
	defer wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-wake:
		case <-stop:
			return
		}
		Flush()
	}
}

// Stop 停掉写库协程并同步落最后一批; 之后进程再退出最多丢一个 flushInterval 的增量。
func Stop() {
	statsMu.Lock()
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	statsMu.Unlock()
	wg.Wait()
	Flush()
}

// Flush 把当前增量写库; 失败原样退回内存, 下一轮重试。
func Flush() {
	flushMu.Lock()
	defer flushMu.Unlock()

	batch := takeDirty()
	if batch.empty() {
		return
	}
	if err := writeBatch(batch); err != nil {
		batch.restore()
	}
}

type groupDelta struct {
	id, name string
	msg      int64
}

type peerDelta struct {
	id  string
	msg int64
}

type dayDelta struct {
	day                string
	recv, sent, button int64
}

type batch struct {
	groups []groupDelta
	peers  []peerDelta
	days   []dayDelta
	trim   bool
}

func (b batch) empty() bool {
	return len(b.groups) == 0 && len(b.peers) == 0 && len(b.days) == 0 && !b.trim
}

// takeDirty 取走增量并清空脏标记: 拿到手的这段时间里新来的计数会累加到归零之后, 不会被覆盖。
func takeDirty() batch {
	statsMu.Lock()
	defer statsMu.Unlock()

	b := batch{
		groups: make([]groupDelta, 0, len(dirtyGroups)),
		peers:  make([]peerDelta, 0, len(dirtyPeers)),
		days:   make([]dayDelta, 0, len(dirtyDays)),
	}
	for id := range dirtyGroups {
		delete(dirtyGroups, id)
		g := groups[id]
		if g == nil || (g.pending == 0) {
			continue
		}
		b.groups = append(b.groups, groupDelta{id: id, name: g.name, msg: g.pending})
		g.pending = 0
	}
	for id := range dirtyPeers {
		delete(dirtyPeers, id)
		p := peers[id]
		if p == nil || p.pending == 0 {
			continue
		}
		b.peers = append(b.peers, peerDelta{id: id, msg: p.pending})
		p.pending = 0
	}
	for day := range dirtyDays {
		delete(dirtyDays, day)
		d := daily[day]
		if d == nil || (d.dRecv == 0 && d.dSent == 0 && d.dButton == 0) {
			continue
		}
		b.days = append(b.days, dayDelta{day: day, recv: d.dRecv, sent: d.dSent, button: d.dButton})
		d.dRecv, d.dSent, d.dButton = 0, 0, 0
	}
	if today := todayOf(time.Now()); lastTrim != today {
		lastTrim = today
		b.trim = true
	}
	return b
}

func (b batch) restore() {
	statsMu.Lock()
	defer statsMu.Unlock()
	for _, d := range b.groups {
		if g := groups[d.id]; g != nil {
			g.pending += d.msg
			dirtyGroups[d.id] = struct{}{}
		}
	}
	for _, d := range b.peers {
		if p := peers[d.id]; p != nil {
			p.pending += d.msg
			dirtyPeers[d.id] = struct{}{}
		}
	}
	for _, d := range b.days {
		if day := daily[d.day]; day != nil {
			day.dRecv += d.recv
			day.dSent += d.sent
			day.dButton += d.button
			dirtyDays[d.day] = struct{}{}
		}
	}
}

func writeBatch(b batch) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, d := range b.groups {
		if _, err := tx.Exec(groupUpsert, d.id, d.name, d.msg); err != nil {
			return err
		}
	}
	for _, d := range b.peers {
		if _, err := tx.Exec(peerUpsert, d.id, d.msg); err != nil {
			return err
		}
	}
	for _, d := range b.days {
		if _, err := tx.Exec(dayUpsert, d.day, d.recv, d.sent, d.button); err != nil {
			return err
		}
	}
	if b.trim {
		cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
		if _, err := tx.Exec(`DELETE FROM stats_daily WHERE day < ?`, cutoff); err != nil {
			return err
		}
	}
	return tx.Commit()
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
		return err
	}
	for rows.Next() {
		var id, name string
		var msg uint64
		if err := rows.Scan(&id, &name, &msg); err != nil {
			rows.Close()
			return err
		}
		groups[id] = &groupStat{name: name, msg: msg}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`SELECT peer_id, msg_count FROM stats_peers`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var msg uint64
		if err := rows.Scan(&id, &msg); err != nil {
			rows.Close()
			return err
		}
		peers[id] = &peerStat{msg: msg}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = db.Query(`SELECT day, recv, sent, button FROM stats_daily`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var day string
		var recv, sent, button uint64
		if err := rows.Scan(&day, &recv, &sent, &button); err != nil {
			return err
		}
		daily[day] = &dayStat{recv: recv, sent: sent, button: button}
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
			return err
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
	Groups    int64      `json:"groups"`
	Peers     int64      `json:"peers"`
	TotalRecv uint64     `json:"total_recv"`
	TotalSent uint64     `json:"total_sent"`
	Today     DayStat    `json:"today"`
	TopGroups []GroupRow `json:"top_groups"`
}

// Snapshot 组装当前统计快照; 读的是内存权威值, 不碰库。
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
