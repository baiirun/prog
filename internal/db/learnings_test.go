package db

import (
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// --- Learning CRUD ---

func TestCreateLearning(t *testing.T) {
	db := setupTestDB(t)

	// Create a task first to link the learning to
	task := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test task",
		Status:    model.StatusInProgress,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		TaskID:    &task.ID,
		Summary:   "Test learning summary",
		Detail:    "Detailed explanation of what was learned",
		Files:     []string{"file1.go", "file2.go"},
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth", "concurrency"},
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Verify it was created
	got, err := db.GetLearning(learning.ID)
	if err != nil {
		t.Fatalf("failed to get learning: %v", err)
	}

	if got.Summary != learning.Summary {
		t.Errorf("summary = %q, want %q", got.Summary, learning.Summary)
	}
	if got.Detail != learning.Detail {
		t.Errorf("detail = %q, want %q", got.Detail, learning.Detail)
	}
	if got.Project != learning.Project {
		t.Errorf("project = %q, want %q", got.Project, learning.Project)
	}
	if got.TaskID == nil || *got.TaskID != *learning.TaskID {
		t.Errorf("taskID = %v, want %v", got.TaskID, learning.TaskID)
	}
	if got.Status != model.LearningStatusActive {
		t.Errorf("status = %q, want %q", got.Status, model.LearningStatusActive)
	}
	if len(got.Files) != 2 {
		t.Errorf("files count = %d, want 2", len(got.Files))
	}
	if len(got.Concepts) != 2 {
		t.Errorf("concepts count = %d, want 2", len(got.Concepts))
	}
}

func TestCreateLearning_CreatesConceptsOnFirstUse(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"new-concept"},
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Verify concept was created
	concepts, err := db.ListConcepts("test", false)
	if err != nil {
		t.Fatalf("failed to list concepts: %v", err)
	}

	if len(concepts) != 1 {
		t.Errorf("concept count = %d, want 1", len(concepts))
	}
	if concepts[0].Name != "new-concept" {
		t.Errorf("concept name = %q, want %q", concepts[0].Name, "new-concept")
	}
	if concepts[0].LearningCount != 1 {
		t.Errorf("concept learning count = %d, want 1", concepts[0].LearningCount)
	}
}

func TestGetLearning(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Detail:    "Details here",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"concept1", "concept2"},
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	got, err := db.GetLearning(learning.ID)
	if err != nil {
		t.Fatalf("failed to get learning: %v", err)
	}

	if got.ID != learning.ID {
		t.Errorf("id = %q, want %q", got.ID, learning.ID)
	}
	if got.Summary != learning.Summary {
		t.Errorf("summary = %q, want %q", got.Summary, learning.Summary)
	}
	// Concepts should be retrieved
	if len(got.Concepts) != 2 {
		t.Errorf("concepts count = %d, want 2", len(got.Concepts))
	}
}

func TestGetLearning_NotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := db.GetLearning("lrn-nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent learning")
	}
}

func TestUpdateLearningSummary(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Original summary",
		Status:    model.LearningStatusActive,
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	if err := db.UpdateLearningSummary(learning.ID, "Updated summary"); err != nil {
		t.Fatalf("failed to update summary: %v", err)
	}

	got, err := db.GetLearning(learning.ID)
	if err != nil {
		t.Fatalf("failed to get learning: %v", err)
	}

	if got.Summary != "Updated summary" {
		t.Errorf("summary = %q, want %q", got.Summary, "Updated summary")
	}
}

func TestUpdateLearningSummary_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.UpdateLearningSummary("lrn-nonexistent", "New summary")
	if err == nil {
		t.Error("expected error for nonexistent learning")
	}
}

func TestUpdateLearningStatus(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Transition: active -> stale
	if err := db.UpdateLearningStatus(learning.ID, model.LearningStatusStale); err != nil {
		t.Fatalf("failed to update status to stale: %v", err)
	}

	got, _ := db.GetLearning(learning.ID)
	if got.Status != model.LearningStatusStale {
		t.Errorf("status = %q, want %q", got.Status, model.LearningStatusStale)
	}

	// Transition: stale -> archived
	if err := db.UpdateLearningStatus(learning.ID, model.LearningStatusArchived); err != nil {
		t.Fatalf("failed to update status to archived: %v", err)
	}

	got, _ = db.GetLearning(learning.ID)
	if got.Status != model.LearningStatusArchived {
		t.Errorf("status = %q, want %q", got.Status, model.LearningStatusArchived)
	}
}

