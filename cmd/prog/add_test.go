package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

func TestAddCmd_ParentFlag(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create an epic to be the parent
	epic := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Parent Epic",
		Status:  model.StatusOpen,
	})

	// Create a task with --parent flag
	task := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Child Task",
		Status:  model.StatusOpen,
	})

	// Simulate what addCmd does with --parent flag
	if err := svc.SetParent(ctx, task.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	// Verify the parent was set
	got := mustItem(t, svc, task.ID)

	if got.ParentID == nil {
		t.Fatal("expected parent to be set")
	}
	if *got.ParentID != epic.ID {
		t.Errorf("parent = %q, want %q", *got.ParentID, epic.ID)
	}
}

func TestAddCmd_BlocksFlag(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create a task that will be blocked
	blockedTask := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Blocked Task",
		Status:  model.StatusOpen,
	})

	// Create a blocker task with --blocks flag
	blocker := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Blocker Task",
		Status:  model.StatusOpen,
	})

	// Simulate what addCmd does with --blocks flag
	// blockedTask depends on blocker (blocker blocks blockedTask)
	if err := svc.AddDep(ctx, blockedTask.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Verify the dependency was created
	g := mustGraph(t, svc)
	deps := g.DepsOf(blockedTask.ID)

	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0] != blocker.ID {
		t.Errorf("dep = %q, want %q", deps[0], blocker.ID)
	}

	// Verify blocker task shows as unmet dependency
	hasUnmet, err := g.List(tasks.ListFilter{Project: "test", HasBlockers: true})
	if err != nil {
		t.Fatalf("failed to list items with unmet deps: %v", err)
	}
	if !slices.ContainsFunc(hasUnmet, func(i model.Item) bool { return i.ID == blockedTask.ID }) {
		t.Error("expected blocked task to have unmet deps")
	}
}

func TestAddCmd_ParentFlag_InvalidParent(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create a task (not an epic)
	notEpic := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Not an Epic",
		Status:  model.StatusOpen,
	})

	// Create another task
	task := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Child Task",
		Status:  model.StatusOpen,
	})

	// Try to set a non-epic as parent - should fail
	err := svc.SetParent(ctx, task.ID, notEpic.ID)
	if err == nil {
		t.Fatal("expected error when setting non-epic as parent")
	}
	if !strings.Contains(err.Error(), "must be an epic") {
		t.Errorf("error should mention 'must be an epic', got: %v", err)
	}
}

func TestAddCmd_BlocksFlag_NonexistentTarget(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create a blocker task
	blocker := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Blocker",
		Status:  model.StatusOpen,
	})

	// Try to block a nonexistent task - should fail
	err := svc.AddDep(ctx, "nonexistent", blocker.ID)
	if err == nil {
		t.Error("expected error when blocking nonexistent task")
	}
}

func TestAddCmd_DescFlag(t *testing.T) {
	svc := setupTestService(t)

	// Create a task with description (simulates --desc flag)
	desc := "This is the task description"
	task := mustCreate(t, svc, model.NewItem{
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Task with Description",
		Description: desc,
		Status:      model.StatusOpen,
	})

	// Verify the description was set
	got := mustItem(t, svc, task.ID)

	if got.Description != desc {
		t.Errorf("description = %q, want %q", got.Description, desc)
	}
}

func TestAddCmd_BothFlags(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	// Create an epic
	epic := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Combo Epic",
		Status:  model.StatusOpen,
	})

	// Create a task to be blocked
	blockedTask := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Blocked Task",
		Status:  model.StatusOpen,
	})

	// Create a task with both --parent and --blocks
	task := mustCreate(t, svc, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Combo Task",
		Status:  model.StatusOpen,
	})

	// Simulate addCmd with both flags
	if err := svc.SetParent(ctx, task.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}
	if err := svc.AddDep(ctx, blockedTask.ID, task.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Verify parent
	g := mustGraph(t, svc)
	got, err := g.Item(task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.ParentID == nil || *got.ParentID != epic.ID {
		t.Error("parent not set correctly")
	}

	// Verify blocking relationship
	deps := g.DepsOf(blockedTask.ID)
	if len(deps) != 1 || deps[0] != task.ID {
		t.Error("blocking relationship not set correctly")
	}
}
