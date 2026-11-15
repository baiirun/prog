package tasks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

// Flow tests: writes through Service over the real SQLite store, then reads
// through a fresh Graph. Ported from the SQL-era internal/db query and dep
// tests; each test name keeps the old intent.

// createPri adds a task with an explicit priority.
func createPri(t *testing.T, svc *tasks.Service, project, title string, status model.Status, priority int) model.Item {
	t.Helper()
	item, err := svc.CreateItem(context.Background(), model.NewItem{
		Project: project, Type: model.ItemTypeTask, Title: title, Status: status, Priority: priority,
	})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return item
}

func createEpic(t *testing.T, svc *tasks.Service, project, title string) model.Item {
	t.Helper()
	return create(t, svc, project, model.ItemTypeEpic, model.StatusOpen, title)
}

func mustAddDep(t *testing.T, svc *tasks.Service, itemID, dependsOnID string) {
	t.Helper()
	if err := svc.AddDep(context.Background(), itemID, dependsOnID); err != nil {
		t.Fatalf("AddDep(%s, %s): %v", itemID, dependsOnID, err)
	}
}

func mustSetParent(t *testing.T, svc *tasks.Service, id, parentID string) {
	t.Helper()
	if err := svc.SetParent(context.Background(), id, parentID); err != nil {
		t.Fatalf("SetParent(%s, %s): %v", id, parentID, err)
	}
}

func mustSetStatus(t *testing.T, svc *tasks.Service, id string, status model.Status) {
	t.Helper()
	if err := svc.SetStatus(context.Background(), id, status); err != nil {
		t.Fatalf("SetStatus(%s, %s): %v", id, status, err)
	}
}

func mustList(t *testing.T, svc *tasks.Service, f tasks.ListFilter) []model.Item {
	t.Helper()
	items, err := graph(t, svc).List(f)
	if err != nil {
		t.Fatalf("List(%+v): %v", f, err)
	}
	return items
}

func idSet(items []model.Item) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, it := range items {
		s[it.ID] = true
	}
	return s
}

// hasUnmetDeps replaces db.HasUnmetDeps: an item has unmet deps exactly when
// it appears in List(HasBlockers) and not in List(NoBlockers). Both are
// checked so the two filters cannot disagree.
func hasUnmetDeps(t *testing.T, svc *tasks.Service, id string) bool {
	t.Helper()
	g := graph(t, svc)
	has, err := g.List(tasks.ListFilter{HasBlockers: true})
	if err != nil {
		t.Fatal(err)
	}
	no, err := g.List(tasks.ListFilter{NoBlockers: true})
	if err != nil {
		t.Fatal(err)
	}
	inHas, inNo := idSet(has)[id], idSet(no)[id]
	if inHas == inNo {
		t.Fatalf("item %s: HasBlockers=%v and NoBlockers=%v must be exclusive", id, inHas, inNo)
	}
	return inHas
}

func effectiveStatus(t *testing.T, svc *tasks.Service, id string) model.Status {
	t.Helper()
	item, err := graph(t, svc).Item(id)
	if err != nil {
		t.Fatalf("Item(%s): %v", id, err)
	}
	return item.Status
}

// --- List ---

func TestList_ProjectAndStatusFilters(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "proj1", "Task 1", model.StatusOpen, 2)
	createPri(t, svc, "proj1", "Task 2", model.StatusDone, 2)
	createPri(t, svc, "proj2", "Task 3", model.StatusOpen, 2)

	if items := mustList(t, svc, tasks.ListFilter{}); len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items := mustList(t, svc, tasks.ListFilter{Project: "proj1"}); len(items) != 2 {
		t.Errorf("expected 2 items for proj1, got %d", len(items))
	}
	status := model.StatusOpen
	if items := mustList(t, svc, tasks.ListFilter{Status: &status}); len(items) != 2 {
		t.Errorf("expected 2 open items, got %d", len(items))
	}
	if items := mustList(t, svc, tasks.ListFilter{Project: "proj1", Status: &status}); len(items) != 1 {
		t.Errorf("expected 1 open item in proj1, got %d", len(items))
	}
}

