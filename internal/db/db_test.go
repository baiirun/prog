package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/baiirun/dotworld-tasks/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	if err := db.Init(); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Should create parent directories
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("failed to get default path: %v", err)
	}

	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}

	if !contains(path, ".world/tasks/tasks.db") {
		t.Errorf("expected path to contain .world/tasks/tasks.db, got %q", path)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s[1:], substr))
}

func TestCreateItem(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test task",
		Status:    model.StatusOpen,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.CreateItem(item); err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	// Verify it was created
	got, err := db.GetItem(item.ID)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}

	if got.Title != item.Title {
		t.Errorf("title = %q, want %q", got.Title, item.Title)
	}
	if got.Project != item.Project {
		t.Errorf("project = %q, want %q", got.Project, item.Project)
	}
}

func TestCreateItem_InvalidType(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:      "ts-123456",
		Project: "test",
		Type:    model.ItemType("invalid"),
		Title:   "Test",
		Status:  model.StatusOpen,
	}

	err := db.CreateItem(item)
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestCreateItem_InvalidStatus(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:      "ts-123456",
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Test",
		Status:  model.Status("invalid"),
	}

	err := db.CreateItem(item)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestGetItem_NotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetItem("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent item")
	}
}

func TestUpdateStatus(t *testing.T) {
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

	if err := db.UpdateStatus(item.ID, model.StatusInProgress); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	got, _ := db.GetItem(item.ID)
	if got.Status != model.StatusInProgress {
		t.Errorf("status = %q, want %q", got.Status, model.StatusInProgress)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.UpdateStatus("nonexistent", model.StatusDone)
	if err == nil {
		t.Error("expected error for nonexistent item")
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	db := setupTestDB(t)

	err := db.UpdateStatus("ts-123456", model.Status("invalid"))
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestAppendDescription(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:          model.GenerateID(model.ItemTypeTask),
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Test",
		Description: "Initial",
		Status:      model.StatusOpen,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.CreateItem(item)

	if err := db.AppendDescription(item.ID, "Appended text"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	got, _ := db.GetItem(item.ID)
	if got.Description == "Initial" {
		t.Error("description was not appended")
	}
}
