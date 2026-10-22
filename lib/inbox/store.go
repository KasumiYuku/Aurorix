// Package inbox 把机器人收发的消息落进 SQLite, 并聚合成可浏览的会话。
package inbox

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 保留策略。没有任何清理的消息库迟早长到没人敢碰, 所以上限写死在这里而不是留给运维去发现。
const (
	maxPerConversation = 500
	maxAge             = 90 * 24 * time.Hour
	previewRunes       = 60
	DefaultPageSize    = 50
	MaxPageSize        = 200
)

type Kind string

const (
	KindGroup Kind = "group"
	KindC2C   Kind = "c2c"
)

type Direction string

const (
	Inbound  Direction = "in"
	Outbound Direction = "out"
)

// Conversation 是一个对话入口: 群或单聊。
type Conversation struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	Name        string    `json:"name"`
	LastAt      time.Time `json:"last_at"`
	LastPreview string    `json:"last_preview"`
}

// Message 是一条入库的消息。方向决定它在页面上靠哪边, sender 只有入站才有意义。
type Message struct {
	ID         int64     `json:"id"`
	ConvID     string    `json:"conv_id"`
	Kind       Kind      `json:"kind"`
	Direction  Direction `json:"direction"`
	SenderID   string    `json:"sender_id,omitempty"`
	SenderName string    `json:"sender_name,omitempty"`
	Content    string    `json:"content"`
	PlatformID string    `json:"platform_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

const schema = `
CREATE TABLE IF NOT EXISTS inbox_conversations (
	id           TEXT PRIMARY KEY,
	kind         TEXT NOT NULL,
	name         TEXT NOT NULL DEFAULT '',
	last_at      INTEGER NOT NULL DEFAULT 0,
	last_preview TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS inbox_messages (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	conv_id         TEXT NOT NULL,
	direction       TEXT NOT NULL,
	sender_id       TEXT NOT NULL DEFAULT '',
	sender_name     TEXT NOT NULL DEFAULT '',
	content         TEXT NOT NULL DEFAULT '',
	platform_msg_id TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS inbox_messages_conv ON inbox_messages(conv_id, id);
CREATE INDEX IF NOT EXISTS inbox_messages_at ON inbox_messages(created_at);
`

func (s *Store) Ensure() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("inbox: 没有可用的数据库")
	}
	_, err := s.db.Exec(schema)
	return err
}

// Ready 报告这个 Store 能不能用; 接线处据此决定要不要挂钩子。
func (s *Store) Ready() bool { return s != nil && s.db != nil }

// Save 写一条消息并顺手更新它所属会话的活跃时间与预览。
func (s *Store) Save(m Message) error {
	if !s.Ready() || strings.TrimSpace(m.ConvID) == "" {
		return nil
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	at := m.CreatedAt.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO inbox_messages (conv_id, direction, sender_id, sender_name, content, platform_msg_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ConvID, string(m.Direction), m.SenderID, m.SenderName, m.Content, m.PlatformID, at,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO inbox_conversations (id, kind, name, last_at, last_preview) VALUES (?, ?, '', ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   kind = excluded.kind,
		   last_at = MAX(inbox_conversations.last_at, excluded.last_at),
		   last_preview = CASE
		     WHEN excluded.last_at >= inbox_conversations.last_at THEN excluded.last_preview
		     ELSE inbox_conversations.last_preview END`,
		m.ConvID, string(m.Kind), at, preview(m.Content),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// Conversations 按最后活跃时间倒序; 从没发过消息的种子会话排在最后。
func (s *Store) Conversations() ([]Conversation, error) {
	if !s.Ready() {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT id, kind, name, last_at, last_preview FROM inbox_conversations ORDER BY last_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Conversation, 0, 64)
	for rows.Next() {
		var c Conversation
		var kind string
		var at int64
		if err := rows.Scan(&c.ID, &kind, &c.Name, &at, &c.LastPreview); err != nil {
			return nil, err
		}
		c.Kind = Kind(kind)
		if at > 0 {
			c.LastAt = time.Unix(at, 0)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Lookup 按 id 取一个会话; 发送前用它确认目标存在。
func (s *Store) Lookup(convID string) (Conversation, bool) {
	if !s.Ready() || convID == "" {
		return Conversation{}, false
	}
	var c Conversation
	var kind string
	var at int64
	err := s.db.QueryRow(
		`SELECT id, kind, name, last_at, last_preview FROM inbox_conversations WHERE id = ?`, convID).
		Scan(&c.ID, &kind, &c.Name, &at, &c.LastPreview)
	if err != nil {
		return Conversation{}, false
	}
	c.Kind = Kind(kind)
	if at > 0 {
		c.LastAt = time.Unix(at, 0)
	}
	return c, true
}

// Messages 取某会话的历史, 按时间正序返回。before 为 0 表示从最新一条往前翻。
func (s *Store) Messages(convID string, before int64, limit int) ([]Message, error) {
	if !s.Ready() || convID == "" {
		return nil, nil
	}
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	query := `SELECT id, conv_id, direction, sender_id, sender_name, content, platform_msg_id, created_at
	          FROM inbox_messages WHERE conv_id = ?`
	args := []any{convID}
	if before > 0 {
		query += ` AND id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Message, 0, limit)
	for rows.Next() {
		var m Message
		var dir string
		var at int64
		if err := rows.Scan(&m.ID, &m.ConvID, &dir, &m.SenderID, &m.SenderName,
			&m.Content, &m.PlatformID, &at); err != nil {
			return nil, err
		}
		m.Direction = Direction(dir)
		m.CreatedAt = time.Unix(at, 0)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Reverse(out)
	return out, nil
}

// SetName 只在名字为空时写入: 群名改过也不必每次覆盖, 少一次写就少一次锁。
func (s *Store) SetName(convID, name string) error {
	if !s.Ready() || convID == "" || strings.TrimSpace(name) == "" {
		return nil
	}
	_, err := s.db.Exec(`UPDATE inbox_conversations SET name = ? WHERE id = ? AND name = ''`, name, convID)
	return err
}

func (s *Store) NameOf(convID string) string {
	if !s.Ready() || convID == "" {
		return ""
	}
	var name string
	if err := s.db.QueryRow(`SELECT name FROM inbox_conversations WHERE id = ?`, convID).Scan(&name); err != nil {
		return ""
	}
	return name
}

// Trim 按保留策略删旧消息。启动时跑一次就够, 没必要为了几天前的消息常驻一个定时器。
// Trim 删掉过期的, 再逐会话删掉超限的; 逐会话删而不是全表窗口, 启动开销才不会随总行数滚雪球。
func (s *Store) Trim() error {
	if !s.Ready() {
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM inbox_messages WHERE created_at < ?`,
		time.Now().Add(-maxAge).Unix()); err != nil {
		return err
	}
	rows, err := s.db.Query(
		`SELECT conv_id FROM inbox_messages GROUP BY conv_id HAVING COUNT(*) > ?`, maxPerConversation)
	if err != nil {
		return err
	}
	var full []string
	for rows.Next() {
		var conv string
		if err := rows.Scan(&conv); err != nil {
			rows.Close()
			return err
		}
		full = append(full, conv)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, conv := range full {
		if _, err := s.db.Exec(
			`DELETE FROM inbox_messages WHERE conv_id = ? AND id <= (
			   SELECT id FROM inbox_messages WHERE conv_id = ? ORDER BY id DESC LIMIT 1 OFFSET ?)`,
			conv, conv, maxPerConversation); err != nil {
			return err
		}
	}
	return nil
}

// Seed 把统计表里见过的群和用户导成初始会话。
// 平台不提供群/好友列表接口, 这是唯一能拿到"历史上有过往来"的线索。
func (s *Store) Seed() error {
	if !s.Ready() {
		return nil
	}
	for _, query := range []string{
		`INSERT OR IGNORE INTO inbox_conversations (id, kind, name, last_at, last_preview)
		 SELECT group_id, 'group', '', 0, '' FROM stats_groups`,
		`INSERT OR IGNORE INTO inbox_conversations (id, kind, name, last_at, last_preview)
		 SELECT peer_id, 'c2c', '', 0, '' FROM stats_peers`,
	} {
		if _, err := s.db.Exec(query); err != nil && !strings.Contains(err.Error(), "no such table") {
			return err
		}
	}
	return nil
}

// preview 把 markdown 压成一行摘要: 换行折叠成空格, 超长截断。
func preview(content string) string {
	text := strings.Join(strings.Fields(content), " ")
	runes := []rune(text)
	if len(runes) > previewRunes {
		return string(runes[:previewRunes]) + "…"
	}
	return text
}