func TestList_InvalidStatus(t *testing.T) {
	svc, _ := newTestService(t)
	status := model.Status("invalid")
	if _, err := graph(t, svc).List(tasks.ListFilter{Status: &status}); err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestList_OrderByPriority(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "test", "Low", model.StatusOpen, 3)
	createPri(t, svc, "test", "High", model.StatusOpen, 1)
	createPri(t, svc, "test", "Medium", model.StatusOpen, 2)

	items := mustList(t, svc, tasks.ListFilter{Project: "test"})
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Title != "High" {
		t.Errorf("first item should be High priority, got %q", items[0].Title)
	}
	if items[2].Title != "Low" {
		t.Errorf("last item should be Low priority, got %q", items[2].Title)
	}
}

func TestList_Parent(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic 1")
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	mustSetParent(t, svc, task1.ID, epic.ID)

	items := mustList(t, svc, tasks.ListFilter{Parent: epic.ID})
	if len(items) != 1 {
		t.Errorf("expected 1 item under epic, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task1.ID {
		t.Errorf("expected task1 under epic, got %s", items[0].ID)
	}
	if idSet(items)[task2.ID] {
		t.Error("task2 should not be under epic")
	}
}

func TestList_Type(t *testing.T) {
	svc, _ := newTestService(t)
	createEpic(t, svc, "test", "Epic 1")
	createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)

	if items := mustList(t, svc, tasks.ListFilter{Type: "epic"}); len(items) != 1 {
		t.Errorf("expected 1 epic, got %d", len(items))
	}
	if items := mustList(t, svc, tasks.ListFilter{Type: "task"}); len(items) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(items))
	}
}

