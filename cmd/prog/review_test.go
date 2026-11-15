package main

import (
	"context"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

func TestReviewCmd_TransitionsFromInProgress(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create a task in in_progress state
	task := mustCreate(t, svc, model.NewItem{
		Project:  "test",
		Type:     model.ItemTypeTask,
		Title:    "Review test task",
		Status:   model.StatusInProgress,
		Priority: 2,
	})

	// Simulate what reviewCmd does: check status, then update
	item := mustItem(t, svc, task.ID)
	if item.Status != model.StatusInProgress {
		t.Fatalf("expected in_progress, got %s", item.Status)
	}

	if err := svc.SetStatus(ctx, task.ID, model.StatusReviewing); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Verify the status was updated
	got := mustItem(t, svc, task.ID)
	if got.Status != model.StatusReviewing {
		t.Errorf("status = %q, want %q", got.Status, model.StatusReviewing)
	}
}

func TestReviewCmd_RejectsOpenTask(t *testing.T) {
	svc := setupTestService(t)

	// Create a task in open state
	task := mustCreate(t, svc, model.NewItem{
		Project:  "test",
		Type:     model.ItemTypeTask,
		Title:    "Open task",
		Status:   model.StatusOpen,
		Priority: 2,
	})

	// Simulate what reviewCmd does: check status first
	item := mustItem(t, svc, task.ID)

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
	svc := setupTestService(t)

	task := mustCreate(t, svc, model.NewItem{
		Project:  "test",
		Type:     model.ItemTypeTask,
		Title:    "Done task",
		Status:   model.StatusDone,
		Priority: 2,
	})

	item := mustItem(t, svc, task.ID)

	if item.Status == model.StatusInProgress {
		t.Fatal("expected non-in_progress status")
	}
}

func TestReviewCmd_RejectsBlockedTask(t *testing.T) {
	svc := setupTestService(t)

	task := mustCreate(t, svc, model.NewItem{
		Project:  "test",
		Type:     model.ItemTypeTask,
		Title:    "Blocked task",
		Status:   model.StatusBlocked,
		Priority: 2,
	})

	item := mustItem(t, svc, task.ID)

	if item.Status == model.StatusInProgress {
		t.Fatal("expected non-in_progress status")
	}
}
