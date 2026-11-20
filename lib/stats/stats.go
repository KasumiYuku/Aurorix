// Package stats 统计: 内存只存还没落库的增量, 库是唯一权威, 读侧走秒级缓存。
package stats

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/storage"
)

const (
	flushInterval = 5 * time.Second
	flushRows     = 4096
	keepDays      = 90
	topLimit      = 10
	snapshotTTL   = time.Second
	dayLayout     = "2006-01-02"
)

const (
	convUpsert = `INSERT INTO stats_conversations (scene, id, name, msg_count, friend) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (scene, id) DO UPDATE SET
			msg_count = stats_conversations.msg_count + excluded.msg_count,
			name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE stats_conversations.name END,
			friend = stats_conversations.friend OR excluded.friend`
	dayUpsert = `INSERT INTO stats_daily (day, recv, sent, button) VALUES ($1, $2, $3, $4)
		ON CONFLICT (day) DO UPDATE SET
			recv = stats_daily.recv + excluded.recv,
			sent = stats_daily.sent + excluded.sent,
			button = stats_daily.button + excluded.button`
	backfillGroups = `INSERT INTO stats_conversations (scene, id, name, msg_count)
		SELECT $1, group_id, name, msg_count FROM stats_groups
		ON CONFLICT DO NOTHING`
	backfillPeers = `INSERT INTO stats_conversations (scene, id, name, msg_count, friend)
		SELECT $1, peer_id, '', msg_count, true FROM stats_peers
		ON CONFLICT (scene, id) DO UPDATE SET friend = true`
	markMigrated = `INSERT INTO stats_migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`
)

type convKey struct {
	scene constant.Scene
	id    string
}

type convDelta struct {
	name   string
	msg    int64
	friend bool
}

type dayDelta struct {
	recv, sent, button int64
}

type totals struct {
	recv, sent, button uint64
}

var (
	mu      sync.Mutex
	flushMu sync.Mutex

	convPending = make(map[convKey]*convDelta)
	dayPending  = make(map[string]*dayDelta)

	life     totals
	todayKey string
	today    totals

	wake   = make(chan struct{}, 1)
	wg     sync.WaitGroup
	stopCh chan struct{}

	lastTrim string

	readMu    sync.Mutex
	readCache counts
	readReady bool
)

// Received 记一条收到的消息: 会话活跃度 + 总量; name 传空不会覆盖库里已有的名字。
func Received(target constant.Target, name string) {
	if target.ID == "" {
		return
	}
	mu.Lock()
	key := convKey{scene: target.Scene, id: target.ID}
	d := convPending[key]
	if d == nil {
		d = &convDelta{}
		convPending[key] = d
	}
	d.msg++
	if d.name == "" {
		d.name = name
	}
	if target.Scene == constant.SceneC2C {
		d.friend = true
	}
	rows := len(convPending) + len(dayPending)
	mu.Unlock()
	Recv()
	if rows >= flushRows {
		nudge()
	}
}