func TestList_InvalidType(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := graph(t, svc).List(tasks.ListFilter{Type: "invalid"}); err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestList_Blocking(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	task3 := createPri(t, svc, "test", "Task 3", model.StatusOpen, 2)
	mustAddDep(t, svc, task2.ID, task1.ID)
	mustAddDep(t, svc, task2.ID, task3.ID)

	items := mustList(t, svc, tasks.ListFilter{Blocking: task2.ID})
	if len(items) != 2 {
		t.Errorf("expected 2 items blocking task2, got %d", len(items))
	}
	ids := idSet(items)
	if !ids[task1.ID] || !ids[task3.ID] {
		t.Errorf("expected task1 and task3, got %v", ids)
	}
}

func TestList_BlockedBy(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	task3 := createPri(t, svc, "test", "Task 3", model.StatusOpen, 2)
	mustAddDep(t, svc, task2.ID, task1.ID)
	mustAddDep(t, svc, task3.ID, task1.ID)

	items := mustList(t, svc, tasks.ListFilter{BlockedBy: task1.ID})
	if len(items) != 2 {
		t.Errorf("expected 2 items blocked by task1, got %d", len(items))
	}
	ids := idSet(items)
	if !ids[task2.ID] || !ids[task3.ID] {
		t.Errorf("expected task2 and task3, got %v", ids)
	}
}

func TestList_HasBlockers(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	createPri(t, svc, "test", "Task 3", model.StatusOpen, 2) // no deps, never listed
	mustAddDep(t, svc, task2.ID, task1.ID)

	items := mustList(t, svc, tasks.ListFilter{HasBlockers: true})
	if len(items) != 1 {
		t.Errorf("expected 1 item with blockers, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task2.ID {
		t.Errorf("expected task2 to have blockers, got %s", items[0].ID)
	}

	mustSetStatus(t, svc, task1.ID, model.StatusDone)
	if items := mustList(t, svc, tasks.ListFilter{HasBlockers: true}); len(items) != 0 {
		t.Errorf("expected 0 items with blockers after completing dep, got %d", len(items))
	}
}

func TestList_NoBlockers(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	task3 := createPri(t, svc, "test", "Task 3", model.StatusOpen, 2)
	mustAddDep(t, svc, task2.ID, task1.ID)

	items := mustList(t, svc, tasks.ListFilter{NoBlockers: true})
	if len(items) != 2 {
		t.Errorf("expected 2 items with no blockers, got %d", len(items))
	}
	ids := idSet(items)
	if !ids[task1.ID] || !ids[task3.ID] {
		t.Errorf("expected task1 and task3, got %v", ids)
	}
	if ids[task2.ID] {
		t.Error("task2 should not be in the no-blockers list")
	}

	mustSetStatus(t, svc, task1.ID, model.StatusDone)
	if items := mustList(t, svc, tasks.ListFilter{NoBlockers: true}); len(items) != 3 {
		t.Errorf("expected 3 items with no blockers after completing dep, got %d", len(items))
	}
}

func TestList_CanceledBlockerResolvesHasBlockers(t *testing.T) {
	svc, _ := newTestService(t)
	blocker := createPri(t, svc, "test", "Blocker", model.StatusOpen, 2)
	task := createPri(t, svc, "test", "Blocked Task", model.StatusOpen, 1)
	mustAddDep(t, svc, task.ID, blocker.ID)

	items := mustList(t, svc, tasks.ListFilter{HasBlockers: true})
	if len(items) != 1 || items[0].ID != task.ID {
		t.Errorf("expected task in has-blockers list, got %v", items)
	}

	mustSetStatus(t, svc, blocker.ID, model.StatusCanceled)
	if items := mustList(t, svc, tasks.ListFilter{HasBlockers: true}); len(items) != 0 {
		t.Errorf("expected 0 items with blockers after cancel, got %d", len(items))
	}
	if !idSet(mustList(t, svc, tasks.ListFilter{NoBlockers: true}))[task.ID] {
		t.Error("task should appear in no-blockers list after blocker is canceled")
	}
}

func TestList_CombinedFilters(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusDone, 2)
	createPri(t, svc, "other", "Task 3", model.StatusOpen, 2)
	mustSetParent(t, svc, task1.ID, epic.ID)
	mustSetParent(t, svc, task2.ID, epic.ID)

	status := model.StatusOpen
	items := mustList(t, svc, tasks.ListFilter{Parent: epic.ID, Status: &status})
	if len(items) != 1 {
		t.Errorf("expected 1 open item under epic, got %d", len(items))
	}
	if len(items) > 0 && items[0].ID != task1.ID {
		t.Errorf("expected task1, got %s", items[0].ID)
	}

	if items := mustList(t, svc, tasks.ListFilter{Project: "test", Type: "task"}); len(items) != 2 {
		t.Errorf("expected 2 tasks in test project, got %d", len(items))
	}
}

func TestList_EpicDerivedStatus(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task := createPri(t, svc, "test", "Task", model.StatusInProgress, 2)
	mustSetParent(t, svc, task.ID, epic.ID)

	inProg := model.StatusInProgress
	items := mustList(t, svc, tasks.ListFilter{Project: "test", Status: &inProg})
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

// --- Ready ---

func TestReady_DependencyCompletion(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 2)
	createPri(t, svc, "test", "Task 3", model.StatusInProgress, 2) // not ready
	mustAddDep(t, svc, task2.ID, task1.ID)

	ready := graph(t, svc).Ready("test", nil)
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != task1.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, task1.ID)
	}

	mustSetStatus(t, svc, task1.ID, model.StatusDone)
	ready = graph(t, svc).Ready("test", nil)
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item after completing dep, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != task2.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, task2.ID)
	}
}

func TestReady_CanceledBlockerUnblocks(t *testing.T) {
	svc, _ := newTestService(t)
	blocker := createPri(t, svc, "test", "Blocker", model.StatusOpen, 2)
	task := createPri(t, svc, "test", "Blocked Task", model.StatusOpen, 1)
	mustAddDep(t, svc, task.ID, blocker.ID)

	if idSet(graph(t, svc).Ready("test", nil))[task.ID] {
		t.Error("task should not be ready while blocker is open")
	}
	mustSetStatus(t, svc, blocker.ID, model.StatusCanceled)
	if !idSet(graph(t, svc).Ready("test", nil))[task.ID] {
		t.Error("task should be ready after blocker is canceled")
	}
}

