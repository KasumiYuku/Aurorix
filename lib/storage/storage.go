// Package storage 把插件数据存进 PostgreSQL, 按命名空间隔离。
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/constant"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	ScopeGlobal  = "global"
	ScopePlugin  = "plugin"
	ScopeCommand = "command"
	ScopeUser    = "user"

	maxConns = 16
)

const schema = `
CREATE TABLE IF NOT EXISTS kv_data (
	scope      TEXT NOT NULL,
	namespace  TEXT NOT NULL,
	key        TEXT NOT NULL,
	value      BYTEA NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (scope, namespace, key)
)`

var (
	db     *sql.DB
	dsn    string
	dbMu   sync.RWMutex
	opened atomic.Bool
)

var errBeforeOpen = errors.New("storage: 取库发生在 storage.Open 之前(通常是某个插件在自己的 init() 里读了库)")

// Store 绑定到某个数据作用域的键值命名空间。
type Store struct {
	scope     string
	namespace string
}

// Open 连接数据库并确保表结构存在; 连接串与当前一致时是空操作。
func Open(databaseURL string) error {
	if databaseURL == "" {
		return errors.New("storage: 数据库连接串为空")
	}

	dbMu.Lock()
	defer dbMu.Unlock()

	if db != nil {
		if dsn == databaseURL {
			return nil
		}
		if err := db.Close(); err != nil {
			return fmt.Errorf("close previous database: %w", err)
		}
		db = nil
	}

	dsn = databaseURL
	if err := openLocked(databaseURL); err != nil {
		return err
	}
	opened.Store(true)
	return nil
}

func openLocked(databaseURL string) error {
	pool, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}

	pool.SetMaxOpenConns(maxConns)
	pool.SetMaxIdleConns(maxConns / 2)
	pool.SetConnMaxLifetime(time.Hour)

	if err := pool.Ping(); err != nil {
		pool.Close()
		return fmt.Errorf("connect to postgres: %w", err)
	}
	if _, err := pool.Exec(schema); err != nil {
		pool.Close()
		return fmt.Errorf("initialize schema: %w", err)
	}

	db = pool
	return nil
}

func Close() error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	dsn = ""
	opened.Store(false)
	return err
}

// DB 返回连接池, 供框架内部与插件扩展表使用。
func DB() (*sql.DB, error) {
	return ensureDB()
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

// Scene 会话数据命名空间: 群/单聊/子频道/频道私信的键互不相通, 也不与用户维度混。
func Scene(scene constant.Scene, id string) *Store {
	return &Store{scope: string(scene), namespace: id}
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
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = now()
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
		"SELECT value FROM kv_data WHERE scope = $1 AND namespace = $2 AND key = $3",
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

	var exists bool
	err = database.QueryRow(
		"SELECT EXISTS (SELECT 1 FROM kv_data WHERE scope = $1 AND namespace = $2 AND key = $3)",
		store.scope, store.namespace, key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check storage value: %w", err)
	}
	return exists, nil
}

// Incr 原子地把整数加上 delta; 用事务级 advisory lock 串行化同一个键, 多实例部署也成立。
func (store *Store) Incr(key string, delta int64) (int64, error) {
	if err := store.validate(key); err != nil {
		return 0, err
	}
	database, err := ensureDB()
	if err != nil {
		return 0, err
	}

	ctx := context.Background()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin incr: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	lockKey := store.scope + "\x1f" + store.namespace + "\x1f" + key
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return 0, fmt.Errorf("lock incr key: %w", err)
	}

	var current int64
	var encoded []byte
	err = tx.QueryRowContext(ctx,
		"SELECT value FROM kv_data WHERE scope = $1 AND namespace = $2 AND key = $3",
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
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO kv_data (scope, namespace, key, value, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (scope, namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = now()
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
		"DELETE FROM kv_data WHERE scope = $1 AND namespace = $2 AND key = $3",
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
		"DELETE FROM kv_data WHERE scope = $1 AND namespace = $2",
		store.scope, store.namespace,
	); err != nil {
		return fmt.Errorf("clear storage namespace: %w", err)
	}
	return nil
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
	if !opened.Load() || dsn == "" {
		return nil, errBeforeOpen
	}
	if err := openLocked(dsn); err != nil {
		return nil, err
	}
	return db, nil
}
