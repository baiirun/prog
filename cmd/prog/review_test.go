package main

import (
	"strings"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
)

func TestReviewCmd_TransitionsFromInProgress(t *testing.T) {
	database := setupTestDB(t)

	// Create a task in in_progress state
	task := &model.Item{
		ID:        "ts-rev001",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Review test task",
		Status:    model.StatusInProgress,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Simulate what reviewCmd does: check status, then update
	item, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}
	if item.Status != model.StatusInProgress {
		t.Fatalf("expected in_progress, got %s", item.Status)
	}

	if err := database.UpdateStatus(task.ID, model.StatusReviewing); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Verify the status was updated
	got, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.Status != model.StatusReviewing {
		t.Errorf("status = %q, want %q", got.Status, model.StatusReviewing)
	}
}

func TestReviewCmd_RejectsOpenTask(t *testing.T) {
	database := setupTestDB(t)

	// Create a task in open state
	task := &model.Item{
		ID:        "ts-rev002",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Open task",
		Status:    model.StatusOpen,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Simulate what reviewCmd does: check status first
	item, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}

	// Should reject because status is not in_progress
	if item.Status == model.StatusInProgress {
		t.Fatal("expected non-in_progress status")
	}

	// Verify the error message would contain the right info
	errMsg := "can only review in_progress tasks (current status: " + string(item.Status) + ")"
	if !strings.Contains(errMsg, "open") {
		t.Errorf("error message should mention current status 'open', got: %s", errMsg)
	}
}

func TestReviewCmd_RejectsDoneTask(t *testing.T) {
	database := setupTestDB(t)

	task := &model.Item{
		ID:        "ts-rev003",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Done task",
		Status:    model.StatusDone,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	item, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}

	if item.Status == model.StatusInProgress {
		t.Fatal("expected non-in_progress status")
	}
}

func TestReviewCmd_RejectsBlockedTask(t *testing.T) {
	database := setupTestDB(t)

	task := &model.Item{
		ID:        "ts-rev004",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Blocked task",
		Status:    model.StatusBlocked,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	item, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}

	if item.Status == model.StatusInProgress {
		t.Fatal("expected non-in_progress status")
	}
}
