package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/model"
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

	t.Cleanup(func() { _ = db.Close() })
	return db
}

// createItem stores in and fails the test on error.
func createItem(t *testing.T, db *DB, in model.NewItem) model.Item {
	t.Helper()
	item, err := db.CreateItem(context.Background(), in)
	if err != nil {
		t.Fatalf("failed to create item %q: %v", in.Title, err)
	}
	return item
}

// createTestItem stores an open task in project "test".
func createTestItem(t *testing.T, db *DB, title string) model.Item {
	t.Helper()
	return createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   title,
		Status:  model.StatusOpen,
	})
}

// loadItem reads one item back through LoadGraph.
func loadItem(t *testing.T, db *DB, id string) model.Item {
	t.Helper()
	items, _, err := db.LoadGraph(context.Background())
	if err != nil {
		t.Fatalf("failed to load graph: %v", err)
	}
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("item %s not in graph", id)
	return model.Item{}
}

// loadDeps returns every dependency edge.
func loadDeps(t *testing.T, db *DB) []model.Dep {
	t.Helper()
	_, deps, err := db.LoadGraph(context.Background())
	if err != nil {
		t.Fatalf("failed to load graph: %v", err)
	}
	return deps
}

// countRows counts rows of table matching where (a constant SQL fragment).
func countRows(t *testing.T, db *DB, table, where string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatalf("failed to count %s: %v", table, err)
	}
	return n
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

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

	if !strings.Contains(path, filepath.Join(".prog", "prog.db")) {
		t.Errorf("expected path to contain .prog/prog.db, got %q", path)
	}
}

func TestCreateItem(t *testing.T) {
	db := setupTestDB(t)

	created := createItem(t, db, model.NewItem{
		Project:  "test",
		Type:     model.ItemTypeTask,
		Title:    "Test task",
		Status:   model.StatusOpen,
		Priority: 2,
	})

	if !strings.HasPrefix(created.ID, "ts-") {
		t.Errorf("task ID = %q, want ts- prefix", created.ID)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("expected store to assign timestamps")
	}

	got := loadItem(t, db, created.ID)
	if got.Title != "Test task" {
		t.Errorf("title = %q, want %q", got.Title, "Test task")
	}
	if got.Project != "test" {
		t.Errorf("project = %q, want %q", got.Project, "test")
	}
	if got.Priority != 2 {
		t.Errorf("priority = %d, want 2", got.Priority)
	}
	if got.ParentID != nil {
		t.Errorf("parent = %q, want nil", *got.ParentID)
	}
}

func TestCreateItem_EpicIDPrefix(t *testing.T) {
	db := setupTestDB(t)

	epic := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Epic",
		Status:  model.StatusOpen,
	})
	if !strings.HasPrefix(epic.ID, "ep-") {
		t.Errorf("epic ID = %q, want ep- prefix", epic.ID)
	}
}

func TestSetStatus(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Test")

	if err := db.SetStatus(context.Background(), item.ID, model.StatusInProgress); err != nil {
		t.Fatalf("failed to set status: %v", err)
	}

	got := loadItem(t, db, item.ID)
	if got.Status != model.StatusInProgress {
		t.Errorf("status = %q, want %q", got.Status, model.StatusInProgress)
	}
}

func TestSetStatus_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.SetStatus(context.Background(), "nonexistent", model.StatusDone)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestSetTitle(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Old")

	if err := db.SetTitle(context.Background(), item.ID, "New"); err != nil {
		t.Fatalf("failed to set title: %v", err)
	}
	if got := loadItem(t, db, item.ID); got.Title != "New" {
		t.Errorf("title = %q, want %q", got.Title, "New")
	}
}

func TestAppendDescription(t *testing.T) {
	db := setupTestDB(t)

	item := createItem(t, db, model.NewItem{
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Test",
		Description: "Initial",
		Status:      model.StatusOpen,
	})

	if err := db.AppendDescription(context.Background(), item.ID, "Appended text"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	got := loadItem(t, db, item.ID)
	if want := "Initial\n\nAppended text"; got.Description != want {
		t.Errorf("description = %q, want %q", got.Description, want)
	}
}

func TestAppendDescription_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.AppendDescription(context.Background(), "nonexistent", "text")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestSetParent(t *testing.T) {
	db := setupTestDB(t)

	epic := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Test Epic",
		Status:  model.StatusOpen,
	})
	task := createTestItem(t, db, "Test Task")

	if err := db.SetParent(context.Background(), task.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}

	got := loadItem(t, db, task.ID)
	if got.ParentID == nil {
		t.Fatal("expected parent ID to be set")
	}
	if *got.ParentID != epic.ID {
		t.Errorf("parent = %q, want %q", *got.ParentID, epic.ID)
	}
}

func TestSetDescription(t *testing.T) {
	db := setupTestDB(t)

	item := createItem(t, db, model.NewItem{
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Test",
		Description: "Original description",
		Status:      model.StatusOpen,
	})

	if err := db.SetDescription(context.Background(), item.ID, "New description"); err != nil {
		t.Fatalf("failed to set description: %v", err)
	}

	got := loadItem(t, db, item.ID)
	if got.Description != "New description" {
		t.Errorf("description = %q, want %q", got.Description, "New description")
	}
}