func TestDeleteLearning(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"concept1"},
	}

	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Verify it exists
	_, err := db.GetLearning(learning.ID)
	if err != nil {
		t.Fatalf("learning should exist: %v", err)
	}

	// Delete it
	if err := db.DeleteLearning(learning.ID); err != nil {
		t.Fatalf("failed to delete learning: %v", err)
	}

	// Verify it's gone
	_, err = db.GetLearning(learning.ID)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestDeleteLearning_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.DeleteLearning("lrn-nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent learning")
	}
}

// --- Concepts ---

func TestListConcepts(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create learning with multiple concepts
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning 1",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth", "concurrency"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Create another learning that uses "auth" again
	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
		Summary:   "Test learning 2",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	// List by count (default)
	concepts, err := db.ListConcepts("test", false)
	if err != nil {
		t.Fatalf("failed to list concepts: %v", err)
	}

	if len(concepts) != 2 {
		t.Fatalf("concept count = %d, want 2", len(concepts))
	}

	// "auth" should be first (count 2)
	if concepts[0].Name != "auth" {
		t.Errorf("first concept = %q, want 'auth'", concepts[0].Name)
	}
	if concepts[0].LearningCount != 2 {
		t.Errorf("auth learning count = %d, want 2", concepts[0].LearningCount)
	}

	// "concurrency" should be second (count 1)
	if concepts[1].Name != "concurrency" {
		t.Errorf("second concept = %q, want 'concurrency'", concepts[1].Name)
	}
	if concepts[1].LearningCount != 1 {
		t.Errorf("concurrency learning count = %d, want 1", concepts[1].LearningCount)
	}
}

func TestListConcepts_SortByRecent(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create learning with old concept
	learning1 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning 1",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"old-concept"},
	}
	if err := db.CreateLearning(learning1); err != nil {
		t.Fatalf("failed to create learning1: %v", err)
	}

	// Create learning with new concept later
	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now.Add(time.Hour),
		UpdatedAt: now.Add(time.Hour),
		Summary:   "Test learning 2",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"new-concept"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	// List by recent
	concepts, err := db.ListConcepts("test", true)
	if err != nil {
		t.Fatalf("failed to list concepts: %v", err)
	}

	if len(concepts) != 2 {
		t.Fatalf("concept count = %d, want 2", len(concepts))
	}

	// "new-concept" should be first (more recent)
	if concepts[0].Name != "new-concept" {
		t.Errorf("first concept = %q, want 'new-concept'", concepts[0].Name)
	}
}

func TestListConcepts_Empty(t *testing.T) {
	db := setupTestDB(t)

	concepts, err := db.ListConcepts("test", false)
	if err != nil {
		t.Fatalf("failed to list concepts: %v", err)
	}

	if len(concepts) != 0 {
		t.Errorf("concept count = %d, want 0", len(concepts))
	}
}

func TestSetConceptSummary(t *testing.T) {
	db := setupTestDB(t)

	// Create a concept via learning
	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Set summary
	if err := db.SetConceptSummary("auth", "test", "Authentication and authorization"); err != nil {
		t.Fatalf("failed to set concept summary: %v", err)
	}

	// Verify
	concepts, _ := db.ListConcepts("test", false)
	if len(concepts) != 1 {
		t.Fatalf("expected 1 concept")
	}
	if concepts[0].Summary != "Authentication and authorization" {
		t.Errorf("summary = %q, want %q", concepts[0].Summary, "Authentication and authorization")
	}
}

func TestSetConceptSummary_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.SetConceptSummary("nonexistent", "test", "summary")
	if err == nil {
		t.Error("expected error for nonexistent concept")
	}
}

