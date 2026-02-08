package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
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
	if err := db.AddDep(task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

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
	if err := db.UpdateStatus(task1.ID, model.StatusDone); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	ready, _ = db.ReadyItems("test")
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item after completing dep, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != task2.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, task2.ID)
	}
}

func TestReadyItems_CanceledBlockerUnblocks(t *testing.T) {
	db := setupTestDB(t)

	blocker := createTestItemWithProject(t, db, "Blocker", "test", model.StatusOpen, 2)
	task := createTestItemWithProject(t, db, "Blocked Task", "test", model.StatusOpen, 1)

	if err := db.AddDep(task.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Task should not be ready (blocked by open blocker)
	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready: %v", err)
	}
	readyIDs := map[string]bool{}
	for _, r := range ready {
		readyIDs[r.ID] = true
	}
	if readyIDs[task.ID] {
		t.Error("task should not be ready while blocker is open")
	}

	// Cancel the blocker — task should now be ready
	if err := db.UpdateStatus(blocker.ID, model.StatusCanceled); err != nil {
		t.Fatalf("failed to cancel blocker: %v", err)
	}

	ready, err = db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready after cancel: %v", err)
	}
	readyIDs = map[string]bool{}
	for _, r := range ready {
		readyIDs[r.ID] = true
	}
	if !readyIDs[task.ID] {
		t.Error("task should be ready after blocker is canceled")
	}
}

func TestListItemsFiltered_CanceledBlockerResolvesHasBlockers(t *testing.T) {
	db := setupTestDB(t)

	blocker := createTestItemWithProject(t, db, "Blocker", "test", model.StatusOpen, 2)
	task := createTestItemWithProject(t, db, "Blocked Task", "test", model.StatusOpen, 1)

	if err := db.AddDep(task.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Task should appear in has-blockers list
	items, err := db.ListItemsFiltered(ListFilter{HasBlockers: true})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 || items[0].ID != task.ID {
		t.Errorf("expected task in has-blockers list, got %v", items)
	}

	// Cancel the blocker
	if err := db.UpdateStatus(blocker.ID, model.StatusCanceled); err != nil {
		t.Fatalf("failed to cancel: %v", err)
	}

	// Task should no longer appear in has-blockers list
	items, err = db.ListItemsFiltered(ListFilter{HasBlockers: true})
	if err != nil {
		t.Fatalf("failed to list after cancel: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items with blockers after cancel, got %d", len(items))
	}

	// Task should appear in no-blockers list
	items, err = db.ListItemsFiltered(ListFilter{NoBlockers: true})
	if err != nil {
		t.Fatalf("failed to list no-blockers: %v", err)
	}
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[task.ID] {
		t.Error("task should appear in no-blockers list after blocker is canceled")
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

func createTestEpic(t *testing.T, db *DB, title, project string) *model.Item {
	t.Helper()
	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeEpic),
		Project:   project,
		Type:      model.ItemTypeEpic,
		Title:     title,
		Status:    model.StatusOpen,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateItem(item); err != nil {
		t.Fatalf("failed to create epic: %v", err)
	}
	return item
}

func TestListItemsFiltered_Parent(t *testing.T) {
	db := setupTestDB(t)

	epic := createTestEpic(t, db, "Epic 1", "test")
	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)

	// Set task1's parent to epic
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	// Filter by parent
	items, err := db.ListItemsFiltered(ListFilter{Parent: epic.ID})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item under epic, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task1.ID {
		t.Errorf("expected task1 under epic, got %s", items[0].ID)
	}

	// task2 should not be included
	for _, item := range items {
		if item.ID == task2.ID {
			t.Error("task2 should not be under epic")
		}
	}
}

func TestListItemsFiltered_Type(t *testing.T) {
	db := setupTestDB(t)

	createTestEpic(t, db, "Epic 1", "test")
	createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)

	// Filter by type=epic
	items, err := db.ListItemsFiltered(ListFilter{Type: "epic"})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 epic, got %d", len(items))
	}

	// Filter by type=task
	items, err = db.ListItemsFiltered(ListFilter{Type: "task"})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(items))
	}
}

