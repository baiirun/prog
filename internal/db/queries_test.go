package db

import (
	"testing"
	"time"

	"github.com/baiirun/dotworld-tasks/internal/model"
)

func createTestItemWithProject(t *testing.T, db *DB, title, project string, status model.Status, priority int) *model.Item {
	t.Helper()
	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   project,
		Type:      model.ItemTypeTask,
		Title:     title,
		Status:    status,
		Priority:  priority,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateItem(item); err != nil {
		t.Fatalf("failed to create item: %v", err)
	}
	return item
}

func TestListItems(t *testing.T) {
	db := setupTestDB(t)

	createTestItemWithProject(t, db, "Task 1", "proj1", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "Task 2", "proj1", model.StatusDone, 2)
	createTestItemWithProject(t, db, "Task 3", "proj2", model.StatusOpen, 2)

	// List all
	items, err := db.ListItems("", nil)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Filter by project
	items, _ = db.ListItems("proj1", nil)
	if len(items) != 2 {
		t.Errorf("expected 2 items for proj1, got %d", len(items))
	}

	// Filter by status
	status := model.StatusOpen
	items, _ = db.ListItems("", &status)
	if len(items) != 2 {
		t.Errorf("expected 2 open items, got %d", len(items))
	}

	// Filter by both
	items, _ = db.ListItems("proj1", &status)
	if len(items) != 1 {
		t.Errorf("expected 1 open item in proj1, got %d", len(items))
	}
}

func TestListItems_InvalidStatus(t *testing.T) {
	db := setupTestDB(t)

	status := model.Status("invalid")
	_, err := db.ListItems("", &status)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestListItems_OrderByPriority(t *testing.T) {
	db := setupTestDB(t)

	createTestItemWithProject(t, db, "Low", "test", model.StatusOpen, 3)
	createTestItemWithProject(t, db, "High", "test", model.StatusOpen, 1)
	createTestItemWithProject(t, db, "Medium", "test", model.StatusOpen, 2)

	items, _ := db.ListItems("test", nil)

	if items[0].Title != "High" {
		t.Errorf("first item should be High priority, got %q", items[0].Title)
	}
	if items[2].Title != "Low" {
		t.Errorf("last item should be Low priority, got %q", items[2].Title)
	}
}

func TestReadyItems(t *testing.T) {
	db := setupTestDB(t)

	// Create tasks with dependencies
	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "Task 3", "test", model.StatusInProgress, 2) // not ready (in_progress)

	// task2 depends on task1
	db.AddDep(task2.ID, task1.ID)

	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready: %v", err)
	}

	// Only task1 should be ready (task2 has unmet dep, task3 is in_progress)
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != task1.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, task1.ID)
	}

	// Complete task1, now task2 should be ready
	db.UpdateStatus(task1.ID, model.StatusDone)

	ready, _ = db.ReadyItems("test")
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item after completing dep, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != task2.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, task2.ID)
	}
}

func TestReadyItems_ProjectFilter(t *testing.T) {
	db := setupTestDB(t)

	createTestItemWithProject(t, db, "Task 1", "proj1", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "Task 2", "proj2", model.StatusOpen, 2)

	ready, _ := db.ReadyItems("proj1")
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item in proj1, got %d", len(ready))
	}

	ready, _ = db.ReadyItems("")
	if len(ready) != 2 {
		t.Errorf("expected 2 ready items total, got %d", len(ready))
	}
}

func TestProjectStatus(t *testing.T) {
	db := setupTestDB(t)

	createTestItemWithProject(t, db, "Open 1", "test", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "Open 2", "test", model.StatusOpen, 1)
	createTestItemWithProject(t, db, "In Progress", "test", model.StatusInProgress, 2)
	createTestItemWithProject(t, db, "Blocked", "test", model.StatusBlocked, 2)
	createTestItemWithProject(t, db, "Done", "test", model.StatusDone, 2)

	report, err := db.ProjectStatus("test")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if report.Open != 2 {
		t.Errorf("open = %d, want 2", report.Open)
	}
	if report.InProgress != 1 {
		t.Errorf("in_progress = %d, want 1", report.InProgress)
	}
	if report.Blocked != 1 {
		t.Errorf("blocked = %d, want 1", report.Blocked)
	}
	if report.Done != 1 {
		t.Errorf("done = %d, want 1", report.Done)
	}
	if report.Ready != 2 {
		t.Errorf("ready = %d, want 2", report.Ready)
	}

	if len(report.InProgItems) != 1 {
		t.Errorf("in_progress items = %d, want 1", len(report.InProgItems))
	}
	if len(report.BlockedItems) != 1 {
		t.Errorf("blocked items = %d, want 1", len(report.BlockedItems))
	}
	if len(report.RecentDone) != 1 {
		t.Errorf("recent done = %d, want 1", len(report.RecentDone))
	}
}

func TestProjectStatus_Empty(t *testing.T) {
	db := setupTestDB(t)

	report, err := db.ProjectStatus("empty")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if report.Open != 0 || report.Done != 0 || report.Ready != 0 {
		t.Error("expected all counts to be 0 for empty project")
	}
}