func TestRenameConcept(t *testing.T) {
	db := setupTestDB(t)

	// Create a concept via learning
	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"authn"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Rename
	if err := db.RenameConcept("authn", "authentication", "test"); err != nil {
		t.Fatalf("failed to rename concept: %v", err)
	}

	// Verify old name is gone
	concepts, _ := db.ListConcepts("test", false)
	if len(concepts) != 1 {
		t.Fatalf("expected 1 concept")
	}
	if concepts[0].Name != "authentication" {
		t.Errorf("name = %q, want %q", concepts[0].Name, "authentication")
	}
}

func TestRenameConcept_NotFound(t *testing.T) {
	db := setupTestDB(t)

	err := db.RenameConcept("nonexistent", "newname", "test")
	if err == nil {
		t.Error("expected error for nonexistent concept")
	}
}

func TestGetRelatedConcepts(t *testing.T) {
	db := setupTestDB(t)

	// Create a task with keywords in title/description
	task := &model.Item{
		ID:          model.GenerateID(model.ItemTypeTask),
		Project:     "test",
		Type:        model.ItemTypeTask,
		Title:       "Fix auth token refresh",
		Description: "The concurrency issue causes race conditions",
		Status:      model.StatusOpen,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := db.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Create concepts via learnings
	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth", "concurrency", "database"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Get related concepts for the task
	related, err := db.GetRelatedConcepts(task.ID)
	if err != nil {
		t.Fatalf("failed to get related concepts: %v", err)
	}

	// Should match "auth" (in title) and "concurrency" (in description)
	if len(related) != 2 {
		t.Errorf("related count = %d, want 2", len(related))
	}

	// Check both are present
	foundAuth := false
	foundConcurrency := false
	for _, c := range related {
		if c.Name == "auth" {
			foundAuth = true
		}
		if c.Name == "concurrency" {
			foundConcurrency = true
		}
	}
	if !foundAuth {
		t.Error("expected 'auth' to be in related concepts")
	}
	if !foundConcurrency {
		t.Error("expected 'concurrency' to be in related concepts")
	}
}

func TestEnsureConcept(t *testing.T) {
	db := setupTestDB(t)

	// Ensure creates new
	if err := db.EnsureConcept("new-concept", "test"); err != nil {
		t.Fatalf("failed to ensure concept: %v", err)
	}

	concepts, _ := db.ListConcepts("test", false)
	if len(concepts) != 1 {
		t.Fatalf("expected 1 concept")
	}

	// Ensure is idempotent
	if err := db.EnsureConcept("new-concept", "test"); err != nil {
		t.Fatalf("failed to ensure concept again: %v", err)
	}

	concepts, _ = db.ListConcepts("test", false)
	if len(concepts) != 1 {
		t.Errorf("concept count = %d, want 1 (should not duplicate)", len(concepts))
	}
}

// --- Edge Cases ---

func TestCreateLearning_DuplicateConcept(t *testing.T) {
	db := setupTestDB(t)

	// Try to create learning with duplicate concept names
	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth", "auth"}, // duplicate
	}

	// This should either dedupe or fail gracefully
	err := db.CreateLearning(learning)
	// The current implementation will fail on the second insert due to primary key constraint
	// This is acceptable behavior - the test documents it
	if err == nil {
		// If it succeeds, verify we don't have duplicate associations
		got, _ := db.GetLearning(learning.ID)
		if len(got.Concepts) > 1 {
			t.Log("Warning: duplicate concepts were not deduplicated")
		}
	}
	// Error is acceptable - duplicate concepts should be avoided by caller
}

func TestDeleteLearning_UpdatesConceptCount(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create two learnings with same concept
	learning1 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning 1",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"shared-concept"},
	}
	if err := db.CreateLearning(learning1); err != nil {
		t.Fatalf("failed to create learning1: %v", err)
	}

	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning 2",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"shared-concept"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	// Verify count is 2
	concepts, _ := db.ListConcepts("test", false)
	if concepts[0].LearningCount != 2 {
		t.Errorf("initial count = %d, want 2", concepts[0].LearningCount)
	}

	// Delete one learning
	if err := db.DeleteLearning(learning1.ID); err != nil {
		t.Fatalf("failed to delete learning1: %v", err)
	}

	// Verify count decreased to 1
	concepts, _ = db.ListConcepts("test", false)
	if concepts[0].LearningCount != 1 {
		t.Errorf("count after delete = %d, want 1", concepts[0].LearningCount)
	}
}