func TestListItemsFiltered_InvalidType(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.ListItemsFiltered(ListFilter{Type: "invalid"})
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestListItemsFiltered_Blocking(t *testing.T) {
	db := setupTestDB(t)

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	task3 := createTestItemWithProject(t, db, "Task 3", "test", model.StatusOpen, 2)

	// task2 depends on task1 (task1 blocks task2)
	if err := db.AddDep(task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}
	// task2 also depends on task3 (task3 blocks task2)
	if err := db.AddDep(task2.ID, task3.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Find items that block task2
	items, err := db.ListItemsFiltered(ListFilter{Blocking: task2.ID})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items blocking task2, got %d", len(items))
	}

	// Verify task1 and task3 are in the list
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[task1.ID] || !ids[task3.ID] {
		t.Errorf("expected task1 and task3, got %v", ids)
	}
}

func TestListItemsFiltered_BlockedBy(t *testing.T) {
	db := setupTestDB(t)

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	task3 := createTestItemWithProject(t, db, "Task 3", "test", model.StatusOpen, 2)

	// task2 depends on task1 (task2 is blocked by task1)
	if err := db.AddDep(task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}
	// task3 depends on task1 (task3 is blocked by task1)
	if err := db.AddDep(task3.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Find items blocked by task1
	items, err := db.ListItemsFiltered(ListFilter{BlockedBy: task1.ID})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items blocked by task1, got %d", len(items))
	}

	// Verify task2 and task3 are in the list
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[task2.ID] || !ids[task3.ID] {
		t.Errorf("expected task2 and task3, got %v", ids)
	}
}

func TestListItemsFiltered_HasBlockers(t *testing.T) {
	db := setupTestDB(t)

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	task3 := createTestItemWithProject(t, db, "Task 3", "test", model.StatusOpen, 2)

	// task2 depends on task1
	if err := db.AddDep(task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Find items with unresolved blockers
	items, err := db.ListItemsFiltered(ListFilter{HasBlockers: true})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item with blockers, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task2.ID {
		t.Errorf("expected task2 to have blockers, got %s", items[0].ID)
	}

	// Complete task1, now task2 should have no unresolved blockers
	if err := db.UpdateStatus(task1.ID, model.StatusDone); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	items, err = db.ListItemsFiltered(ListFilter{HasBlockers: true})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items with blockers after completing dep, got %d", len(items))
	}

	// task3 was never returned because it has no deps
	_ = task3
}

func TestListItemsFiltered_NoBlockers(t *testing.T) {
	db := setupTestDB(t)

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	task3 := createTestItemWithProject(t, db, "Task 3", "test", model.StatusOpen, 2)

	// task2 depends on task1
	if err := db.AddDep(task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Find items with no blockers (task1 and task3)
	items, err := db.ListItemsFiltered(ListFilter{NoBlockers: true})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items with no blockers, got %d", len(items))
	}

	// Verify task1 and task3 are in the list (not task2)
	ids := map[string]bool{}
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids[task1.ID] || !ids[task3.ID] {
		t.Errorf("expected task1 and task3, got %v", ids)
	}
	if ids[task2.ID] {
		t.Error("task2 should not be in the no-blockers list")
	}

	// Complete task1, now task2 should also have no unresolved blockers
	if err := db.UpdateStatus(task1.ID, model.StatusDone); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	items, err = db.ListItemsFiltered(ListFilter{NoBlockers: true})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items with no blockers after completing dep, got %d", len(items))
	}
}

func TestListItemsFiltered_CombinedFilters(t *testing.T) {
	db := setupTestDB(t)

	epic := createTestEpic(t, db, "Epic", "test")
	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusOpen, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusDone, 2)
	task3 := createTestItemWithProject(t, db, "Task 3", "other", model.StatusOpen, 2)

	// Set parents
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	// Filter by parent and status
	status := model.StatusOpen
	items, err := db.ListItemsFiltered(ListFilter{
		Parent: epic.ID,
		Status: &status,
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 open item under epic, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task1.ID {
		t.Errorf("expected task1, got %s", items[0].ID)
	}

	// Filter by project and type
	items, err = db.ListItemsFiltered(ListFilter{
		Project: "test",
		Type:    "task",
	})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 tasks in test project, got %d", len(items))
	}

	_ = task3
}

