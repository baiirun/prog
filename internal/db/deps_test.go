package db

import (
	"context"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

func TestAddDep(t *testing.T) {
	db := setupTestDB(t)

	task1 := createTestItem(t, db, "Task 1")
	task2 := createTestItem(t, db, "Task 2")

	if err := db.AddDep(context.Background(), task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}

	deps := loadDeps(t, db)
	want := model.Dep{ItemID: task2.ID, DependsOn: task1.ID}
	if len(deps) != 1 || deps[0] != want {
		t.Errorf("deps = %v, want [%v]", deps, want)
	}
}

func TestAddDep_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	task1 := createTestItem(t, db, "Task 1")
	task2 := createTestItem(t, db, "Task 2")

	if err := db.AddDep(ctx, task2.ID, task1.ID); err != nil {
		t.Fatalf("failed to add dep: %v", err)
	}
	// Adding a duplicate is idempotent.
	if err := db.AddDep(ctx, task2.ID, task1.ID); err != nil {
		t.Errorf("duplicate dep should not error: %v", err)
	}

	if deps := loadDeps(t, db); len(deps) != 1 {
		t.Errorf("expected 1 dep after duplicate, got %v", deps)
	}
}
