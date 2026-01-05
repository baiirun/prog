package db

import (
	"testing"
	"time"

	"github.com/baiirun/dotworld-tasks/internal/model"
)

func TestAddLog(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.CreateItem(item)

	if err := db.AddLog(item.ID, "First log"); err != nil {
		t.Fatalf("failed to add log: %v", err)
	}
	if err := db.AddLog(item.ID, "Second log"); err != nil {
		t.Fatalf("failed to add second log: %v", err)
	}

	logs, err := db.GetLogs(item.ID)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}

	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}

	if logs[0].Message != "First log" {
		t.Errorf("first log = %q, want %q", logs[0].Message, "First log")
	}
	if logs[1].Message != "Second log" {
		t.Errorf("second log = %q, want %q", logs[1].Message, "Second log")
	}
}

func TestGetLogs_Empty(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.CreateItem(item)

	logs, err := db.GetLogs(item.ID)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}

	if len(logs) != 0 {
		t.Errorf("expected 0 logs, got %d", len(logs))
	}
}

func TestGetLogs_Order(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.CreateItem(item)

	db.AddLog(item.ID, "First")
	db.AddLog(item.ID, "Second")
	db.AddLog(item.ID, "Third")

	logs, _ := db.GetLogs(item.ID)

	// Should be in chronological order
	if logs[0].Message != "First" || logs[2].Message != "Third" {
		t.Error("logs not in chronological order")
	}
}