func TestReady_ProjectFilter(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "proj1", "Task 1", model.StatusOpen, 2)
	createPri(t, svc, "proj2", "Task 2", model.StatusOpen, 2)

	g := graph(t, svc)
	if ready := g.Ready("proj1", nil); len(ready) != 1 {
		t.Errorf("expected 1 ready item in proj1, got %d", len(ready))
	}
	if ready := g.Ready("", nil); len(ready) != 2 {
		t.Errorf("expected 2 ready items total, got %d", len(ready))
	}
}

func TestReady_ReviewingExcluded(t *testing.T) {
	svc, _ := newTestService(t)
	task := createPri(t, svc, "test", "Reviewing Task", model.StatusOpen, 2)
	mustSetStatus(t, svc, task.ID, model.StatusInProgress)
	mustSetStatus(t, svc, task.ID, model.StatusReviewing)
	openTask := createPri(t, svc, "test", "Open Task", model.StatusOpen, 2)

	ready := graph(t, svc).Ready("test", nil)
	if len(ready) != 1 {
		t.Errorf("expected 1 ready item, got %d", len(ready))
	}
	if len(ready) > 0 && ready[0].ID != openTask.ID {
		t.Errorf("ready item = %q, want %q", ready[0].ID, openTask.ID)
	}
}

func TestReady_ReviewingDoesNotResolveDeps(t *testing.T) {
	svc, _ := newTestService(t)
	blocker := createPri(t, svc, "test", "Blocker", model.StatusOpen, 2)
	mustSetStatus(t, svc, blocker.ID, model.StatusInProgress)
	mustSetStatus(t, svc, blocker.ID, model.StatusReviewing)
	task := createPri(t, svc, "test", "Blocked Task", model.StatusOpen, 1)
	mustAddDep(t, svc, task.ID, blocker.ID)

	if idSet(graph(t, svc).Ready("test", nil))[task.ID] {
		t.Error("task blocked by reviewing item should NOT be ready")
	}
	if !hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected unmet deps when blocker is reviewing")
	}
}

func TestReady_WithDefinitionOfDone(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	createPri(t, svc, "test", "Task without DoD", model.StatusOpen, 2)
	dod := "Tests pass; Docs updated"
	withDoD, err := svc.CreateItem(ctx, model.NewItem{
		Project: "test", Type: model.ItemTypeTask, Title: "Task with DoD",
		DefinitionOfDone: &dod, Status: model.StatusOpen, Priority: 1,
	})
	if err != nil {
		t.Fatalf("create item with DoD: %v", err)
	}

	ready := graph(t, svc).Ready("test", nil)
	if len(ready) != 2 {
		t.Errorf("expected 2 ready items, got %d", len(ready))
	}
	found := false
	for _, item := range ready {
		if item.ID != withDoD.ID {
			continue
		}
		found = true
		if item.DefinitionOfDone == nil {
			t.Error("expected DefinitionOfDone to be populated")
		} else if *item.DefinitionOfDone != dod {
			t.Errorf("DefinitionOfDone = %q, want %q", *item.DefinitionOfDone, dod)
		}
	}
	if !found {
		t.Error("item with DoD not found in ready list")
	}
}

func TestReady_ExcludesEpics(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task := createPri(t, svc, "test", "Open Task", model.StatusOpen, 2)

	ready := graph(t, svc).Ready("test", nil)
	if idSet(ready)[epic.ID] {
		t.Error("epic should not appear in ready items")
	}
	if len(ready) != 1 || ready[0].ID != task.ID {
		t.Errorf("expected 1 ready task, got %d", len(ready))
	}
}

func TestReady_EpicDepResolvesWhenChildrenDone(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Phase 1")
	epicChild := createPri(t, svc, "test", "Phase 1 Task", model.StatusOpen, 2)
	mustSetParent(t, svc, epicChild.ID, epic.ID)
	phase2Task := createPri(t, svc, "test", "Phase 2 Task", model.StatusOpen, 1)
	mustAddDep(t, svc, phase2Task.ID, epic.ID)

	if idSet(graph(t, svc).Ready("test", nil))[phase2Task.ID] {
		t.Error("phase 2 task should not be ready while epic child is open")
	}
	mustSetStatus(t, svc, epicChild.ID, model.StatusDone)
	if !idSet(graph(t, svc).Ready("test", nil))[phase2Task.ID] {
		t.Error("phase 2 task should be ready after epic's children are all done")
	}
}

