package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

const (
	ScopeGlobal  = "global"
	ScopePlugin  = "plugin"
	ScopeCommand = "command"
	ScopeUser    = "user"
	ScopeGroup   = "group"

	defaultDBPath = "bot.db"

	maxConns = 4
)

var pragmas = []string{
	"busy_timeout(10000)",
	"journal_mode(WAL)",
	"synchronous=NORMAL",
}

var pathEscaper = strings.NewReplacer("?", "%3f", "#", "%23")

var (
	db     *sql.DB
	dbPath = defaultDBPath
	dbMu   sync.RWMutex
	// opened 记"有人真的 Open 过": 在那之前取库一律失败(见 errBeforeOpen)。
	opened atomic.Bool
)

// errBeforeOpen 是"还没 Open 就取库"的那一个错。
//
// 从前这里会顺手在**工作目录**开一份默认 bot.db: 插件的配置回调是在各自 init() 里注册的, 而注册即回调
// (见插件侧的 settings.OnChange) —— 于是总有插件在框架打开库之前碰库, 那份空库就这么被建出来, 里面
// 写进去的东西随后被真库顶掉, 部署目录里只剩下一个来路不明的文件。宁可明确报错。
//
// 这不是运行期错误, 是装配顺序问题: 看到它就说明有组件在自己的 init 里读了库, 该改的是那个调用点。
var errBeforeOpen = errors.New("SQLite 尚未打开: 取库发生在框架 storage.Open 之前(通常是某个插件在自己的 init() 里读了库); 不在工作目录另开一份默认库")

// Store is a key-value namespace bound to a specific data scope.
type Store struct {
	scope     string
	namespace string
}

// Open opens the SQLite database at path.
func Open(path string) error {
	if path == "" {
		path = defaultDBPath
	}
	path = canonicalPath(path)

	dbMu.Lock()
	defer dbMu.Unlock()

	if db != nil {
		if sameFile(dbPath, path) {
			return nil
		}
		if err := db.Close(); err != nil {
			return fmt.Errorf("close previous sqlite database: %w", err)
		}
		db = nil
	}

	dbPath = path
	if err := openLocked(path); err != nil {
		return err
	}
	opened.Store(true)
	return nil
}

func canonicalPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func Close() error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	dbPath = defaultDBPath
	// 关掉之后不再算"打开过": 其后若还有谁取库(收尾期间的定时任务/在飞任务), 应当明确报错, 而不是
	// 顺手在工作目录再开一份默认库。
	opened.Store(false)
	return err
}

func dsnFor(path string) string {
	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	return "file:" + pathEscaper.Replace(path) + "?" + q.Encode()
}

func openLocked(path string) error {
	opened, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}

	opened.SetMaxOpenConns(maxConns)
	opened.SetMaxIdleConns(maxConns)
	opened.SetConnMaxLifetime(0)

	if err = opened.Ping(); err != nil {
		opened.Close()
		return fmt.Errorf("connect to sqlite database: %w", err)
	}

	if _, err = opened.Exec(`
		CREATE TABLE IF NOT EXISTS kv_data (
			scope TEXT NOT NULL,
			namespace TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (scope, namespace, key)
		)
	`); err != nil {
		opened.Close()
		return fmt.Errorf("initialize sqlite database: %w", err)
	}

	db = opened
	return nil
}

func Global() *Store {
	return &Store{scope: ScopeGlobal, namespace: ScopeGlobal}
}

func Plugin(pluginID string) *Store {
	return &Store{scope: ScopePlugin, namespace: pluginID}
}

func Command(pluginID, commandID string) *Store {
	return &Store{scope: ScopeCommand, namespace: pluginID + ":" + commandID}
}

func User(userID string) *Store {
	return &Store{scope: ScopeUser, namespace: userID}
}

func Group(groupID string) *Store {
	return &Store{scope: ScopeGroup, namespace: groupID}
}

