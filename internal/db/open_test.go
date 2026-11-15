package db

import (
	"context"
	"path/filepath"
	"testing"
)

// Protects connection setup: SQLite applies foreign_keys and busy_timeout per
// connection, and database/sql opens connections lazily, so every connection
// in the pool must get them, not just the first.
func TestOpen_EveryPooledConnectionHasPragmas(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	const n = 4
	for i := range n {
		c, err := d.conn.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// Hold every connection so the pool must open n distinct ones.
		defer func() { _ = c.Close() }()
		var fk, busyMS int
		if err := c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Fatal(err)
		}
		if err := c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyMS); err != nil {
			t.Fatal(err)
		}
		if fk != 1 || busyMS != busyTimeoutMS {
			t.Errorf("connection %d: foreign_keys=%d busy_timeout=%d, want 1 and %d", i, fk, busyMS, busyTimeoutMS)
		}
	}
}

// Protects against silently opening a different file: the driver treats
// everything after '?' as connection parameters.
func TestOpen_RejectsPathWithQuestionMark(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "what?.db")); err == nil {
		t.Fatal("Open accepted a path containing '?'")
	}
}