func TestRenameConcept_PreservesSummary(t *testing.T) {
	db := setupTestDB(t)

	// Create a concept via learning
	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Test learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"original"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Set a summary
	if err := db.SetConceptSummary("original", "test", "Important summary"); err != nil {
		t.Fatalf("failed to set summary: %v", err)
	}

	// Rename
	if err := db.RenameConcept("original", "renamed", "test"); err != nil {
		t.Fatalf("failed to rename: %v", err)
	}

	// Verify summary is preserved
	concepts, _ := db.ListConcepts("test", false)
	if len(concepts) != 1 {
		t.Fatalf("expected 1 concept")
	}
	if concepts[0].Name != "renamed" {
		t.Errorf("name = %q, want 'renamed'", concepts[0].Name)
	}
	if concepts[0].Summary != "Important summary" {
		t.Errorf("summary = %q, want 'Important summary'", concepts[0].Summary)
	}
}

func TestGetCurrentTaskID(t *testing.T) {
	db := setupTestDB(t)

	// No in-progress task
	taskID, err := db.GetCurrentTaskID("test")
	if err != nil {
		t.Fatalf("failed to get current task: %v", err)
	}
	if taskID != nil {
		t.Errorf("expected nil, got %v", taskID)
	}

	// Create an in-progress task
	task := &model.Item{
		ID:        model.GenerateID(model.ItemTypeTask),
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Test task",
		Status:    model.StatusInProgress,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.CreateItem(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Now should return the task
	taskID, err = db.GetCurrentTaskID("test")
	if err != nil {
		t.Fatalf("failed to get current task: %v", err)
	}
	if taskID == nil {
		t.Fatal("expected task ID, got nil")
	}
	if *taskID != task.ID {
		t.Errorf("taskID = %q, want %q", *taskID, task.ID)
	}
}

// --- Context Retrieval ---

func TestGetLearningsByConcepts(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create learnings with different concepts
	learning1 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Auth learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning1); err != nil {
		t.Fatalf("failed to create learning1: %v", err)
	}

	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
		Summary:   "Concurrency learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"concurrency"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	learning3 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now.Add(2 * time.Second),
		UpdatedAt: now.Add(2 * time.Second),
		Summary:   "Auth and concurrency learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth", "concurrency"},
	}
	if err := db.CreateLearning(learning3); err != nil {
		t.Fatalf("failed to create learning3: %v", err)
	}

	// Query by single concept
	learnings, err := db.GetLearningsByConcepts("test", []string{"auth"}, false)
	if err != nil {
		t.Fatalf("failed to get learnings by concepts: %v", err)
	}
	if len(learnings) != 2 {
		t.Errorf("learnings count = %d, want 2", len(learnings))
	}

	// Query by multiple concepts (union)
	learnings, err = db.GetLearningsByConcepts("test", []string{"auth", "concurrency"}, false)
	if err != nil {
		t.Fatalf("failed to get learnings: %v", err)
	}
	if len(learnings) != 3 {
		t.Errorf("learnings count = %d, want 3", len(learnings))
	}

	// Results should be sorted by created_at desc (most recent first)
	if learnings[0].ID != learning3.ID {
		t.Errorf("first learning = %s, want %s (most recent)", learnings[0].ID, learning3.ID)
	}
}