func TestReadyItems_ReviewingExcluded(t *testing.T) {
	db := setupTestDB(t)

	// Create a task and move it to reviewing
	task := createTestItemWithProject(t, db, "Reviewing Task", "test", model.StatusOpen, 2)
	if err := db.UpdateStatus(task.ID, model.StatusInProgress); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if err := db.UpdateStatus(task.ID, model.StatusReviewing); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Create an open task that should be ready
	openTask := createTestItemWithProject(t, db, "Open Task", "test", model.StatusOpen, 2)

	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready: %v", err)
	}

	// Only the open task should be ready; reviewing task should not appear
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != openTask.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, openTask.ID)
	}
}

func TestReadyItems_ReviewingDoesNotResolveDeps(t *testing.T) {
	db := setupTestDB(t)

	// Create blocker task and move it to reviewing
	blocker := createTestItemWithProject(t, db, "Blocker", "test", model.StatusOpen, 2)
	if err := db.UpdateStatus(blocker.ID, model.StatusInProgress); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if err := db.UpdateStatus(blocker.ID, model.StatusReviewing); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	// Create task that depends on the reviewing blocker
	task := createTestItemWithProject(t, db, "Blocked Task", "test", model.StatusOpen, 1)
	if err := db.AddDep(task.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// Task should NOT be ready because reviewing is not a terminal status
	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready: %v", err)
	}
	for _, r := range ready {
		if r.ID == task.ID {
			t.Error("task blocked by reviewing item should NOT be ready")
		}
	}

	// HasUnmetDeps should also return true
	unmet, err := db.HasUnmetDeps(task.ID)
	if err != nil {
		t.Fatalf("failed to check deps: %v", err)
	}
	if !unmet {
		t.Error("expected unmet deps when blocker is reviewing")
	}
}

func TestProjectStatus_WithReviewing(t *testing.T) {
	db := setupTestDB(t)

	createTestItemWithProject(t, db, "Open", "test", model.StatusOpen, 2)
	createTestItemWithProject(t, db, "In Progress", "test", model.StatusInProgress, 2)
	createTestItemWithProject(t, db, "Reviewing", "test", model.StatusReviewing, 2)
	createTestItemWithProject(t, db, "Done", "test", model.StatusDone, 2)

	report, err := db.ProjectStatus("test")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	if report.Reviewing != 1 {
		t.Errorf("reviewing = %d, want 1", report.Reviewing)
	}
	if len(report.ReviewingItems) != 1 {
		t.Errorf("reviewing items = %d, want 1", len(report.ReviewingItems))
	}
}

func TestReadyItems_WithDefinitionOfDone(t *testing.T) {
	db := setupTestDB(t)

	// Create a task without DoD
	createTestItemWithProject(t, db, "Task without DoD", "test", model.StatusOpen, 2)

	// Create a task with DoD
	dod := "Tests pass; Docs updated"
	itemWithDoD := &model.Item{
		ID:               model.GenerateID(model.ItemTypeTask),
		Project:          "test",
		Type:             model.ItemTypeTask,
		Title:            "Task with DoD",
		DefinitionOfDone: &dod,
		Status:           model.StatusOpen,
		Priority:         1,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := db.CreateItem(itemWithDoD); err != nil {
		t.Fatalf("failed to create item with DoD: %v", err)
	}

	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("failed to get ready: %v", err)
	}

	if len(ready) != 2 {
		t.Errorf("expected 2 ready items, got %d", len(ready))
	}

	// Verify the item with DoD has DefinitionOfDone populated
	var foundDoD bool
	for _, item := range ready {
		if item.ID == itemWithDoD.ID {
			if item.DefinitionOfDone == nil {
				t.Error("expected DefinitionOfDone to be populated")
			} else if *item.DefinitionOfDone != dod {
				t.Errorf("DefinitionOfDone = %q, want %q", *item.DefinitionOfDone, dod)
			}
			foundDoD = true
		}
	}
	if !foundDoD {
		t.Error("item with DoD not found in ready list")
	}
}