// Name 补一个会话名; 与 Received 分开是因为名字来得比消息晚(群名/频道名要先查平台接口)。
func Name(target constant.Target, name string) {
	if target.ID == "" || name == "" {
		return
	}
	mu.Lock()
	key := convKey{scene: target.Scene, id: target.ID}
	d := convPending[key]
	if d == nil {
		d = &convDelta{}
		convPending[key] = d
	}
	if d.name == "" {
		d.name = name
	}
	rows := len(convPending) + len(dayPending)
	mu.Unlock()
	if rows >= flushRows {
		nudge()
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
	key := time.Now().Format(dayLayout)
	mu.Lock()
	if key != todayKey {
		todayKey, today = key, totals{}
	}
	switch kind {
	case recvKind:
		today.recv++
		life.recv++
	case sentKind:
		today.sent++
		life.sent++
	case buttonKind:
		today.button++
		life.button++
	}
	d := dayPending[key]
	if d == nil {
		d = &dayDelta{}
		dayPending[key] = d
	}
	switch kind {
	case recvKind:
		d.recv++
	case sentKind:
		d.sent++
	case buttonKind:
		d.button++
	}
	rows := len(convPending) + len(dayPending)
	mu.Unlock()
	if rows >= flushRows {
		nudge()
	}
}

func nudge() {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// Start 建表、把旧表并进来一次、读一次汇总值, 然后启动唯一那个写库协程。
func Start() error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	if err := ensureSchema(db); err != nil {
		return err
	}
	if err := backfillLegacy(db); err != nil {
		return err
	}
	if err := seed(db); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	if stopCh != nil {
		return nil
	}
	stopCh = make(chan struct{})
	wg.Add(1)
	go loop(stopCh)
	return nil
}

func backfillLegacy(db *sql.DB) error {
	steps := []struct {
		name  string
		table string
		query string
		scene string
	}{
		{"stats_groups", "stats_groups", backfillGroups, string(constant.SceneGroup)},
		{"stats_peers", "stats_peers", backfillPeers, string(constant.SceneC2C)},
	}
	for _, step := range steps {
		var migrated bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM stats_migrations WHERE name = $1)`, step.name).Scan(&migrated); err != nil {
			return err
		}
		if migrated {
			continue
		}
		var present bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, step.table).Scan(&present); err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if present {
			if _, err := tx.Exec(step.query, step.scene); err != nil {
				tx.Rollback()
				return err
			}
		}
		if _, err := tx.Exec(markMigrated, step.name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func seed(db *sql.DB) error {
	var lifeTotals totals
	if err := db.QueryRow(
		`SELECT COALESCE(SUM(recv),0), COALESCE(SUM(sent),0), COALESCE(SUM(button),0) FROM stats_daily`).
		Scan(&lifeTotals.recv, &lifeTotals.sent, &lifeTotals.button); err != nil {
		return err
	}

	key := time.Now().Format(dayLayout)
	var dayTotals totals
	err := db.QueryRow(`SELECT recv, sent, button FROM stats_daily WHERE day = $1`, key).
		Scan(&dayTotals.recv, &dayTotals.sent, &dayTotals.button)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	mu.Lock()
	life, todayKey, today = lifeTotals, key, dayTotals
	mu.Unlock()
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
	mu.Lock()
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	mu.Unlock()
	wg.Wait()
	Flush()
}

// Flush 把当前增量写库; 失败原样退回内存, 下一轮重试。
func Flush() {
	flushMu.Lock()
	defer flushMu.Unlock()

	b := takePending()
	if b.empty() {
		return
	}
	if err := writeBatch(b); err != nil {
		b.restore()
	}
}

type batch struct {
	convs map[convKey]*convDelta
	days  map[string]*dayDelta
	trim  bool
}

func (b batch) empty() bool {
	return len(b.convs) == 0 && len(b.days) == 0 && !b.trim
}

func takePending() batch {
	mu.Lock()
	defer mu.Unlock()
	b := batch{convs: convPending, days: dayPending}
	convPending = make(map[convKey]*convDelta)
	dayPending = make(map[string]*dayDelta)
	if key := time.Now().Format(dayLayout); lastTrim != key {
		lastTrim = key
		b.trim = true
	}
	return b
}

func (b batch) restore() {
	mu.Lock()
	defer mu.Unlock()
	for key, d := range b.convs {
		if cur := convPending[key]; cur != nil {
			cur.msg += d.msg
			cur.friend = cur.friend || d.friend
			if d.name != "" {
				cur.name = d.name
			}
			continue
		}
		convPending[key] = d
	}
	for day, d := range b.days {
		if cur := dayPending[day]; cur != nil {
			cur.recv += d.recv
			cur.sent += d.sent
			cur.button += d.button
			continue
		}
		dayPending[day] = d
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

	for key, d := range b.convs {
		if _, err := tx.Exec(convUpsert, string(key.scene), key.id, d.name, d.msg, d.friend); err != nil {
			return err
		}
	}
	for day, d := range b.days {
		if _, err := tx.Exec(dayUpsert, day, d.recv, d.sent, d.button); err != nil {
			return err
		}
	}
	if b.trim {
		cutoff := time.Now().AddDate(0, 0, -keepDays).Format(dayLayout)
		if _, err := tx.Exec(`DELETE FROM stats_daily WHERE day < $1`, cutoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS stats_conversations (
			scene TEXT NOT NULL,
			id TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			msg_count BIGINT NOT NULL DEFAULT 0,
			friend BOOLEAN NOT NULL DEFAULT false,
			PRIMARY KEY (scene, id)
		)`,
		`ALTER TABLE stats_conversations ADD COLUMN IF NOT EXISTS friend BOOLEAN NOT NULL DEFAULT false`,
		`CREATE TABLE IF NOT EXISTS stats_daily (
			day TEXT PRIMARY KEY,
			recv BIGINT NOT NULL DEFAULT 0,
			sent BIGINT NOT NULL DEFAULT 0,
			button BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS stats_migrations (
			name TEXT PRIMARY KEY,
			at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS stats_conversations_msg ON stats_conversations(msg_count DESC)`,
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

// ConversationRow 活跃会话排行行。
type ConversationRow struct {
	Scene constant.Scene `json:"scene"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Msg   uint64         `json:"msg"`
}

// View 统计快照; Peers 是有过单聊往来的人(含已删好友的历史), Friends 是当前仍在好友位上的。
type View struct {
	Groups           int64             `json:"groups"`
	Peers            int64             `json:"peers"`
	Friends          int64             `json:"friends"`
	Channels         int64             `json:"channels"`
	TotalRecv        uint64            `json:"total_recv"`
	TotalSent        uint64            `json:"total_sent"`
	Today            DayStat           `json:"today"`
	TopConversations []ConversationRow `json:"top_conversations"`
}

type counts struct {
	at                               time.Time
	groups, peers, friends, channels int64
	top                              []ConversationRow
}

// Snapshot 组装当前统计快照。累计与今日来自内存累计值, 排行榜与实体数来自库。
func Snapshot() View {
	mu.Lock()
	view := View{
		TotalRecv: life.recv,
		TotalSent: life.sent,
		Today: DayStat{
			Recv:   today.recv,
			Sent:   today.sent,
			Button: today.button,
		},
	}
	mu.Unlock()

	cached := cachedCounts()
	view.Groups, view.Peers, view.Channels = cached.groups, cached.peers, cached.channels
	view.Friends = cached.friends
	view.TopConversations = cached.top
	if view.TopConversations == nil {
		view.TopConversations = []ConversationRow{}
	}
	return view
}

func cachedCounts() counts {
	readMu.Lock()
	defer readMu.Unlock()
	if readReady && time.Since(readCache.at) < snapshotTTL {
		return readCache
	}
	next, err := readCounts()
	if err != nil {
		return readCache
	}
	next.at = time.Now()
	readCache, readReady = next, true
	return next
}

func readCounts() (counts, error) {
	db, err := storage.DB()
	if err != nil {
		return counts{}, err
	}
	out := counts{top: make([]ConversationRow, 0, topLimit)}
	if err := db.QueryRow(
		`SELECT
			COUNT(*) FILTER (WHERE scene = $1),
			COUNT(*) FILTER (WHERE scene = $2),
			COUNT(*) FILTER (WHERE scene = $3 OR scene = $4),
			COUNT(*) FILTER (WHERE scene = $2 AND friend)
		FROM stats_conversations`,
		string(constant.SceneGroup), string(constant.SceneC2C),
		string(constant.SceneChannel), string(constant.SceneDirect),
	).Scan(&out.groups, &out.peers, &out.channels, &out.friends); err != nil {
		return counts{}, err
	}

	rows, err := db.Query(
		`SELECT scene, id, name, msg_count FROM stats_conversations
		ORDER BY msg_count DESC, scene, id LIMIT $1`, topLimit)
	if err != nil {
		return counts{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row ConversationRow
		var scene string
		if err := rows.Scan(&scene, &row.ID, &row.Name, &row.Msg); err != nil {
			return counts{}, err
		}
		row.Scene = constant.Scene(scene)
		out.top = append(out.top, row)
	}
	return out, rows.Err()
}

// Friend 记下一个好友; 加好友事件很稀疏, 直接落库不进增量队列。平台不提供好友列表, 只能靠事件与消息一点点补。
func Friend(userID string) error {
	if userID == "" {
		return nil
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO stats_conversations (scene, id, friend) VALUES ($1, $2, true)
		ON CONFLICT (scene, id) DO UPDATE SET friend = true`,
		string(constant.SceneC2C), userID)
	return err
}

// Unfriend 好友被删除: 只摘好友标记, 历史消息数留着 —— 活跃会话该有多少还是多少。
func Unfriend(userID string) error {
	if userID == "" {
		return nil
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE stats_conversations SET friend = false WHERE scene = $1 AND id = $2`,
		string(constant.SceneC2C), userID)
	return err
}
