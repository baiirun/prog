package main

import (
	"strings"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
)

func TestAddCmd_ParentFlag(t *testing.T) {
	database := setupTestDB(t)

	// Create an epic to be the parent
	epic := &model.Item{
		ID:        "ep-parent",
		Project:   "test",
		Type:      model.ItemTypeEpic,
		Title:     "Parent Epic",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(epic); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create a task with --parent flag
	task := &model.Item{
		ID:        "ts-child1",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Child Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Simulate what addCmd does with --parent flag
	if err := database.SetParent(task.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	// Verify the parent was set
	got, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if got.ParentID == nil {
		t.Fatal("expected parent to be set")
	}
	if *got.ParentID != epic.ID {
		t.Errorf("parent = %q, want %q", *got.ParentID, epic.ID)
	}
}

func TestAddCmd_BlocksFlag(t *testing.T) {
	database := setupTestDB(t)

	// Create a task that will be blocked
	blockedTask := &model.Item{
		ID:        "ts-blocked",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Blocked Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(blockedTask); err != nil {
		t.Fatalf("failed to create blocked task: %v", err)
	}

	// Create a blocker task with --blocks flag
	blocker := &model.Item{
		ID:        "ts-blocker",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Blocker Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Simulate what addCmd does with --blocks flag
	// blockedTask depends on blocker (blocker blocks blockedTask)
	if err := database.AddDep(blockedTask.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Verify the dependency was created
	deps, err := database.GetDeps(blockedTask.ID)
	if err != nil {
		t.Fatalf("failed to get deps: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0] != blocker.ID {
		t.Errorf("dep = %q, want %q", deps[0], blocker.ID)
	}

	// Verify blocker task shows as unmet dependency
	hasUnmet, err := database.HasUnmetDeps(blockedTask.ID)
	if err != nil {
		t.Fatalf("failed to check unmet deps: %v", err)
	}
	if !hasUnmet {
		t.Error("expected blocked task to have unmet deps")
	}
}

func TestAddCmd_ParentFlag_InvalidParent(t *testing.T) {
	database := setupTestDB(t)

	// Create a task (not an epic)
	notEpic := &model.Item{
		ID:        "ts-notepic",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Not an Epic",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(notEpic); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create another task
	task := &model.Item{
		ID:        "ts-child2",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Child Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Try to set a non-epic as parent - should fail
	err := database.SetParent(task.ID, notEpic.ID)
	if err == nil {
		t.Error("expected error when setting non-epic as parent")
	}
	if !strings.Contains(err.Error(), "must be an epic") {
		t.Errorf("error should mention 'must be an epic', got: %v", err)
	}
}

func TestAddCmd_BlocksFlag_NonexistentTarget(t *testing.T) {
	database := setupTestDB(t)

	// Create a blocker task
	blocker := &model.Item{
		ID:        "ts-blocker2",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Blocker",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(blocker); err != nil {
		t.Fatalf("failed to create blocker: %v", err)
	}

	// Try to block a nonexistent task - should fail
	err := database.AddDep("nonexistent", blocker.ID)
	if err == nil {
		t.Error("expected error when blocking nonexistent task")
	}
}

func TestAddCmd_DescFlag(t *testing.T) {
	database := setupTestDB(t)

	// Create a task with description (simulates --desc flag)
	desc := "This is the task description"
	task := &model.Item{
		ID:          "ts-withdesc",
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Task with Description",
		Description: desc,
		Status:      model.StatusOpen,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Verify the description was set
	got, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if got.Description != desc {
		t.Errorf("description = %q, want %q", got.Description, desc)
	}
}

func TestAddCmd_BothFlags(t *testing.T) {
	database := setupTestDB(t)

	// Create an epic
	epic := &model.Item{
		ID:        "ep-combo",
		Project:   "test",
		Type:      model.ItemTypeEpic,
		Title:     "Combo Epic",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(epic); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}

	// Create a task to be blocked
	blockedTask := &model.Item{
		ID:        "ts-blocked2",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Blocked Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(blockedTask); err != nil {
		t.Fatalf("failed to create blocked task: %v", err)
	}

	// Create a task with both --parent and --blocks
	task := &model.Item{
		ID:        "ts-combo",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Combo Task",
		Status:    model.StatusOpen,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Simulate addCmd with both flags
	if err := database.SetParent(task.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}
	if err := database.AddDep(blockedTask.ID, task.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Verify parent
	got, err := database.GetItem(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != epic.ID {
		t.Error("parent not set correctly")
	}

	// Verify blocking relationship
	deps, err := database.GetDeps(blockedTask.ID)
	if err != nil {
		t.Fatalf("failed to get deps: %v", err)
	}
	if len(deps) != 1 || deps[0] != task.ID {
		t.Error("blocking relationship not set correctly")
	}
}