// --- Derived Epic Status Tests ---

func TestDeriveEpicStatus_NoChildren(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Empty Epic", "test")

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusOpen {
		t.Errorf("empty epic status = %q, want open", status)
	}
}

func TestDeriveEpicStatus_AllChildrenDone(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusDone, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusDone, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusDone {
		t.Errorf("all-done epic status = %q, want done", status)
	}
}

func TestDeriveEpicStatus_AllChildrenCanceled(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusCanceled, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusDone {
		t.Errorf("all-canceled epic status = %q, want done", status)
	}
}

func TestDeriveEpicStatus_MixDoneAndCanceled(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusDone, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusCanceled, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusDone {
		t.Errorf("done+canceled epic status = %q, want done", status)
	}
}

func TestDeriveEpicStatus_InProgressChild(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusInProgress, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusInProgress {
		t.Errorf("in_progress child epic status = %q, want in_progress", status)
	}
}

func TestDeriveEpicStatus_DoneChildWithOpenRemaining(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusDone, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusOpen, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	// Has a done child but not all resolved => in_progress
	if status != model.StatusInProgress {
		t.Errorf("partial-done epic status = %q, want in_progress", status)
	}
}

func TestDeriveEpicStatus_AllChildrenBlocked(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusBlocked, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusBlocked, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusBlocked {
		t.Errorf("all-blocked epic status = %q, want blocked", status)
	}
}

func TestDeriveEpicStatus_BlockedWithDoneChildren(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusDone, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusBlocked, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	// Done children exist but remaining are blocked => blocked
	if status != model.StatusBlocked {
		t.Errorf("done+blocked epic status = %q, want blocked", status)
	}
}

func TestDeriveEpicStatus_AllOpen(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusOpen, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusOpen {
		t.Errorf("all-open epic status = %q, want open", status)
	}
}

func TestDeriveEpicStatus_ReviewingChild(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusReviewing, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	// Reviewing counts as active progress
	if status != model.StatusInProgress {
		t.Errorf("reviewing-child epic status = %q, want in_progress", status)
	}
}

