package tasks_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/db"
	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

// Service tests run against the real SQLite store (no mocks).

// newTestService returns a Service over a fresh SQLite database. The *db.DB
// is returned for fixtures that need raw SQL (for example fixed timestamps).
func newTestService(t *testing.T) (*tasks.Service, *db.DB) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Init(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return tasks.NewService(database), database
}

// create adds an item through the Service and fails the test on error.
func create(t *testing.T, svc *tasks.Service, project string, typ model.ItemType, status model.Status, title string) model.Item {
	t.Helper()
	item, err := svc.CreateItem(context.Background(), model.NewItem{
		Project: project, Type: typ, Title: title, Status: status, Priority: 2,
	})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return item
}

// graph loads a fresh snapshot and fails the test on error.
func graph(t *testing.T, svc *tasks.Service) *tasks.Graph {
	t.Helper()
	g, err := svc.Graph(context.Background())
	if err != nil {
		t.Fatalf("load graph: %v", err)
	}
	return g
}

// Protects the epic write rule now that it lives in Service, not SQL.
func TestSetStatus_EpicOnlyAcceptsTerminal(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	epic := create(t, svc, "p", model.ItemTypeEpic, model.StatusOpen, "epic")

	for _, s := range []model.Status{model.StatusOpen, model.StatusInProgress, model.StatusDraft, model.StatusReviewing} {
		if err := svc.SetStatus(ctx, epic.ID, s); err == nil {
			t.Errorf("SetStatus(epic, %s) succeeded; derived epic status must not be overwritten", s)
		}
	}
	if err := svc.SetStatus(ctx, epic.ID, model.StatusDone); err != nil {
		t.Fatalf("force-close epic: %v", err)
	}
	got, _ := graph(t, svc).Item(epic.ID)
	if got.Status != model.StatusDone {
		t.Errorf("force-closed epic status = %s, want done", got.Status)
	}
	if err := svc.SetStatus(ctx, "ts-missing", model.StatusDone); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("SetStatus(missing) err = %v, want not found", err)
	}
}

func TestSetParent_RejectsNonEpicParent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	task := create(t, svc, "p", model.ItemTypeTask, model.StatusOpen, "task")
	notEpic := create(t, svc, "p", model.ItemTypeTask, model.StatusOpen, "not an epic")
	epic := create(t, svc, "p", model.ItemTypeEpic, model.StatusOpen, "epic")

	if err := svc.SetParent(ctx, task.ID, notEpic.ID); err == nil {
		t.Error("SetParent to a task succeeded, want error")
	}
	if err := svc.SetParent(ctx, task.ID, "ep-missing"); err == nil {
		t.Error("SetParent to a missing parent succeeded, want error")
	}
	if err := svc.SetParent(ctx, "ts-missing", epic.ID); err == nil {
		t.Error("SetParent of a missing item succeeded, want error")
	}
	if err := svc.SetParent(ctx, task.ID, epic.ID); err != nil {
		t.Fatalf("SetParent to epic: %v", err)
	}
	got, _ := graph(t, svc).Item(task.ID)
	if got.ParentID == nil || *got.ParentID != epic.ID {
		t.Errorf("ParentID = %v, want %s", got.ParentID, epic.ID)
	}
}

// Protects backend-assigned IDs and project normalization at the service boundary.
func TestCreateItem_AssignsPrefixedIDAndNormalizesProject(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)

	task := create(t, svc, "  alpha\t", model.ItemTypeTask, model.StatusOpen, "task")
	epic := create(t, svc, "alpha", model.ItemTypeEpic, model.StatusOpen, "epic")
	if !strings.HasPrefix(task.ID, "ts-") || !strings.HasPrefix(epic.ID, "ep-") {
		t.Errorf("IDs = %s, %s; want ts- and ep- prefixes", task.ID, epic.ID)
	}
	if task.Project != "alpha" {
		t.Errorf("Project = %q, want normalized %q", task.Project, "alpha")
	}
	items, err := graph(t, svc).List(tasks.ListFilter{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Errorf("List(alpha) returned %d items, want 2", len(items))
	}
	projects, _ := svc.ListProjects(ctx)
	if len(projects) != 1 || projects[0] != "alpha" {
		t.Errorf("ListProjects = %q, want [alpha]", projects)
	}

	if _, err := svc.CreateItem(ctx, model.NewItem{Project: "p", Type: "story", Title: "x", Status: model.StatusOpen}); err == nil {
		t.Error("CreateItem accepted an invalid type")
	}
	if _, err := svc.CreateItem(ctx, model.NewItem{Project: "p", Type: model.ItemTypeTask, Title: "x", Status: "bogus"}); err == nil {
		t.Error("CreateItem accepted an invalid status")
	}
}
