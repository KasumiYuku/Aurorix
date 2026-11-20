// Command migrate-to-pg 把旧 SQLite 库整库搬进 PostgreSQL, 并逐表校验。
package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type column struct {
	name    string
	typ     string
	notNull bool
	dflt    string
	pk      int
}

type index struct {
	name   string
	unique bool
	cols   []string
	where  string
}

func main() {
	sqlitePath := flag.String("sqlite", "bot.db", "旧 SQLite 库路径")
	pgURL := flag.String("postgres", "", "目标 PostgreSQL 连接串")
	dryRun := flag.Bool("dry-run", false, "只打印计划, 不写库")
	sample := flag.Int("sample", 200, "每表抽样比对的行数")
	flag.Parse()

	if err := run(*sqlitePath, *pgURL, *dryRun, *sample); err != nil {
		fmt.Fprintln(os.Stderr, "迁移失败:", err)
		os.Exit(1)
	}
}

func run(sqlitePath, pgURL string, dryRun bool, sample int) error {
	if _, err := os.Stat(sqlitePath); err != nil {
		return fmt.Errorf("读不到旧库 %s: %w", sqlitePath, err)
	}
	if pgURL == "" {
		return errors.New("必须给 -postgres 连接串")
	}

	src, err := sql.Open("sqlite", "file:"+sqlitePath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("打开旧库: %w", err)
	}
	defer src.Close()

	dst, err := sql.Open("pgx", pgURL)
	if err != nil {
		return fmt.Errorf("连接 PostgreSQL: %w", err)
	}
	defer dst.Close()
	ctx := context.Background()
	if err := dst.PingContext(ctx); err != nil {
		return fmt.Errorf("连不上 PostgreSQL: %w", err)
	}

	tables, err := listTables(ctx, src)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return errors.New("旧库里没有任何用户表")
	}
	fmt.Printf("旧库 %s: %d 张表\n", sqlitePath, len(tables))

	for _, table := range tables {
		cols, err := tableInfo(ctx, src, table)
		if err != nil {
			return err
		}
		indexes, err := tableIndexes(ctx, src, table)
		if err != nil {
			return err
		}
		identity, err := autoincrementColumn(ctx, src, table)
		if err != nil {
			return fmt.Errorf("读 %s 的自增信息: %w", table, err)
		}
		ddl := createTableSQL(table, cols, identity)
		if dryRun {
			fmt.Println(ddl + ";")
			for _, idx := range indexes {
				fmt.Println(createIndexSQL(table, idx) + ";")
			}
			continue
		}
		if _, err := dst.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("建表 %s: %w", table, err)
		}
		for _, idx := range indexes {
			if _, err := dst.ExecContext(ctx, createIndexSQL(table, idx)); err != nil {
				return fmt.Errorf("建索引 %s.%s: %w", table, idx.name, err)
			}
		}
		copied, err := copyRows(ctx, dst, src, table, cols)
		if err != nil {
			return err
		}
		if identity != "" {
			if err := syncIdentity(ctx, dst, table, identity); err != nil {
				return err
			}
			var next int64
			if err := dst.QueryRowContext(ctx,
				`SELECT nextval(pg_get_serial_sequence($1, $2))`, table, identity).Scan(&next); err != nil {
				return fmt.Errorf("%s.%s 的自增序列不可用: %w", table, identity, err)
			}
		}
		want, err := countRows(ctx, src, table)
		if err != nil {
			return err
		}
		got, err := countRows(ctx, dst, table)
		if err != nil {
			return err
		}
		status := "OK"
		if want != got || int64(copied) != want {
			status = fmt.Sprintf("行数不符 源=%d 搬=%d 目标=%d", want, copied, got)
		}
		fmt.Printf("  %-24s %8d 行  %s\n", table, got, status)
		if status != "OK" {
			return fmt.Errorf("表 %s 行数不一致", table)
		}
	}

	if dryRun {
		return nil
	}
	fmt.Println("开始抽样比对...")
	for _, table := range tables {
		cols, err := tableInfo(ctx, src, table)
		if err != nil {
			return err
		}
		diff, err := sampleDiff(ctx, dst, src, table, cols, sample)
		if err != nil {
			return err
		}
		if diff != "" {
			return fmt.Errorf("表 %s 内容不一致: %s", table, diff)
		}
		fmt.Printf("  %-24s 抽样一致\n", table)
	}
	if err := verifySchema(ctx, dst, src, tables); err != nil {
		return err
	}
	fmt.Println("迁移完成: 行数、抽样内容与表结构全部一致。")
	return nil
}

