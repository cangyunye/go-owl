//go:build !duckdb
// +build !duckdb

package history

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// 回归(P2-6 并发演练):PRAGMA 必须经 DSN 注入,让连接池的每个连接都带上
// busy_timeout/WAL。仅对新连接 Exec PRAGMA 只作用于单个池化连接,后续连接
// 没有 busy_timeout,与 serve/monitor 并发写同一个 owl.db 时会立即报
// SQLITE_BUSY(2026-09-10 演练实测 CLI 写失败率 ~14%,39189 次 API 写入
// 因带 busy_timeout 而零失败)。
func TestNewDB_PragmasApplyToEveryConnection(t *testing.T) {
	db, err := NewDB(&Config{Enabled: true, DBPath: filepath.Join(t.TempDir(), "pragma.db")})
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conn := db.Connection()

	ctx := context.Background()
	c1, err := conn.Conn(ctx)
	if err != nil {
		t.Fatalf("conn1: %v", err)
	}
	defer c1.Close()
	c2, err := conn.Conn(ctx)
	if err != nil {
		t.Fatalf("conn2: %v", err)
	}
	defer c2.Close()

	queryPragma := func(c *sql.Conn, pragma string) (string, error) {
		rows, err := c.QueryContext(ctx, pragma)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return "", err
			}
			return "", sql.ErrNoRows
		}
		var v string
		err = rows.Scan(&v)
		return v, err
	}

	for i, c := range []*sql.Conn{c1, c2} {
		if journal, err := queryPragma(c, "PRAGMA journal_mode"); err != nil {
			t.Fatalf("conn %d journal_mode: %v", i, err)
		} else if journal != "wal" {
			t.Errorf("conn %d journal_mode = %q, want wal", i, journal)
		}
		if busy, err := queryPragma(c, "PRAGMA busy_timeout"); err != nil {
			t.Fatalf("conn %d busy_timeout: %v", i, err)
		} else if busy != "5000" {
			t.Errorf("conn %d busy_timeout = %q, want 5000", i, busy)
		}
		if fk, err := queryPragma(c, "PRAGMA foreign_keys"); err != nil {
			t.Fatalf("conn %d foreign_keys: %v", i, err)
		} else if fk != "1" {
			t.Errorf("conn %d foreign_keys = %q, want 1", i, fk)
		}
	}
}