// --- Report ---

func TestReport_Counts(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "test", "Open 1", model.StatusOpen, 2)
	createPri(t, svc, "test", "Open 2", model.StatusOpen, 1)
	createPri(t, svc, "test", "In Progress", model.StatusInProgress, 2)
	createPri(t, svc, "test", "Blocked", model.StatusBlocked, 2)
	createPri(t, svc, "test", "Done", model.StatusDone, 2)

	report := graph(t, svc).Report("test", nil)
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

func TestReport_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	report := graph(t, svc).Report("empty", nil)
	if report.Open != 0 || report.Done != 0 || report.Ready != 0 {
		t.Error("expected all counts to be 0 for empty project")
	}
}

func TestReport_WithReviewing(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "test", "Open", model.StatusOpen, 2)
	createPri(t, svc, "test", "In Progress", model.StatusInProgress, 2)
	createPri(t, svc, "test", "Reviewing", model.StatusReviewing, 2)
	createPri(t, svc, "test", "Done", model.StatusDone, 2)

	report := graph(t, svc).Report("test", nil)
	if report.Reviewing != 1 {
		t.Errorf("reviewing = %d, want 1", report.Reviewing)
	}
	if len(report.ReviewingItems) != 1 {
		t.Errorf("reviewing items = %d, want 1", len(report.ReviewingItems))
	}
}

func TestReport_EpicDerivedCounts(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task := createPri(t, svc, "test", "Task", model.StatusInProgress, 2)
	mustSetParent(t, svc, task.ID, epic.ID)

	report := graph(t, svc).Report("test", nil)
	// epic (stored open, derived in_progress) + task (in_progress)
	if report.InProgress != 2 {
		t.Errorf("in_progress count = %d, want 2 (task + epic)", report.InProgress)
	}
	if report.Open != 0 {
		t.Errorf("open count = %d, want 0 (epic should be derived in_progress)", report.Open)
	}
}

// --- Epic derivation flows ---

func TestEpicStatus_ReopenedChildRevertsEpic(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task1 := createPri(t, svc, "test", "Task 1", model.StatusDone, 2)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusDone, 2)
	mustSetParent(t, svc, task1.ID, epic.ID)
	mustSetParent(t, svc, task2.ID, epic.ID)

	if got := effectiveStatus(t, svc, epic.ID); got != model.StatusDone {
		t.Fatalf("precondition: epic should be done, got %q", got)
	}
	mustSetStatus(t, svc, task1.ID, model.StatusOpen)
	// One done child + one open child => in_progress
	if got := effectiveStatus(t, svc, epic.ID); got != model.StatusInProgress {
		t.Errorf("reopened-child epic status = %q, want in_progress", got)
	}
}

// Ported from the two manual-override derivation tests: the force-close
// write goes through Service.SetStatus, so this is a write+read flow.
func TestEpicStatus_ManualOverrideWinsOverChildren(t *testing.T) {
	for _, forced := range []model.Status{model.StatusDone, model.StatusCanceled} {
		t.Run(string(forced), func(t *testing.T) {
			svc, _ := newTestService(t)
			epic := createEpic(t, svc, "test", "Epic")
			task := createPri(t, svc, "test", "Task", model.StatusOpen, 2)
			mustSetParent(t, svc, task.ID, epic.ID)
			mustSetStatus(t, svc, epic.ID, forced)

			if got := effectiveStatus(t, svc, epic.ID); got != forced {
				t.Errorf("manually-%s epic status = %q, want %s", forced, got, forced)
			}
		})
	}
}

func TestItem_EpicDerivedStatus(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	task := createPri(t, svc, "test", "Task", model.StatusDone, 2)
	mustSetParent(t, svc, task.ID, epic.ID)

	if got := effectiveStatus(t, svc, epic.ID); got != model.StatusDone {
		t.Errorf("Item epic status = %q, want done", got)
	}
}