func TestDeriveEpicStatus_ManualOverrideDone(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	// Add an open child
	task := createTestItemWithProject(t, db, "Task", "test", model.StatusOpen, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Manually mark epic done (force-close)
	if err := db.UpdateStatus(epic.ID, model.StatusDone); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	// Manual done overrides derived status
	if status != model.StatusDone {
		t.Errorf("manually-done epic status = %q, want done", status)
	}
}

func TestDeriveEpicStatus_ManualOverrideCanceled(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusOpen, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	if err := db.UpdateStatus(epic.ID, model.StatusCanceled); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	if status != model.StatusCanceled {
		t.Errorf("manually-canceled epic status = %q, want canceled", status)
	}
}

func TestDeriveEpicStatus_ReopenedChildRevertsEpic(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task1 := createTestItemWithProject(t, db, "Task 1", "test", model.StatusDone, 2)
	task2 := createTestItemWithProject(t, db, "Task 2", "test", model.StatusDone, 2)
	if err := db.SetParent(task1.ID, epic.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetParent(task2.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Verify epic is done
	status, _ := db.DeriveEpicStatus(epic.ID)
	if status != model.StatusDone {
		t.Fatalf("precondition: epic should be done, got %q", status)
	}

	// Reopen task1
	if err := db.UpdateStatus(task1.ID, model.StatusOpen); err != nil {
		t.Fatal(err)
	}

	status, err := db.DeriveEpicStatus(epic.ID)
	if err != nil {
		t.Fatalf("DeriveEpicStatus: %v", err)
	}
	// One done child + one open child => in_progress
	if status != model.StatusInProgress {
		t.Errorf("reopened-child epic status = %q, want in_progress", status)
	}
}

func TestGetItem_EpicDerivedStatus(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusDone, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	item, err := db.GetItem(epic.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	// GetItem should return derived status
	if item.Status != model.StatusDone {
		t.Errorf("GetItem epic status = %q, want done", item.Status)
	}
}

func TestListItemsFiltered_EpicDerivedStatus(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")

	task := createTestItemWithProject(t, db, "Task", "test", model.StatusInProgress, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Filter by in_progress should include the epic (derived in_progress, stored open)
	inProg := model.StatusInProgress
	items, err := db.ListItemsFiltered(ListFilter{Project: "test", Status: &inProg})
	if err != nil {
		t.Fatalf("ListItemsFiltered: %v", err)
	}

	found := false
	for _, item := range items {
		if item.ID == epic.ID {
			found = true
			if item.Status != model.StatusInProgress {
				t.Errorf("epic status in list = %q, want in_progress", item.Status)
			}
		}
	}
	if !found {
		t.Error("epic with derived in_progress not found in in_progress filtered list")
	}
}

func TestReadyItems_ExcludesEpics(t *testing.T) {
	db := setupTestDB(t)
	epic := createTestEpic(t, db, "Epic", "test")
	task := createTestItemWithProject(t, db, "Open Task", "test", model.StatusOpen, 2)

	_ = epic // epic is open with no deps, but should not appear in ready

	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("ReadyItems: %v", err)
	}

	for _, item := range ready {
		if item.ID == epic.ID {
			t.Error("epic should not appear in ready items")
		}
	}

	if len(ready) != 1 || ready[0].ID != task.ID {
		t.Errorf("expected 1 ready task, got %d", len(ready))
	}
}

func TestReadyItems_EpicDepResolvesWhenChildrenDone(t *testing.T) {
	db := setupTestDB(t)

	// Create epic with one child task
	epic := createTestEpic(t, db, "Phase 1", "test")
	epicChild := createTestItemWithProject(t, db, "Phase 1 Task", "test", model.StatusOpen, 2)
	if err := db.SetParent(epicChild.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Create a task that depends on the epic
	phase2Task := createTestItemWithProject(t, db, "Phase 2 Task", "test", model.StatusOpen, 1)
	if err := db.AddDep(phase2Task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Phase 2 task should NOT be ready (epic child is open)
	ready, err := db.ReadyItems("test")
	if err != nil {
		t.Fatalf("ReadyItems: %v", err)
	}
	for _, r := range ready {
		if r.ID == phase2Task.ID {
			t.Error("phase 2 task should not be ready while epic child is open")
		}
	}

	// Complete the epic's child
	if err := db.UpdateStatus(epicChild.ID, model.StatusDone); err != nil {
		t.Fatal(err)
	}

	// Now phase 2 task should be ready (epic's derived status is done)
	ready, err = db.ReadyItems("test")
	if err != nil {
		t.Fatalf("ReadyItems: %v", err)
	}
	found := false
	for _, r := range ready {
		if r.ID == phase2Task.ID {
			found = true
		}
	}
	if !found {
		t.Error("phase 2 task should be ready after epic's children are all done")
	}
}

func TestProjectStatus_EpicDerivedCounts(t *testing.T) {
	db := setupTestDB(t)

	// Create epic with in_progress child (stored: open, derived: in_progress)
	epic := createTestEpic(t, db, "Epic", "test")
	task := createTestItemWithProject(t, db, "Task", "test", model.StatusInProgress, 2)
	if err := db.SetParent(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	report, err := db.ProjectStatus("test")
	if err != nil {
		t.Fatalf("ProjectStatus: %v", err)
	}

	// Epic should count as in_progress (derived), not open (stored)
	// Items: epic (derived in_progress) + task (in_progress) = 2 in_progress
	if report.InProgress != 2 {
		t.Errorf("in_progress count = %d, want 2 (task + epic)", report.InProgress)
	}
	if report.Open != 0 {
		t.Errorf("open count = %d, want 0 (epic should be derived in_progress)", report.Open)
	}
}

// TestDeriveAndDeps_Consistency verifies the two derivation paths (Go-side
// DeriveEpicStatus and SQL-side depUnresolvedExpr) agree on whether an epic
// is "resolved". For every child-status combination where DeriveEpicStatus
// returns done or canceled, HasUnmetDeps must return false, and vice versa.
func TestDeriveAndDeps_Consistency(t *testing.T) {
	cases := []struct {
		name          string
		childStatuses []model.Status
		wantResolved  bool // true = epic derived done/canceled
	}{
		{"all done", []model.Status{model.StatusDone, model.StatusDone}, true},
		{"all canceled", []model.Status{model.StatusCanceled}, true},
		{"done+canceled", []model.Status{model.StatusDone, model.StatusCanceled}, true},
		{"all open", []model.Status{model.StatusOpen}, false},
		{"one open one done", []model.Status{model.StatusOpen, model.StatusDone}, false},
		{"in_progress", []model.Status{model.StatusInProgress}, false},
		{"blocked", []model.Status{model.StatusBlocked}, false},
		{"reviewing", []model.Status{model.StatusReviewing}, false},
		{"open+in_progress+done", []model.Status{model.StatusOpen, model.StatusInProgress, model.StatusDone}, false},
		{"blocked+done", []model.Status{model.StatusBlocked, model.StatusDone}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)

			epic := createTestEpic(t, db, "Epic", "test")
			for i, s := range tc.childStatuses {
				child := createTestItemWithProject(t, db, fmt.Sprintf("Child %d", i), "test", s, 2)
				if err := db.SetParent(child.ID, epic.ID); err != nil {
					t.Fatal(err)
				}
			}

			// Create a downstream task that depends on the epic
			downstream := createTestItemWithProject(t, db, "Downstream", "test", model.StatusOpen, 1)
			if err := db.AddDep(downstream.ID, epic.ID); err != nil {
				t.Fatal(err)
			}

			derived, err := db.DeriveEpicStatus(epic.ID)
			if err != nil {
				t.Fatalf("DeriveEpicStatus: %v", err)
			}
			epicResolved := derived == model.StatusDone || derived == model.StatusCanceled

			unmet, err := db.HasUnmetDeps(downstream.ID)
			if err != nil {
				t.Fatalf("HasUnmetDeps: %v", err)
			}
			depsResolved := !unmet

			if epicResolved != depsResolved {
				t.Errorf("derivation paths diverge: DeriveEpicStatus=%q (resolved=%v) but HasUnmetDeps=%v (resolved=%v)",
					derived, epicResolved, unmet, depsResolved)
			}
			if epicResolved != tc.wantResolved {
				t.Errorf("resolved=%v, want %v (derived=%q)", epicResolved, tc.wantResolved, derived)
			}
		})
	}
}

func TestHasUnmetDeps_EpicDepResolvedByChildren(t *testing.T) {
	db := setupTestDB(t)

	epic := createTestEpic(t, db, "Epic", "test")
	epicChild := createTestItemWithProject(t, db, "Child", "test", model.StatusOpen, 2)
	if err := db.SetParent(epicChild.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	task := createTestItemWithProject(t, db, "Downstream", "test", model.StatusOpen, 1)
	if err := db.AddDep(task.ID, epic.ID); err != nil {
		t.Fatal(err)
	}

	// Epic child is open => epic is unresolved
	unmet, err := db.HasUnmetDeps(task.ID)
	if err != nil {
		t.Fatalf("HasUnmetDeps: %v", err)
	}
	if !unmet {
		t.Error("expected unmet deps when epic child is open")
	}

	// Complete the child => epic derived done => dep resolved
	if err := db.UpdateStatus(epicChild.ID, model.StatusDone); err != nil {
		t.Fatal(err)
	}

	unmet, err = db.HasUnmetDeps(task.ID)
	if err != nil {
		t.Fatalf("HasUnmetDeps: %v", err)
	}
	if unmet {
		t.Error("expected no unmet deps after epic child completed")
	}
}
