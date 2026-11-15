package db

import (
	"context"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

func createLabel(t *testing.T, db *DB, in model.NewLabel) {
	t.Helper()
	if err := db.CreateLabel(context.Background(), in); err != nil {
		t.Fatalf("failed to create label %q: %v", in.Name, err)
	}
}

func listLabels(t *testing.T, db *DB, project string) []model.Label {
	t.Helper()
	labels, err := db.ListLabels(context.Background(), project)
	if err != nil {
		t.Fatalf("failed to list labels: %v", err)
	}
	return labels
}

func labelNames(labels []model.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCreateLabel(t *testing.T) {
	db := setupTestDB(t)

	createLabel(t, db, model.NewLabel{Project: "test", Name: "bug", Color: "#ff0000"})

	labels := listLabels(t, db, "test")
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	got := labels[0]
	if got.Name != "bug" {
		t.Errorf("name = %q, want %q", got.Name, "bug")
	}
	if got.Project != "test" {
		t.Errorf("project = %q, want %q", got.Project, "test")
	}
	if got.Color != "#ff0000" {
		t.Errorf("color = %q, want %q", got.Color, "#ff0000")
	}
	if got.ID == "" || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("expected store to assign ID and timestamps, got %+v", got)
	}
}

func TestCreateLabel_DuplicateNameFails(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	createLabel(t, db, model.NewLabel{Project: "test", Name: "bug"})
	if err := db.CreateLabel(ctx, model.NewLabel{Project: "test", Name: "bug"}); err == nil {
		t.Error("expected error creating duplicate label in the same project")
	}
	// The same name in another project is a different label.
	if err := db.CreateLabel(ctx, model.NewLabel{Project: "other", Name: "bug"}); err != nil {
		t.Errorf("same name in another project should succeed: %v", err)
	}
}

func TestListLabels(t *testing.T) {
	db := setupTestDB(t)

	for _, name := range []string{"gamma", "alpha", "beta"} {
		createLabel(t, db, model.NewLabel{Project: "test", Name: name})
	}
	createLabel(t, db, model.NewLabel{Project: "other-project", Name: "other"})

	// Only labels for "test", sorted by name.
	got := labelNames(listLabels(t, db, "test"))
	if want := []string{"alpha", "beta", "gamma"}; !equalStrings(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

func TestRenameLabel(t *testing.T) {
	db := setupTestDB(t)

	createLabel(t, db, model.NewLabel{Project: "test", Name: "old-name"})
	before := listLabels(t, db, "test")

	if err := db.RenameLabel(context.Background(), "test", "old-name", "new-name"); err != nil {
		t.Fatalf("failed to rename label: %v", err)
	}

	after := listLabels(t, db, "test")
	if len(after) != 1 || after[0].Name != "new-name" {
		t.Fatalf("labels = %v, want [new-name]", labelNames(after))
	}
	if after[0].ID != before[0].ID {
		t.Error("renamed label has different ID")
	}
}

func TestRenameLabel_NotFound(t *testing.T) {
	db := setupTestDB(t)

	if err := db.RenameLabel(context.Background(), "test", "missing", "new"); err == nil {
		t.Error("expected error renaming nonexistent label")
	}
}

func TestDeleteLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	createLabel(t, db, model.NewLabel{Project: "test", Name: "to-delete"})
	item := createTestItem(t, db, "Test task")
	if err := db.AddLabelToItem(ctx, item.ID, "test", "to-delete"); err != nil {
		t.Fatalf("failed to add label to item: %v", err)
	}

	if err := db.DeleteLabel(ctx, "test", "to-delete"); err != nil {
		t.Fatalf("failed to delete label: %v", err)
	}

	if labels := listLabels(t, db, "test"); len(labels) != 0 {
		t.Errorf("labels = %v, want none", labelNames(labels))
	}
	if got := loadItem(t, db, item.ID); len(got.Labels) != 0 {
		t.Errorf("item labels = %v, want none", got.Labels)
	}
}

func TestDeleteLabel_NotFound(t *testing.T) {
	db := setupTestDB(t)

	if err := db.DeleteLabel(context.Background(), "test", "missing"); err == nil {
		t.Error("expected error deleting nonexistent label")
	}
}

func TestAddLabelToItem(t *testing.T) {
	db := setupTestDB(t)
	item := createTestItem(t, db, "Test task")

	// Adding a missing label creates it.
	if err := db.AddLabelToItem(context.Background(), item.ID, "test", "new-label"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}

	if got := labelNames(listLabels(t, db, "test")); !equalStrings(got, []string{"new-label"}) {
		t.Errorf("project labels = %v, want [new-label]", got)
	}
	if got := loadItem(t, db, item.ID).Labels; !equalStrings(got, []string{"new-label"}) {
		t.Errorf("item labels = %v, want [new-label]", got)
	}
}

func TestAddLabelToItem_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test task")

	if err := db.AddLabelToItem(ctx, item.ID, "test", "bug"); err != nil {
		t.Fatalf("first add failed: %v", err)
	}
	if err := db.AddLabelToItem(ctx, item.ID, "test", "bug"); err != nil {
		t.Fatalf("second add failed: %v", err)
	}

	if got := loadItem(t, db, item.ID).Labels; !equalStrings(got, []string{"bug"}) {
		t.Errorf("item labels = %v, want [bug]", got)
	}
}

// Attaching an existing label reuses it instead of creating a duplicate.
func TestAddLabelToItem_ReusesExistingLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	createLabel(t, db, model.NewLabel{Project: "test", Name: "shared", Color: "#00ff00"})
	first := createTestItem(t, db, "First")
	second := createTestItem(t, db, "Second")

	for _, item := range []model.Item{first, second} {
		if err := db.AddLabelToItem(ctx, item.ID, "test", "shared"); err != nil {
			t.Fatalf("failed to add label to %s: %v", item.ID, err)
		}
	}

	labels := listLabels(t, db, "test")
	if len(labels) != 1 {
		t.Fatalf("labels = %v, want one shared label", labelNames(labels))
	}
	if labels[0].Color != "#00ff00" {
		t.Errorf("color = %q, want existing label's %q", labels[0].Color, "#00ff00")
	}
	if n := countRows(t, db, "item_labels", "label_id = ?", labels[0].ID); n != 2 {
		t.Errorf("label attached to %d items, want 2", n)
	}
}