// Verifies epic derivation and dependency resolution agree end to end: the
// epic is resolved (derived done/canceled) exactly when a task depending on
// it has no unmet deps.
func TestDeriveAndDeps_Consistency(t *testing.T) {
	cases := []struct {
		name          string
		childStatuses []model.Status
		wantResolved  bool
	}{
		{"no children", []model.Status{}, false},
		{"all done", []model.Status{model.StatusDone, model.StatusDone}, true},
		{"all canceled", []model.Status{model.StatusCanceled}, true},
		{"done+canceled", []model.Status{model.StatusDone, model.StatusCanceled}, true},
		{"all draft", []model.Status{model.StatusDraft}, false},
		{"all open", []model.Status{model.StatusOpen}, false},
		{"draft+open", []model.Status{model.StatusDraft, model.StatusOpen}, false},
		{"draft+done", []model.Status{model.StatusDraft, model.StatusDone}, false},
		{"one open one done", []model.Status{model.StatusOpen, model.StatusDone}, false},
		{"in_progress", []model.Status{model.StatusInProgress}, false},
		{"blocked", []model.Status{model.StatusBlocked}, false},
		{"reviewing", []model.Status{model.StatusReviewing}, false},
		{"open+in_progress+done", []model.Status{model.StatusOpen, model.StatusInProgress, model.StatusDone}, false},
		{"blocked+done", []model.Status{model.StatusBlocked, model.StatusDone}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTestService(t)
			epic := createEpic(t, svc, "test", "Epic")
			for i, s := range tc.childStatuses {
				child := createPri(t, svc, "test", fmt.Sprintf("Child %d", i), s, 2)
				mustSetParent(t, svc, child.ID, epic.ID)
			}
			downstream := createPri(t, svc, "test", "Downstream", model.StatusOpen, 1)
			mustAddDep(t, svc, downstream.ID, epic.ID)

			derived := effectiveStatus(t, svc, epic.ID)
			epicResolved := derived == model.StatusDone || derived == model.StatusCanceled
			unmet := hasUnmetDeps(t, svc, downstream.ID)
			readyResolved := idSet(graph(t, svc).Ready("test", nil))[downstream.ID]

			if epicResolved != !unmet || epicResolved != readyResolved {
				t.Errorf("paths diverge: derived=%q (resolved=%v), unmet=%v, ready=%v",
					derived, epicResolved, unmet, readyResolved)
			}
			if epicResolved != tc.wantResolved {
				t.Errorf("resolved=%v, want %v (derived=%q)", epicResolved, tc.wantResolved, derived)
			}
		})
	}
}

// --- Dependencies ---

func TestAddDep_NonexistentItem(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 2)

	if err := svc.AddDep(ctx, task1.ID, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent dependency")
	}
	if err := svc.AddDep(ctx, "nonexistent", task1.ID); err == nil {
		t.Error("expected error for nonexistent item")
	}
	if deps := graph(t, svc).DepsOf(task1.ID); len(deps) != 0 {
		t.Errorf("rejected AddDep must not write an edge, got %v", deps)
	}
}

func TestUnmetDeps_DoneResolves(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 0)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 0)
	mustAddDep(t, svc, task2.ID, task1.ID)

	if !hasUnmetDeps(t, svc, task2.ID) {
		t.Error("expected unmet deps when dependency is open")
	}
	mustSetStatus(t, svc, task1.ID, model.StatusDone)
	if hasUnmetDeps(t, svc, task2.ID) {
		t.Error("expected no unmet deps when dependency is done")
	}
}

func TestUnmetDeps_NoDeps(t *testing.T) {
	svc, _ := newTestService(t)
	task := createPri(t, svc, "test", "Task", model.StatusOpen, 0)
	if hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected no unmet deps for task with no dependencies")
	}
}

func TestDepsOf_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	task := createPri(t, svc, "test", "Task", model.StatusOpen, 0)
	deps := graph(t, svc).DepsOf(task.ID)
	if deps == nil {
		t.Error("DepsOf returned nil, want empty non-nil slice")
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %d", len(deps))
	}
}