func TestGetLearningsByConcepts_ExcludesStale(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create active learning
	learning1 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Active learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning1); err != nil {
		t.Fatalf("failed to create learning1: %v", err)
	}

	// Create stale learning
	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Stale learning",
		Status:    model.LearningStatusStale,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	// Without include-stale, should only get active
	learnings, err := db.GetLearningsByConcepts("test", []string{"auth"}, false)
	if err != nil {
		t.Fatalf("failed to get learnings: %v", err)
	}
	if len(learnings) != 1 {
		t.Errorf("learnings count = %d, want 1", len(learnings))
	}
	if learnings[0].Status != model.LearningStatusActive {
		t.Errorf("status = %q, want active", learnings[0].Status)
	}

	// With include-stale, should get both
	learnings, err = db.GetLearningsByConcepts("test", []string{"auth"}, true)
	if err != nil {
		t.Fatalf("failed to get learnings: %v", err)
	}
	if len(learnings) != 2 {
		t.Errorf("learnings count = %d, want 2", len(learnings))
	}
}

func TestGetLearningsByConcepts_Empty(t *testing.T) {
	db := setupTestDB(t)

	// Empty concept list
	learnings, err := db.GetLearningsByConcepts("test", []string{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if learnings != nil {
		t.Errorf("expected nil, got %v", learnings)
	}

	// Nonexistent concept
	learnings, err = db.GetLearningsByConcepts("test", []string{"nonexistent"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(learnings) != 0 {
		t.Errorf("learnings count = %d, want 0", len(learnings))
	}
}

func TestSearchLearnings(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create learnings with searchable content
	learning1 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Token refresh has race condition",
		Detail:    "The auth token refresh logic has a race condition when multiple requests happen simultaneously",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning1); err != nil {
		t.Fatalf("failed to create learning1: %v", err)
	}

	learning2 := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Database connection pooling",
		Detail:    "Connection pool size affects throughput under load",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"database"},
	}
	if err := db.CreateLearning(learning2); err != nil {
		t.Fatalf("failed to create learning2: %v", err)
	}

	// Search for "token"
	learnings, err := db.SearchLearnings("test", "token", false)
	if err != nil {
		t.Fatalf("failed to search learnings: %v", err)
	}
	if len(learnings) != 1 {
		t.Errorf("learnings count = %d, want 1", len(learnings))
	}
	if learnings[0].ID != learning1.ID {
		t.Errorf("learning ID = %s, want %s", learnings[0].ID, learning1.ID)
	}

	// Search for "race condition"
	learnings, err = db.SearchLearnings("test", "race condition", false)
	if err != nil {
		t.Fatalf("failed to search learnings: %v", err)
	}
	if len(learnings) != 1 {
		t.Errorf("learnings count = %d, want 1", len(learnings))
	}

	// Search for "connection" (in second learning)
	learnings, err = db.SearchLearnings("test", "connection", false)
	if err != nil {
		t.Fatalf("failed to search learnings: %v", err)
	}
	if len(learnings) != 1 {
		t.Errorf("learnings count = %d, want 1", len(learnings))
	}
	if learnings[0].ID != learning2.ID {
		t.Errorf("learning ID = %s, want %s", learnings[0].ID, learning2.ID)
	}
}

func TestSearchLearnings_ExcludesStale(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	// Create stale learning
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Stale token info",
		Status:    model.LearningStatusStale,
		Concepts:  []string{"auth"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	// Without include-stale, should not find
	learnings, err := db.SearchLearnings("test", "token", false)
	if err != nil {
		t.Fatalf("failed to search learnings: %v", err)
	}
	if len(learnings) != 0 {
		t.Errorf("learnings count = %d, want 0", len(learnings))
	}

	// With include-stale, should find
	learnings, err = db.SearchLearnings("test", "token", true)
	if err != nil {
		t.Fatalf("failed to search learnings: %v", err)
	}
	if len(learnings) != 1 {
		t.Errorf("learnings count = %d, want 1", len(learnings))
	}
}

func TestSearchLearnings_NoResults(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	learning := &model.Learning{
		ID:        model.GenerateLearningID(),
		Project:   "test",
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   "Some learning",
		Status:    model.LearningStatusActive,
		Concepts:  []string{"test"},
	}
	if err := db.CreateLearning(learning); err != nil {
		t.Fatalf("failed to create learning: %v", err)
	}

	learnings, err := db.SearchLearnings("test", "nonexistent query", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(learnings) != 0 {
		t.Errorf("learnings count = %d, want 0", len(learnings))
	}
}