func TestSetDescription_EmptyToContent(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Test")

	if err := db.SetDescription(context.Background(), item.ID, "Added description"); err != nil {
		t.Fatalf("failed to set description: %v", err)
	}

	got := loadItem(t, db, item.ID)
	if got.Description != "Added description" {
		t.Errorf("description = %q, want %q", got.Description, "Added description")
	}
}

func TestSetDescription_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.SetDescription(context.Background(), "nonexistent", "text")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestSetParent_NotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	epic := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Epic",
		Status:  model.StatusOpen,
	})

	// Nonexistent task
	err := db.SetParent(ctx, "nonexistent", epic.ID)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error for nonexistent task, got %v", err)
	}

	// Nonexistent parent: rejected by the parent_id foreign key.
	task := createTestItem(t, db, "Task")
	if err := db.SetParent(ctx, task.ID, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent parent")
	}
	if got := loadItem(t, db, task.ID); got.ParentID != nil {
		t.Errorf("parent = %q after failed SetParent, want nil", *got.ParentID)
	}
}

func TestSetProject(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Task")

	if err := db.SetProject(context.Background(), item.ID, "other"); err != nil {
		t.Fatalf("failed to set project: %v", err)
	}
	if got := loadItem(t, db, item.ID); got.Project != "other" {
		t.Errorf("project = %q, want %q", got.Project, "other")
	}
}

func TestCreateItem_WithDefinitionOfDone(t *testing.T) {
	db := setupTestDB(t)

	dod := "Tests pass; Docs updated"
	item := createItem(t, db, model.NewItem{
		Project:          "test",
		Type:             model.ItemTypeTask,
		Title:            "Task with DoD",
		DefinitionOfDone: &dod,
		Status:           model.StatusOpen,
		Priority:         2,
	})

	got := loadItem(t, db, item.ID)
	if got.DefinitionOfDone == nil {
		t.Fatal("expected DefinitionOfDone to be set")
	}
	if *got.DefinitionOfDone != dod {
		t.Errorf("DefinitionOfDone = %q, want %q", *got.DefinitionOfDone, dod)
	}
}

func TestSetDefinitionOfDone(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test")

	// Set DoD
	dod := "All tests pass"
	if err := db.SetDefinitionOfDone(ctx, item.ID, &dod); err != nil {
		t.Fatalf("failed to set DoD: %v", err)
	}

	got := loadItem(t, db, item.ID)
	if got.DefinitionOfDone == nil {
		t.Fatal("expected DefinitionOfDone to be set")
	}
	if *got.DefinitionOfDone != dod {
		t.Errorf("DefinitionOfDone = %q, want %q", *got.DefinitionOfDone, dod)
	}

	// Clear DoD
	if err := db.SetDefinitionOfDone(ctx, item.ID, nil); err != nil {
		t.Fatalf("failed to clear DoD: %v", err)
	}

	got = loadItem(t, db, item.ID)
	if got.DefinitionOfDone != nil {
		t.Errorf("expected DefinitionOfDone to be nil, got %q", *got.DefinitionOfDone)
	}
}

