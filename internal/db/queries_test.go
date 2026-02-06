package db

import (
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
		ID:       model.GenerateID(model.ItemTypeEpic),
		Project:  project,
		Type:     model.ItemTypeEpic,
		Title:    title,
		Status:   model.StatusOpen,
		Priority: 2,
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
