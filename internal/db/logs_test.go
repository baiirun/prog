package db

import (
	"context"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

func readLogs(t *testing.T, db *DB, itemID string) []model.Log {
	t.Helper()
	logs, err := db.Logs(context.Background(), itemID)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}
	return logs
}

func TestAddLog(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test")

	if err := db.AddLog(ctx, item.ID, "First log"); err != nil {
		t.Fatalf("failed to add log: %v", err)
	}
	if err := db.AddLog(ctx, item.ID, "Second log"); err != nil {
		t.Fatalf("failed to add second log: %v", err)
	}

	logs := readLogs(t, db, item.ID)
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].Message != "First log" {
		t.Errorf("first log = %q, want %q", logs[0].Message, "First log")
	}
	if logs[1].Message != "Second log" {
		t.Errorf("second log = %q, want %q", logs[1].Message, "Second log")
	}
	if logs[0].ItemID != item.ID {
		t.Errorf("log item = %q, want %q", logs[0].ItemID, item.ID)
	}
}

func TestLogs_Empty(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Test")

	if logs := readLogs(t, db, item.ID); len(logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(logs))
	}
}

func TestLogs_Order(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test")

	for _, msg := range []string{"First", "Second", "Third"} {
		if err := db.AddLog(ctx, item.ID, msg); err != nil {
			t.Fatalf("failed to add log: %v", err)
		}
	}

	logs := readLogs(t, db, item.ID)
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	// Chronological order.
	if logs[0].Message != "First" || logs[1].Message != "Second" || logs[2].Message != "Third" {
		t.Errorf("logs not in chronological order: %v", logs)
	}
}