func (store *Store) Set(key string, value any) error {
	if err := store.validate(key); err != nil {
		return err
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal storage value: %w", err)
	}
	database, err := ensureDB()
	if err != nil {
		return err
	}

	_, err = database.Exec(`
		INSERT INTO kv_data (scope, namespace, key, value, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, store.scope, store.namespace, key, encoded)
	if err != nil {
		return fmt.Errorf("set storage value: %w", err)
	}
	return nil
}

func (store *Store) Get(key string, target any) (bool, error) {
	if err := store.validate(key); err != nil {
		return false, err
	}
	if target == nil {
		return false, errors.New("storage target cannot be nil")
	}

	database, err := ensureDB()
	if err != nil {
		return false, err
	}

	var encoded []byte
	err = database.QueryRow(
		"SELECT value FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?",
		store.scope, store.namespace, key,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get storage value: %w", err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return false, fmt.Errorf("unmarshal storage value: %w", err)
	}
	return true, nil
}

func (store *Store) Has(key string) (bool, error) {
	if err := store.validate(key); err != nil {
		return false, err
	}
	database, err := ensureDB()
	if err != nil {
		return false, err
	}

	var exists int
	err = database.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?)",
		store.scope, store.namespace, key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check storage value: %w", err)
	}
	return exists == 1, nil
}

// incrMu 串行化自增。自增是「读-改-写」, 而 SQLite 的读锁升级在并发下可能直接返回 BUSY,
// 光靠 busy_timeout 挡不住; 本进程内互斥即可 —— 框架本身按单实例运行。
var incrMu sync.Mutex

// Incr 把整数计数加上 delta 并返回新值; 键不存在时从 0 起算。
// 读取与写回在同一个事务里完成, 并发点击同一个按钮不会丢计数;
// 键上存的不是整数时明确报错, 而不是悄悄从 0 重来。
func (store *Store) Incr(key string, delta int64) (int64, error) {
	if err := store.validate(key); err != nil {
		return 0, err
	}
	database, err := ensureDB()
	if err != nil {
		return 0, err
	}

	incrMu.Lock()
	defer incrMu.Unlock()

	tx, err := database.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin incr: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current int64
	var encoded []byte
	err = tx.QueryRow(
		"SELECT value FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?",
		store.scope, store.namespace, key,
	).Scan(&encoded)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		current = 0
	case err != nil:
		return 0, fmt.Errorf("read incr value: %w", err)
	default:
		if err := json.Unmarshal(encoded, &current); err != nil {
			return 0, fmt.Errorf("键 %q 存的不是整数, 无法自增: %w", key, err)
		}
	}

	next := current + delta
	if encoded, err = json.Marshal(next); err != nil {
		return 0, fmt.Errorf("marshal incr value: %w", err)
	}
	if _, err = tx.Exec(`
		INSERT INTO kv_data (scope, namespace, key, value, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, store.scope, store.namespace, key, encoded); err != nil {
		return 0, fmt.Errorf("write incr value: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit incr: %w", err)
	}
	return next, nil
}

func (store *Store) Delete(key string) error {
	if err := store.validate(key); err != nil {
		return err
	}
	database, err := ensureDB()
	if err != nil {
		return err
	}

	if _, err = database.Exec(
		"DELETE FROM kv_data WHERE scope = ? AND namespace = ? AND key = ?",
		store.scope, store.namespace, key,
	); err != nil {
		return fmt.Errorf("delete storage value: %w", err)
	}
	return nil
}

func (store *Store) Clear() error {
	if store == nil || store.scope == "" || store.namespace == "" {
		return errors.New("invalid storage namespace")
	}
	database, err := ensureDB()
	if err != nil {
		return err
	}

	if _, err = database.Exec(
		"DELETE FROM kv_data WHERE scope = ? AND namespace = ?",
		store.scope, store.namespace,
	); err != nil {
		return fmt.Errorf("clear storage namespace: %w", err)
	}
	return nil
}

// DB 返回底层 SQLite 连接池, 供框架内部与插件扩展表使用 (如 stats / 各插件的 repo)。
func DB() (*sql.DB, error) {
	return ensureDB()
}

func (store *Store) validate(key string) error {
	if store == nil || store.scope == "" || store.namespace == "" {
		return errors.New("invalid storage namespace")
	}
	if key == "" {
		return errors.New("storage key cannot be empty")
	}
	return nil
}

func ensureDB() (*sql.DB, error) {
	dbMu.RLock()
	if db != nil {
		database := db
		dbMu.RUnlock()
		return database, nil
	}
	dbMu.RUnlock()

	dbMu.Lock()
	defer dbMu.Unlock()
	if db != nil {
		return db, nil
	}
	if !opened.Load() {
		return nil, errBeforeOpen
	}
	if dbPath == "" {
		dbPath = defaultDBPath
	}
	if err := openLocked(dbPath); err != nil {
		return nil, err
	}
	return db, nil
}
