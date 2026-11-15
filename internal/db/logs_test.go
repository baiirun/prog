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
