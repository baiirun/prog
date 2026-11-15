package db

import (
	"context"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

func TestEnsureProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if err := db.EnsureProject(ctx, "myproject"); err != nil {
		t.Fatalf("failed to ensure project: %v", err)
	}
	// Idempotent.
	if err := db.EnsureProject(ctx, "myproject"); err != nil {
		t.Fatalf("failed on second ensure: %v", err)
	}

	projects, err := db.ListProjects(ctx)
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projects) != 1 || projects[0] != "myproject" {
		t.Errorf("expected [myproject], got %v", projects)
	}
}

func TestListProjectsSorted(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"zeta", "alpha", "mid"} {
		if err := db.EnsureProject(ctx, name); err != nil {
			t.Fatalf("failed to ensure project %q: %v", name, err)
		}
	}

	projects, err := db.ListProjects(ctx)
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if want := []string{"alpha", "mid", "zeta"}; !equalStrings(projects, want) {
		t.Errorf("projects = %v, want %v", projects, want)
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
		if _, err := db.conn.Exec(stmt); err != nil {
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

	for _, id := range []string{"ep-old", "ts-new"} {
		if got := loadItem(t, db, id); got.Project != "spindle" {
			t.Errorf("%s project = %q, want %q", id, got.Project, "spindle")
		}
	}

	projects, err := db.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projects) != 1 || projects[0] != "spindle" {
		t.Errorf("projects = %q, want [spindle]", projects)
	}
}

// The store stores what it is given; trimming is tasks.Service's job.
func TestCreateItemStoresProjectVerbatim(t *testing.T) {
	db := setupTestDB(t)

	item := createItem(t, db, model.NewItem{
		Project: " raw\t",
		Type:    model.ItemTypeTask,
		Title:   "verbatim",
		Status:  model.StatusOpen,
	})
	if got := loadItem(t, db, item.ID); got.Project != " raw\t" {
		t.Errorf("project = %q, want %q", got.Project, " raw\t")
	}
}