func TestSetDefinitionOfDone_NotFound(t *testing.T) {
	db := setupTestDB(t)

	dod := "Some criteria"
	err := db.SetDefinitionOfDone(context.Background(), "nonexistent", &dod)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

// Regression: DeleteItem used to delete logs and deps, then fail on the
// item_labels foreign key, losing the item's history while keeping the item.
func TestDeleteItem_LabeledItem(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	item := createTestItem(t, db, "Labeled")
	other := createTestItem(t, db, "Other")
	if err := db.AddLabelToItem(ctx, item.ID, "test", "bug"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}
	if err := db.AddLog(ctx, item.ID, "history"); err != nil {
		t.Fatalf("failed to add log: %v", err)
	}
	if err := db.AddDep(ctx, item.ID, other.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}
	if err := db.AddDep(ctx, other.ID, item.ID); err != nil {
		t.Fatalf("failed to add reverse dep: %v", err)
	}

	if err := db.DeleteItem(ctx, item.ID); err != nil {
		t.Fatalf("failed to delete labeled item: %v", err)
	}

	if n := countRows(t, db, "items", "id = ?", item.ID); n != 0 {
		t.Errorf("items rows = %d, want 0", n)
	}
	if n := countRows(t, db, "item_labels", "item_id = ?", item.ID); n != 0 {
		t.Errorf("item_labels rows = %d, want 0", n)
	}
	if n := countRows(t, db, "logs", "item_id = ?", item.ID); n != 0 {
		t.Errorf("logs rows = %d, want 0", n)
	}
	if n := countRows(t, db, "deps", "item_id = ?1 OR depends_on = ?1", item.ID); n != 0 {
		t.Errorf("deps rows = %d, want 0", n)
	}
	// The label itself stays in the project; only the association goes.
	labels, err := db.ListLabels(ctx, "test")
	if err != nil {
		t.Fatalf("failed to list labels: %v", err)
	}
	if len(labels) != 1 || labels[0].Name != "bug" {
		t.Errorf("labels = %v, want [bug]", labels)
	}
	// Unrelated item survives.
	if n := countRows(t, db, "items", "id = ?", other.ID); n != 1 {
		t.Errorf("other item rows = %d, want 1", n)
	}
}

func TestDeleteItem_EpicWithChildRefused(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	epic := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Epic",
		Status:  model.StatusOpen,
	})
	child := createTestItem(t, db, "Child")
	blocker := createTestItem(t, db, "Blocker")
	if err := db.SetParent(ctx, child.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}
	if err := db.AddLog(ctx, epic.ID, "epic history"); err != nil {
		t.Fatalf("failed to add log: %v", err)
	}
	if err := db.AddDep(ctx, epic.ID, blocker.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}
	if err := db.AddLabelToItem(ctx, epic.ID, "test", "big"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}

	err := db.DeleteItem(ctx, epic.ID)
	if err == nil {
		t.Fatal("expected error deleting epic with a child")
	}
	if !strings.Contains(err.Error(), "child") {
		t.Errorf("error = %q, want mention of child items", err)
	}

	if n := countRows(t, db, "items", "id = ?", epic.ID); n != 1 {
		t.Errorf("epic rows = %d, want 1", n)
	}
	if n := countRows(t, db, "logs", "item_id = ?", epic.ID); n != 1 {
		t.Errorf("epic logs = %d, want 1", n)
	}
	if n := countRows(t, db, "deps", "item_id = ?", epic.ID); n != 1 {
		t.Errorf("epic deps = %d, want 1", n)
	}
	if n := countRows(t, db, "item_labels", "item_id = ?", epic.ID); n != 1 {
		t.Errorf("epic item_labels = %d, want 1", n)
	}
	if got := loadItem(t, db, child.ID); got.ParentID == nil || *got.ParentID != epic.ID {
		t.Errorf("child parent changed after refused delete: %v", got.ParentID)
	}
}

func TestDeleteItem_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.DeleteItem(context.Background(), "ts-missing")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got %v", err)
	}
}

func TestLoadGraph(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// An epic whose only child is done: a derived status would be done, but
	// the store must report what is stored.
	epic := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeEpic,
		Title:   "Epic",
		Status:  model.StatusOpen,
	})
	child := createItem(t, db, model.NewItem{
		Project: "test",
		Type:    model.ItemTypeTask,
		Title:   "Child",
		Status:  model.StatusDone,
	})
	if err := db.SetParent(ctx, child.ID, epic.ID); err != nil {
		t.Fatalf("failed to set parent: %v", err)
	}
	if err := db.AddLabelToItem(ctx, child.ID, "test", "zeta"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}
	if err := db.AddLabelToItem(ctx, child.ID, "test", "alpha"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}
	if err := db.AddDep(ctx, child.ID, epic.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	// A dangling edge, as left by databases written before deletes were
	// transactional. It can only be inserted with foreign keys off.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to get conn: %v", err)
	}
	for _, stmt := range []string{
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO deps (item_id, depends_on) VALUES ('` + child.ID + `', 'ts-gone')`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			_ = conn.Close()
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close conn: %v", err)
	}

	items, deps, err := db.LoadGraph(ctx)
	if err != nil {
		t.Fatalf("failed to load graph: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	byID := map[string]model.Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if got := byID[epic.ID].Status; got != model.StatusOpen {
		t.Errorf("epic status = %q, want stored %q", got, model.StatusOpen)
	}
	if got := byID[epic.ID].Labels; len(got) != 0 {
		t.Errorf("epic labels = %v, want none", got)
	}
	gotChild := byID[child.ID]
	if len(gotChild.Labels) != 2 || gotChild.Labels[0] != "alpha" || gotChild.Labels[1] != "zeta" {
		t.Errorf("child labels = %v, want [alpha zeta]", gotChild.Labels)
	}
	if gotChild.ParentID == nil || *gotChild.ParentID != epic.ID {
		t.Errorf("child parent = %v, want %s", gotChild.ParentID, epic.ID)
	}

	want := []model.Dep{
		{ItemID: child.ID, DependsOn: epic.ID},
		{ItemID: child.ID, DependsOn: "ts-gone"},
	}
	if len(deps) != len(want) {
		t.Fatalf("deps = %v, want %v", deps, want)
	}
	for i := range want {
		if deps[i] != want[i] {
			t.Errorf("deps[%d] = %v, want %v", i, deps[i], want[i])
		}
	}
}

func TestLoadGraph_Empty(t *testing.T) {
	db := setupTestDB(t)

	items, deps, err := db.LoadGraph(context.Background())
	if err != nil {
		t.Fatalf("failed to load graph: %v", err)
	}
	if len(items) != 0 || len(deps) != 0 {
		t.Errorf("got %d items, %d deps, want none", len(items), len(deps))
	}
}
