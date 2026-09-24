package db

import (
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
)

func TestEnsureProject(t *testing.T) {
	db := setupTestDB(t)

	// First call should create the project
	err := db.EnsureProject("myproject")
	if err != nil {
		t.Fatalf("failed to ensure project: %v", err)
	}

	// Second call should be idempotent (no error)
	err = db.EnsureProject("myproject")
	if err != nil {
		t.Fatalf("failed on second ensure: %v", err)
	}

	// Project should appear in list
	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}

	if len(projects) != 1 || projects[0] != "myproject" {
		t.Errorf("expected [myproject], got %v", projects)
	}
}

func TestListProjectsEmpty(t *testing.T) {
	db := setupTestDB(t)

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("expected empty list, got %v", projects)
	}
}

// Regression: `prog add -p " spindle"` stored the project verbatim, so
// `prog list -p spindle` returned nothing even though show printed "spindle".
func TestCreateItemNormalizesProjectForFiltering(t *testing.T) {
	db := setupTestDB(t)

	item := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   " brandnew\t",
		Type:      model.ItemTypeTask,
		Title:     "task in a brand-new project",
		Status:    model.StatusOpen,
		Priority:  2,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateItem(item); err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	items, err := db.ListItemsFiltered(ListFilter{Project: "brandnew"})
	if err != nil {
		t.Fatalf("failed to list items: %v", err)
	}
	if len(items) != 1 || items[0].ID != item.ID {
		t.Fatalf("list -p brandnew: got %d items, want [%s]", len(items), item.ID)
	}

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projects) != 1 || projects[0] != "brandnew" {
		t.Errorf("projects = %q, want [brandnew]", projects)
	}
}

// Existing rows written before normalization must become filterable after
// the startup migration, without a manual step.
func TestMigrateTrimsExistingProjectNames(t *testing.T) {
	db := setupTestDB(t)

	for _, stmt := range []string{
		`INSERT INTO projects (name) VALUES (' spindle'), ('spindle')`,
		`INSERT INTO items (id, project, type, title, description, status) VALUES
			('ep-old', ' spindle', 'epic', 'old epic', '', 'open'),
			('ts-new', 'spindle', 'task', 'new task', '', 'open')`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Idempotent: a second startup is a no-op.
	if err := db.Migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	items, err := db.ListItemsFiltered(ListFilter{Project: "spindle"})
	if err != nil {
		t.Fatalf("failed to list items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("list -p spindle: got %d items, want 2", len(items))
	}

	projects, err := db.ListProjects()
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projects) != 1 || projects[0] != "spindle" {
		t.Errorf("projects = %q, want [spindle]", projects)
	}
}