func TestUnmetDeps_CanceledDepResolved(t *testing.T) {
	svc, _ := newTestService(t)
	blocker := createPri(t, svc, "test", "Blocker", model.StatusOpen, 0)
	task := createPri(t, svc, "test", "Blocked Task", model.StatusOpen, 0)
	mustAddDep(t, svc, task.ID, blocker.ID)

	if !hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected unmet deps when blocker is open")
	}
	mustSetStatus(t, svc, blocker.ID, model.StatusCanceled)
	if hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected no unmet deps when blocker is canceled")
	}
	// The edge is preserved; only its resolution changed.
	if deps := graph(t, svc).DepsOf(task.ID); len(deps) != 1 || deps[0] != blocker.ID {
		t.Errorf("dep row should be preserved after cancel, got %v", deps)
	}
}

func TestUnmetDeps_EpicDepResolvedByChildren(t *testing.T) {
	svc, _ := newTestService(t)
	epic := createEpic(t, svc, "test", "Epic")
	epicChild := createPri(t, svc, "test", "Child", model.StatusOpen, 2)
	mustSetParent(t, svc, epicChild.ID, epic.ID)
	task := createPri(t, svc, "test", "Downstream", model.StatusOpen, 1)
	mustAddDep(t, svc, task.ID, epic.ID)

	if !hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected unmet deps when epic child is open")
	}
	mustSetStatus(t, svc, epicChild.ID, model.StatusDone)
	if hasUnmetDeps(t, svc, task.ID) {
		t.Error("expected no unmet deps after epic child completed")
	}
}

func TestEdges(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 0)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 0)
	task3 := createPri(t, svc, "test", "Task 3", model.StatusOpen, 0)
	mustAddDep(t, svc, task2.ID, task1.ID)
	mustAddDep(t, svc, task3.ID, task1.ID)

	edges := graph(t, svc).Edges("")
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
	for _, e := range edges {
		if e.DependsOnID != task1.ID {
			t.Errorf("expected all deps on %s, got %s", task1.ID, e.DependsOnID)
		}
		if e.DependsOnTitle != "Task 1" {
			t.Errorf("expected dep title 'Task 1', got %q", e.DependsOnTitle)
		}
	}
}

func TestEdges_FilterByProject(t *testing.T) {
	svc, _ := newTestService(t)
	task1 := createPri(t, svc, "test", "Task 1", model.StatusOpen, 0)
	task2 := createPri(t, svc, "test", "Task 2", model.StatusOpen, 0)
	mustAddDep(t, svc, task2.ID, task1.ID)
	other1 := createPri(t, svc, "other", "Other Task", model.StatusOpen, 0)
	other2 := createPri(t, svc, "other", "Other Task 2", model.StatusOpen, 0)
	mustAddDep(t, svc, other2.ID, other1.ID)

	g := graph(t, svc)
	if edges := g.Edges("test"); len(edges) != 1 {
		t.Errorf("expected 1 edge for 'test' project, got %d", len(edges))
	}
	if edges := g.Edges("other"); len(edges) != 1 {
		t.Errorf("expected 1 edge for 'other' project, got %d", len(edges))
	}
}

func TestEdges_Empty(t *testing.T) {
	svc, _ := newTestService(t)
	createPri(t, svc, "test", "Task 1", model.StatusOpen, 0)
	if edges := graph(t, svc).Edges(""); len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

// --- Service write rules ---

func TestSetStatus_InvalidStatus(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	if err := svc.SetStatus(ctx, "ts-123456", model.Status("invalid")); err == nil {
		t.Error("expected error for invalid status")
	}
	// Invalid status must be rejected even for an existing item.
	task := createPri(t, svc, "test", "Task", model.StatusOpen, 2)
	if err := svc.SetStatus(ctx, task.ID, model.Status("invalid")); err == nil {
		t.Error("expected error for invalid status on existing item")
	}
	if got := effectiveStatus(t, svc, task.ID); got != model.StatusOpen {
		t.Errorf("status after rejected write = %q, want open", got)
	}
}
