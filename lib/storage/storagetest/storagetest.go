// Package storagetest 给每个测试一个独占的 PostgreSQL schema, 跑完即删。
package storagetest

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultURL = "postgres://postgres@127.0.0.1:5432/aurorix?sslmode=disable"

var seq atomic.Int64

// T 同时覆盖 *testing.T 与 *testing.B。
type T interface {
	Helper()
	Cleanup(func())
	Fatalf(format string, args ...any)
	Skip(args ...any)
}

// DSN 返回一条连到新建 schema 的连接串; 没配 AURORIX_TEST_DATABASE 又连不上默认库时跳过用例, 显式配了却连不上才算真错。
func DSN(t T) string {
	t.Helper()
	base := os.Getenv("AURORIX_TEST_DATABASE")
	configured := base != ""
	if !configured {
		base = defaultURL
	}

	schema := fmt.Sprintf("t_%d_%d", os.Getpid(), seq.Add(1))
	admin, err := sql.Open("pgx", base)
	if err != nil {
		return unavailable(t, configured, base, err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		admin.Close()
		return unavailable(t, configured, base, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = admin.Close()
	})

	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + "search_path=" + schema
}

func unavailable(t T, configured bool, base string, err error) string {
	if !configured {
		t.Skip("跳过需要 PostgreSQL 的用例: 没配 AURORIX_TEST_DATABASE, 且连不上 " + base + "(" + err.Error() + ")")
	}
	t.Fatalf("建测试 schema 失败, 把 AURORIX_TEST_DATABASE 指向一个可连的库: %v", err)
	return ""
}
