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

// UpdateLearningSummary updates a learning's summary.
func (db *DB) UpdateLearningSummary(id, summary string) error {
	result, err := db.Exec(`
		UPDATE learnings SET summary = ?, updated_at = ?
		WHERE id = ?
	`, summary, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// UpdateLearningStatus updates a learning's status (active, stale, archived).
func (db *DB) UpdateLearningStatus(id string, status model.LearningStatus) error {
	result, err := db.Exec(`
		UPDATE learnings SET status = ?, updated_at = ?
		WHERE id = ?
	`, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// UpdateLearningDetail updates a learning's detail.
func (db *DB) UpdateLearningDetail(id, detail string) error {
	result, err := db.Exec(`
		UPDATE learnings SET detail = ?, updated_at = ?
		WHERE id = ?
	`, detail, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update learning detail: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}
	return nil
}

// DeleteLearning removes a learning and its concept associations.
func (db *DB) DeleteLearning(id string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete concept associations
	_, err = tx.Exec(`DELETE FROM learning_concepts WHERE learning_id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete concept associations: %w", err)
	}

	// Delete learning
	result, err := tx.Exec(`DELETE FROM learnings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete learning: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("learning not found: %s", id)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
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

// GetLearningsByConcepts returns learnings that have any of the specified concepts.
// Only returns active learnings by default. Results are sorted by created_at desc.
func (db *DB) GetLearningsByConcepts(project string, conceptNames []string, includeStale bool) ([]model.Learning, error) {
	if len(conceptNames) == 0 {
		return nil, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(conceptNames))
	args := make([]interface{}, 0, len(conceptNames)+2)
	args = append(args, project)
	for i, name := range conceptNames {
		placeholders[i] = "?"
		args = append(args, name)
	}

	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	query := `
		SELECT DISTINCT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		JOIN learning_concepts lc ON lc.learning_id = l.id
		JOIN concepts c ON c.id = lc.concept_id
		WHERE l.project = ? AND c.name IN (` + strings.Join(placeholders, ",") + `)
		` + statusFilter + `
		ORDER BY l.created_at DESC
	`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query learnings: %w", err)
	}
	defer rows.Close()

	var learnings []model.Learning
	for rows.Next() {
		var l model.Learning
		var filesJSON string
		var taskID *string
		if err := rows.Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID,
			&l.Summary, &l.Detail, &filesJSON, &l.Status); err != nil {
			return nil, fmt.Errorf("failed to scan learning: %w", err)
		}
		l.TaskID = taskID

		// Parse files JSON
		if filesJSON != "" && filesJSON != "[]" {
			if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
				return nil, fmt.Errorf("failed to unmarshal files: %w", err)
			}
		}

		// Get associated concepts
		conceptRows, err := db.Query(`
			SELECT c.name FROM learning_concepts lc
			JOIN concepts c ON c.id = lc.concept_id
			WHERE lc.learning_id = ?
		`, l.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get concepts: %w", err)
		}
		for conceptRows.Next() {
			var concept string
			if err := conceptRows.Scan(&concept); err != nil {
				conceptRows.Close()
				return nil, fmt.Errorf("failed to scan concept: %w", err)
			}
			l.Concepts = append(l.Concepts, concept)
		}
		conceptRows.Close()

		learnings = append(learnings, l)
	}

	return learnings, nil
}

// SearchLearnings performs full-text search on learnings.
// Returns learnings matching the query, sorted by relevance.
func (db *DB) SearchLearnings(project string, query string, includeStale bool) ([]model.Learning, error) {
	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	sqlQuery := `
		SELECT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		JOIN learnings_fts fts ON l.rowid = fts.rowid
		WHERE learnings_fts MATCH ? AND l.project = ?
		` + statusFilter + `
		ORDER BY rank
	`

	rows, err := db.Query(sqlQuery, query, project)
	if err != nil {
		return nil, fmt.Errorf("failed to search learnings: %w", err)
	}
	defer rows.Close()

	var learnings []model.Learning
	for rows.Next() {
		var l model.Learning
		var filesJSON string
		var taskID *string
		if err := rows.Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID,
			&l.Summary, &l.Detail, &filesJSON, &l.Status); err != nil {
			return nil, fmt.Errorf("failed to scan learning: %w", err)
		}
		l.TaskID = taskID

		// Parse files JSON
		if filesJSON != "" && filesJSON != "[]" {
			if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
				return nil, fmt.Errorf("failed to unmarshal files: %w", err)
			}
		}

		// Get associated concepts
		conceptRows, err := db.Query(`
			SELECT c.name FROM learning_concepts lc
			JOIN concepts c ON c.id = lc.concept_id
			WHERE lc.learning_id = ?
		`, l.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get concepts: %w", err)
		}
		for conceptRows.Next() {
			var concept string
			if err := conceptRows.Scan(&concept); err != nil {
				conceptRows.Close()
				return nil, fmt.Errorf("failed to scan concept: %w", err)
			}
			l.Concepts = append(l.Concepts, concept)
		}
		conceptRows.Close()

		learnings = append(learnings, l)
	}

	return learnings, nil
}

// ConceptStats holds statistics for a concept.
type ConceptStats struct {
	Name          string
	LearningCount int
	OldestAge     *time.Duration // nil if no learnings
}

// ListConceptsWithStats returns all concepts with learning count and oldest learning age.
func (db *DB) ListConceptsWithStats(project string) ([]ConceptStats, error) {
	rows, err := db.Query(`
		SELECT c.name,
			COUNT(l.id) as count,
			MIN(l.created_at) as oldest
		FROM concepts c
		LEFT JOIN learning_concepts lc ON lc.concept_id = c.id
		LEFT JOIN learnings l ON l.id = lc.learning_id AND l.status = 'active'
		WHERE c.project = ?
		GROUP BY c.id
		ORDER BY count DESC, c.name
	`, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list concept stats: %w", err)
	}
	defer rows.Close()

	var stats []ConceptStats
	now := time.Now()
	for rows.Next() {
		var s ConceptStats
		var oldestStr *string
		if err := rows.Scan(&s.Name, &s.LearningCount, &oldestStr); err != nil {
			return nil, fmt.Errorf("failed to scan concept stats: %w", err)
		}
		if oldestStr != nil && *oldestStr != "" {
			// Parse the timestamp string - Go's default format with monotonic clock suffix
			// Format: "2006-01-02 15:04:05.999999999 -0700 MST m=+0.000000000"
			str := *oldestStr
			// Strip monotonic clock suffix if present
			if idx := strings.Index(str, " m="); idx > 0 {
				str = str[:idx]
			}
			oldest, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", str)
			if err != nil {
				oldest, err = time.Parse(time.RFC3339Nano, str)
			}
			if err == nil {
				age := now.Sub(oldest)
				s.OldestAge = &age
			}
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetAllLearnings returns all learnings for a project, sorted by created_at desc.
// Only returns active learnings by default.
func (db *DB) GetAllLearnings(project string, includeStale bool) ([]model.Learning, error) {
	statusFilter := "AND l.status = 'active'"
	if includeStale {
		statusFilter = "AND l.status IN ('active', 'stale')"
	}

	query := `
		SELECT l.id, l.project, l.created_at, l.updated_at, l.task_id,
			l.summary, l.detail, l.files, l.status
		FROM learnings l
		WHERE l.project = ?
		` + statusFilter + `
		ORDER BY l.created_at DESC
	`

	rows, err := db.Query(query, project)
	if err != nil {
		return nil, fmt.Errorf("failed to query learnings: %w", err)
	}
	defer rows.Close()

	var learnings []model.Learning
	for rows.Next() {
		var l model.Learning
		var filesJSON string
		var taskID *string
		if err := rows.Scan(&l.ID, &l.Project, &l.CreatedAt, &l.UpdatedAt, &taskID,
			&l.Summary, &l.Detail, &filesJSON, &l.Status); err != nil {
			return nil, fmt.Errorf("failed to scan learning: %w", err)
		}
		l.TaskID = taskID

		// Parse files JSON
		if filesJSON != "" && filesJSON != "[]" {
			if err := json.Unmarshal([]byte(filesJSON), &l.Files); err != nil {
				return nil, fmt.Errorf("failed to unmarshal files: %w", err)
			}
		}

		// Get associated concepts
		conceptRows, err := db.Query(`
			SELECT c.name FROM learning_concepts lc
			JOIN concepts c ON c.id = lc.concept_id
			WHERE lc.learning_id = ?
		`, l.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get concepts: %w", err)
		}
		for conceptRows.Next() {
			var concept string
			if err := conceptRows.Scan(&concept); err != nil {
				conceptRows.Close()
				return nil, fmt.Errorf("failed to scan concept: %w", err)
			}
			l.Concepts = append(l.Concepts, concept)
		}
		conceptRows.Close()

		learnings = append(learnings, l)
	}

	return learnings, nil
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
	// Only include concepts that have at least one learning
	var related []model.Concept
	for _, c := range concepts {
		if c.LearningCount > 0 && strings.Contains(searchText, strings.ToLower(c.Name)) {
			related = append(related, c)
		}
	}

	return related, nil
}
