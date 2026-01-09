package db

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// CreateLearning inserts a new learning and its concept associations.
// Creates concepts that don't exist yet.
func (db *DB) CreateLearning(l *model.Learning) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize files to JSON
	filesJSON := "[]"
	if len(l.Files) > 0 {
		b, err := json.Marshal(l.Files)
		if err != nil {
			return fmt.Errorf("failed to marshal files: %w", err)
		}
		filesJSON = string(b)
	}

	// Insert learning
	_, err = tx.Exec(`
		INSERT INTO learnings (id, project, created_at, updated_at, task_id, summary, detail, files, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, l.ID, l.Project, l.CreatedAt, l.UpdatedAt, l.TaskID, l.Summary, l.Detail, filesJSON, l.Status)
	if err != nil {
		return fmt.Errorf("failed to insert learning: %w", err)
	}

	// Ensure concepts exist and create associations
	for _, conceptName := range l.Concepts {
		// Check if concept exists
		var conceptID string
		err = tx.QueryRow(`SELECT id FROM concepts WHERE name = ? AND project = ?`, conceptName, l.Project).Scan(&conceptID)
		if err != nil {
			// Concept doesn't exist, create it
			conceptID = model.GenerateConceptID()
			_, err = tx.Exec(`
				INSERT INTO concepts (id, name, project, last_updated)
				VALUES (?, ?, ?, ?)
			`, conceptID, conceptName, l.Project, l.UpdatedAt)
			if err != nil {
				return fmt.Errorf("failed to create concept %q: %w", conceptName, err)
			}
		} else {
			// Update last_updated
			_, err = tx.Exec(`UPDATE concepts SET last_updated = ? WHERE id = ?`, l.UpdatedAt, conceptID)
			if err != nil {
				return fmt.Errorf("failed to update concept %q: %w", conceptName, err)
			}
		}

		// Create association
		_, err = tx.Exec(`
			INSERT INTO learning_concepts (learning_id, concept_id)
			VALUES (?, ?)
		`, l.ID, conceptID)
		if err != nil {
			return fmt.Errorf("failed to create concept association: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetLearning retrieves a learning by ID.
func (db *DB) GetLearning(id string) (*model.Learning, error) {
	var l model.Learning
	var filesJSON string
	var taskID *string

	err := db.QueryRow(`
		SELECT id, project, created_at, updated_at, task_id, summary, detail, files, status
		FROM learnings WHERE id = ?
	`, id).Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID, &l.Summary, &l.Detail, &filesJSON, &l.Status)
	if err != nil {
		return nil, fmt.Errorf("learning not found: %s", id)
	}
	l.TaskID = taskID

	// Parse files JSON
	if filesJSON != "" && filesJSON != "[]" {
		if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
			return nil, fmt.Errorf("failed to unmarshal files: %w", err)
		}
	}

	// Get associated concepts
	rows, err := db.Query(`
		SELECT c.name FROM learning_concepts lc
		JOIN concepts c ON c.id = lc.concept_id
		WHERE lc.learning_id = ?
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get concepts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var concept string
		if err := rows.Scan(&concept); err != nil {
			return nil, fmt.Errorf("failed to scan concept: %w", err)
		}
		l.Concepts = append(l.Concepts, concept)
	}

	return &l, nil
}

// GetCurrentTaskID returns the ID of the first in-progress task for a project.
// Returns nil if no task is in progress.
func (db *DB) GetCurrentTaskID(project string) (*string, error) {
	var taskID string
	err := db.QueryRow(`
		SELECT id FROM items
		WHERE status = 'in_progress' AND project = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, project).Scan(&taskID)
	if err != nil {
		return nil, nil // No task in progress, not an error
	}
	return &taskID, nil
}

// ListConcepts returns all concepts for a project, sorted by learning count (most used first).
func (db *DB) ListConcepts(project string, sortByRecent bool) ([]model.Concept, error) {
	orderBy := "count DESC, c.name"
	if sortByRecent {
		orderBy = "c.last_updated DESC, c.name"
	}

	rows, err := db.Query(`
		SELECT c.id, c.name, c.project, c.summary, c.last_updated,
			(SELECT COUNT(*) FROM learning_concepts lc WHERE lc.concept_id = c.id) as count
		FROM concepts c
		WHERE c.project = ?
		ORDER BY `+orderBy, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list concepts: %w", err)
	}
	defer rows.Close()

	var concepts []model.Concept
	for rows.Next() {
		var c model.Concept
		var summary *string
		if err := rows.Scan(&c.ID, &c.Name, &c.Project, &summary, &c.LastUpdated, &c.LearningCount); err != nil {
			return nil, fmt.Errorf("failed to scan concept: %w", err)
		}
		if summary != nil {
			c.Summary = *summary
		}
		concepts = append(concepts, c)
	}

	return concepts, nil
}

// EnsureConcept creates a concept if it doesn't exist.
func (db *DB) EnsureConcept(name, project string) error {
	_, err := db.Exec(`
		INSERT INTO concepts (id, name, project, last_updated)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (name, project) DO NOTHING
	`, model.GenerateConceptID(), name, project, time.Now())
	return err
}

// SetConceptSummary updates a concept's summary.
func (db *DB) SetConceptSummary(name, project, summary string) error {
	result, err := db.Exec(`
		UPDATE concepts SET summary = ?, last_updated = ?
		WHERE name = ? AND project = ?
	`, summary, time.Now(), name, project)
	if err != nil {
		return fmt.Errorf("failed to update concept: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("concept not found: %s", name)
	}
	return nil
}

// RenameConcept changes a concept's name.
func (db *DB) RenameConcept(oldName, newName, project string) error {
	result, err := db.Exec(`
		UPDATE concepts SET name = ?, last_updated = ?
		WHERE name = ? AND project = ?
	`, newName, time.Now(), oldName, project)
	if err != nil {
		return fmt.Errorf("failed to rename concept: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("concept not found: %s", oldName)
	}
	return nil
}

// GetRelatedConcepts returns concepts that match keywords in a task's title/description.
// Matches are case-insensitive and ranked by learning count.
func (db *DB) GetRelatedConcepts(taskID string) ([]model.Concept, error) {
	// Get task details
	item, err := db.GetItem(taskID)
	if err != nil {
		return nil, err
	}

	// Get all concepts for this project
	concepts, err := db.ListConcepts(item.Project, false)
	if err != nil {
		return nil, err
	}

	if len(concepts) == 0 {
		return nil, nil
	}

	// Build search text from title and description
	searchText := strings.ToLower(item.Title + " " + item.Description)

	// Filter concepts whose name appears in the search text
	var related []model.Concept
	for _, c := range concepts {
		if strings.Contains(searchText, strings.ToLower(c.Name)) {
			related = append(related, c)
		}
	}

	return related, nil
}