func verifySchema(ctx context.Context, dst, src *sql.DB, tables []string) error {
	for _, table := range tables {
		cols, err := tableInfo(ctx, src, table)
		if err != nil {
			return err
		}
		for _, c := range cols {
			var nullable string
			var def sql.NullString
			err := dst.QueryRowContext(ctx, `
				SELECT is_nullable, column_default FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
				table, c.name).Scan(&nullable, &def)
			if err != nil {
				return fmt.Errorf("%s.%s 在目标库里不存在: %w", table, c.name, err)
			}
			if (c.notNull || c.pk > 0) && nullable == "YES" {
				return fmt.Errorf("%s.%s 丢了 NOT NULL", table, c.name)
			}
			if c.dflt != "" && !def.Valid {
				return fmt.Errorf("%s.%s 丢了默认值 %s", table, c.name, c.dflt)
			}
		}
		idxes, err := tableIndexes(ctx, src, table)
		if err != nil {
			return err
		}
		for _, idx := range idxes {
			var n int
			if err := dst.QueryRowContext(ctx,
				`SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`,
				idx.name).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("%s 的索引 %s 没建出来", table, idx.name)
			}
		}
	}
	return nil
}

func listTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("列旧库的表: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func tableInfo(ctx context.Context, db *sql.DB, table string) ([]column, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("读 %s 的列: %w", table, err)
	}
	defer rows.Close()

	var cols []column
	for rows.Next() {
		var cid int
		var c column
		var dflt sql.NullString
		if err := rows.Scan(&cid, &c.name, &c.typ, &c.notNull, &dflt, &c.pk); err != nil {
			return nil, err
		}
		c.dflt = dflt.String
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("表 %s 没有任何列", table)
	}
	return cols, nil
}

func tableIndexes(ctx context.Context, db *sql.DB, table string) ([]index, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA index_list("+quoteIdent(table)+")")
	if err != nil {
		return nil, fmt.Errorf("读 %s 的索引: %w", table, err)
	}
	defer rows.Close()

	var out []index
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		if origin == "pk" {
			continue
		}
		cols, err := indexColumns(ctx, db, name)
		if err != nil {
			return nil, err
		}
		if len(cols) == 0 {
			continue
		}
		cond := ""
		if partial != 0 {
			cond, err = indexWhere(ctx, db, name)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, index{name: name, unique: unique != 0, cols: cols, where: cond})
	}
	return out, rows.Err()
}

func indexWhere(ctx context.Context, db *sql.DB, name string) (string, error) {
	var ddl sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&ddl); err != nil {
		return "", err
	}
	upper := strings.ToUpper(ddl.String)
	i := strings.Index(upper, " WHERE ")
	if i < 0 {
		return "", nil
	}
	return strings.TrimSpace(ddl.String[i+len(" WHERE "):]), nil
}

func indexColumns(ctx context.Context, db *sql.DB, name string) ([]string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA index_info("+quoteIdent(name)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	byPos := map[int]string{}
	for rows.Next() {
		var seqno, cid int
		var colName sql.NullString
		if err := rows.Scan(&seqno, &cid, &colName); err != nil {
			return nil, err
		}
		if colName.Valid {
			byPos[seqno] = colName.String
		}
	}
	positions := make([]int, 0, len(byPos))
	for p := range byPos {
		positions = append(positions, p)
	}
	sort.Ints(positions)
	for _, p := range positions {
		cols = append(cols, byPos[p])
	}
	return cols, rows.Err()
}

func pgDefault(d string) string {
	d = strings.TrimSpace(d)
	if d == "" || strings.EqualFold(d, "NULL") {
		return ""
	}
	switch strings.ToUpper(d) {
	case "CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP()", "DATETIME('NOW')":
		return "now()"
	}
	return d
}

var autoincRe = regexp.MustCompile(`(?i)([A-Za-z_][A-Za-z0-9_]*)\s+INTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT`)

func autoincrementColumn(ctx context.Context, db *sql.DB, table string) (string, error) {
	var ddl sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&ddl)
	if err != nil {
		return "", err
	}
	m := autoincRe.FindStringSubmatch(ddl.String)
	if len(m) < 2 {
		return "", nil
	}
	return m[1], nil
}

func syncIdentity(ctx context.Context, db *sql.DB, table, col string) error {
	_, err := db.ExecContext(ctx,
		`SELECT setval(pg_get_serial_sequence($1, $2),
		   COALESCE((SELECT max(`+quoteIdent(col)+`) FROM `+quoteIdent(table)+`), 0) + 1, false)`,
		table, col)
	if err != nil {
		return fmt.Errorf("同步 %s.%s 的自增序列: %w", table, col, err)
	}
	return nil
}

func createTableSQL(table string, cols []column, identity string) string {
	parts := make([]string, 0, len(cols)+1)
	var pk []column
	for _, c := range cols {
		def := quoteIdent(c.name) + " " + pgType(c.typ)
		if c.name == identity {
			def += " GENERATED BY DEFAULT AS IDENTITY"
		} else if d := pgDefault(c.dflt); d != "" {
			def += " DEFAULT " + d
		}
		if c.notNull || c.pk > 0 {
			def += " NOT NULL"
		}
		parts = append(parts, def)
		if c.pk > 0 {
			pk = append(pk, c)
		}
	}
	if len(pk) > 0 {
		sort.Slice(pk, func(i, j int) bool { return pk[i].pk < pk[j].pk })
		names := make([]string, len(pk))
		for i, c := range pk {
			names[i] = quoteIdent(c.name)
		}
		parts = append(parts, "PRIMARY KEY ("+strings.Join(names, ", ")+")")
	}
	return "CREATE TABLE IF NOT EXISTS " + quoteIdent(table) + " (\n\t" + strings.Join(parts, ",\n\t") + "\n)"
}

func createIndexSQL(table string, idx index) string {
	cols := make([]string, len(idx.cols))
	for i, c := range idx.cols {
		cols[i] = quoteIdent(c)
	}
	unique := ""
	if idx.unique {
		unique = "UNIQUE "
	}
	name := idx.name
	if name == "" {
		name = table + "_" + strings.Join(idx.cols, "_")
	}
	out := "CREATE " + unique + "INDEX IF NOT EXISTS " + quoteIdent(name) + " ON " + quoteIdent(table) +
		" (" + strings.Join(cols, ", ") + ")"
	if idx.where != "" {
		out += " WHERE " + idx.where
	}
	return out
}

var sqliteInt = regexp.MustCompile(`(?i)^(int|integer|bigint|smallint|tinyint|mediumint)`)

func pgType(sqliteType string) string {
	t := strings.ToUpper(strings.TrimSpace(sqliteType))
	switch {
	case t == "":
		return "TEXT"
	case sqliteInt.MatchString(t):
		return "BIGINT"
	case strings.HasPrefix(t, "REAL"), strings.HasPrefix(t, "DOUBLE"), strings.HasPrefix(t, "FLOAT"):
		return "DOUBLE PRECISION"
	case strings.HasPrefix(t, "BLOB"):
		return "BYTEA"
	case strings.HasPrefix(t, "NUMERIC"), strings.HasPrefix(t, "DECIMAL"):
		return "NUMERIC"
	case strings.HasPrefix(t, "DATE"), strings.HasPrefix(t, "TIME"):
		return "TIMESTAMPTZ"
	case strings.HasPrefix(t, "BOOL"):
		return "BOOLEAN"
	default:
		return "TEXT"
	}
}

const batchRows = 500

func copyRows(ctx context.Context, dst, src *sql.DB, table string, cols []column) (int, error) {
	names := make([]string, len(cols))
	flags := make([]string, len(cols))
	for i, c := range cols {
		names[i] = quoteIdent(c.name)
		flags[i] = fmt.Sprintf("$%d", i+1)
	}
	insert := "INSERT INTO " + quoteIdent(table) + " (" + strings.Join(names, ", ") + ") VALUES "

	rows, err := src.QueryContext(ctx, "SELECT * FROM "+quoteIdent(table))
	if err != nil {
		return 0, fmt.Errorf("读 %s: %w", table, err)
	}
	defer rows.Close()

	tx, err := dst.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	total := 0
	for {
		var values []any
		var placeholders []string
		read := 0
		for read < batchRows && rows.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return 0, fmt.Errorf("扫 %s: %w", table, err)
			}
			for _, v := range raw {
				values = append(values, normalizeForWrite(v))
			}
			rowFlags := make([]string, len(flags))
			for i := range flags {
				rowFlags[i] = fmt.Sprintf("$%d", len(values)-len(cols)+i+1)
			}
			placeholders = append(placeholders, "("+strings.Join(rowFlags, ", ")+")")
			read++
		}
		if read == 0 {
			break
		}
		if _, err := tx.ExecContext(ctx, insert+strings.Join(placeholders, ", "), values...); err != nil {
			return 0, fmt.Errorf("写 %s: %w", table, err)
		}
		total += read
		if err := rows.Err(); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

func normalizeForWrite(v any) any {
	switch x := v.(type) {
	case nil, int64, float64, string, []byte, bool:
		return x
	case time.Time:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func countRows(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteIdent(table)).Scan(&n)
	return n, err
}

var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.999999999Z07:00",
	time.RFC3339Nano,
}

func sampleDiff(ctx context.Context, dst, src *sql.DB, table string, cols []column, sample int) (string, error) {
	if sample <= 0 {
		return "", nil
	}
	var pkIdx []int
	for i, c := range cols {
		if c.pk > 0 {
			pkIdx = append(pkIdx, i)
		}
	}
	if len(pkIdx) == 0 {
		return "", nil
	}
	sort.Slice(pkIdx, func(a, b int) bool { return cols[pkIdx[a]].pk < cols[pkIdx[b]].pk })

	srcRows, err := scanAll(ctx, src,
		"SELECT * FROM "+quoteIdent(table)+" LIMIT "+strconv.Itoa(sample), nil)
	if err != nil {
		return "", err
	}

	where := make([]string, len(pkIdx))
	args := make([]any, len(pkIdx))
	for i, idx := range pkIdx {
		where[i] = quoteIdent(cols[idx].name) + " = $" + strconv.Itoa(i+1)
	}
	lookup := "SELECT * FROM " + quoteIdent(table) + " WHERE " + strings.Join(where, " AND ")

	for n, row := range srcRows {
		for i, idx := range pkIdx {
			args[i] = normalizeForWrite(row[idx])
		}
		dstRows, err := scanAll(ctx, dst, lookup, args)
		if err != nil {
			return "", fmt.Errorf("第 %d 行回查失败: %w", n+1, err)
		}
		if len(dstRows) == 0 {
			return fmt.Sprintf("第 %d 行在目标库里不存在 (主键 %v)", n+1, args), nil
		}
		got := dstRows[0]
		if len(row) != len(got) {
			return fmt.Sprintf("第 %d 行列数不同", n+1), nil
		}
		for j := range row {
			x := normalizeForCompare(row[j])
			y := normalizeForCompare(got[j])
			if x != y {
				return fmt.Sprintf("第 %d 行第 %s 列 源=%q 目标=%q", n+1, cols[j].name, x, y), nil
			}
		}
	}
	return "", nil
}

func scanAll(ctx context.Context, db *sql.DB, query string, args []any) ([][]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out [][]any
	for rows.Next() {
		raw := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, rows.Err()
}

func normalizeForCompare(v any) string {
	switch x := v.(type) {
	case nil:
		return "<null>"
	case []byte:
		return "0x" + hex.EncodeToString(x)
	case string:
		if t, ok := parseAnyTime(x); ok {
			return t.UTC().Format(time.RFC3339Nano)
		}
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

func parseAnyTime(s string) (time.Time, bool) {
	if len(s) < 10 {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
