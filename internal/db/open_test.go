package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
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
		if fk != 1 || busyMS <= 0 || busyMS != busyTimeoutMS {
			t.Errorf("connection %d: foreign_keys=%d busy_timeout=%d, want 1 and %d", i, fk, busyMS, busyTimeoutMS)
		}
	}
}

// Protects against silently opening a different file: the driver treats
// everything after '?' as connection parameters, and an empty path would put
// the parameters themselves in the file name.
func TestOpen_RejectsPathsTheDriverWouldMisread(t *testing.T) {
	for _, path := range []string{filepath.Join(t.TempDir(), "what?.db"), ""} {
		if _, err := Open(path); err == nil {
			t.Errorf("Open(%q) succeeded, want error", path)
		}
	}
}

// Protects pragma order in the DSN: switching a file to WAL needs a lock, so
// busy_timeout must already be set or Open fails while another process is
// writing (the "database is locked" errors seen with the aetherflow daemon).
// The holder takes an EXCLUSIVE lock: against a RESERVED one SQLite returns
// SQLITE_BUSY without waiting, to avoid deadlock, whatever the timeout.
func TestOpen_WaitsForAnotherWriter(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "busy.db")
	raw, err := sql.Open("sqlite", path) // no pragmas, so still in rollback-journal mode
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	holder, err := raw.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if _, err := holder.ExecContext(ctx, "CREATE TABLE t (x)"); err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = holder.ExecContext(ctx, "COMMIT")
	}()

	d, err := Open(path)
	if err != nil {
		t.Fatalf("Open while another connection held the write lock: %v", err)
	}
	_ = d.Close()
}