func TestRemoveLabelFromItem(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test task")

	if err := db.AddLabelToItem(ctx, item.ID, "test", "temp"); err != nil {
		t.Fatalf("failed to add label: %v", err)
	}
	if err := db.RemoveLabelFromItem(ctx, item.ID, "test", "temp"); err != nil {
		t.Fatalf("failed to remove label: %v", err)
	}

	if got := loadItem(t, db, item.ID).Labels; len(got) != 0 {
		t.Errorf("item labels = %v, want none", got)
	}
	// The label is only detached, not deleted.
	if got := labelNames(listLabels(t, db, "test")); !equalStrings(got, []string{"temp"}) {
		t.Errorf("project labels = %v, want [temp]", got)
	}
}

func TestRemoveLabelFromItem_NotAttached(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test task")

	createLabel(t, db, model.NewLabel{Project: "test", Name: "unused"})
	if err := db.RemoveLabelFromItem(ctx, item.ID, "test", "unused"); err == nil {
		t.Error("expected error removing a label the item does not carry")
	}
	if err := db.RemoveLabelFromItem(ctx, item.ID, "test", "missing"); err == nil {
		t.Error("expected error removing a nonexistent label")
	}
}

func TestItemLabels_MultipleSorted(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	item := createTestItem(t, db, "Test task")

	for _, name := range []string{"bug", "urgent", "backend"} {
		if err := db.AddLabelToItem(ctx, item.ID, "test", name); err != nil {
			t.Fatalf("failed to add label %s: %v", name, err)
		}
	}

	got := loadItem(t, db, item.ID).Labels
	if want := []string{"backend", "bug", "urgent"}; !equalStrings(got, want) {
		t.Errorf("item labels = %v, want %v", got, want)
	}
}
