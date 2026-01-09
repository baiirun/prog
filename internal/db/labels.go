package db

import (
	"fmt"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// CreateLabel inserts a new label.
func (db *DB) CreateLabel(l *model.Label) error {
	_, err := db.Exec(`
		INSERT INTO labels (id, name, project, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, l.ID, l.Name, l.Project, l.Color, l.CreatedAt, l.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create label: %w", err)
	}
	return nil
}

// GetLabelByName retrieves a label by name and project.
// This is the primary lookup method for labels.
func (db *DB) GetLabelByName(project, name string) (*model.Label, error) {
	var l model.Label
	var color *string
	err := db.QueryRow(`
		SELECT id, name, project, color, created_at, updated_at
		FROM labels WHERE name = ? AND project = ?
	`, name, project).Scan(&l.ID, &l.Name, &l.Project, &color, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("label not found: %s", name)
	}
	if color != nil {
		l.Color = *color
	}
	return &l, nil
}

// GetLabel retrieves a label by ID (internal use).
func (db *DB) GetLabel(id string) (*model.Label, error) {
	var l model.Label
	var color *string
	err := db.QueryRow(`
		SELECT id, name, project, color, created_at, updated_at
		FROM labels WHERE id = ?
	`, id).Scan(&l.ID, &l.Name, &l.Project, &color, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("label not found: %s", id)
	}
	if color != nil {
		l.Color = *color
	}
	return &l, nil
}

// ListLabels returns all labels for a project, sorted by name.
func (db *DB) ListLabels(project string) ([]model.Label, error) {
	rows, err := db.Query(`
		SELECT id, name, project, color, created_at, updated_at
		FROM labels
		WHERE project = ?
		ORDER BY name
	`, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}
	defer rows.Close()

	var labels []model.Label
	for rows.Next() {
		var l model.Label
		var color *string
		if err := rows.Scan(&l.ID, &l.Name, &l.Project, &color, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan label: %w", err)
		}
		if color != nil {
			l.Color = *color
		}
		labels = append(labels, l)
	}

	return labels, nil
}

// RenameLabel changes a label's name.
func (db *DB) RenameLabel(project, oldName, newName string) error {
	result, err := db.Exec(`
		UPDATE labels SET name = ?, updated_at = ?
		WHERE name = ? AND project = ?
	`, newName, time.Now(), oldName, project)
	if err != nil {
		return fmt.Errorf("failed to rename label: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label not found: %s", oldName)
	}
	return nil
}

// DeleteLabel removes a label and all its item associations.
func (db *DB) DeleteLabel(project, name string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get label ID
	var labelID string
	err = tx.QueryRow(`SELECT id FROM labels WHERE name = ? AND project = ?`, name, project).Scan(&labelID)
	if err != nil {
		return fmt.Errorf("label not found: %s", name)
	}

	// Delete item associations
	_, err = tx.Exec(`DELETE FROM item_labels WHERE label_id = ?`, labelID)
	if err != nil {
		return fmt.Errorf("failed to delete label associations: %w", err)
	}

	// Delete label
	_, err = tx.Exec(`DELETE FROM labels WHERE id = ?`, labelID)
	if err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// EnsureLabel creates a label if it doesn't exist, returns the label.
func (db *DB) EnsureLabel(project, name string) (*model.Label, error) {
	// Try to get existing label
	label, err := db.GetLabelByName(project, name)
	if err == nil {
		return label, nil
	}

	// Create new label
	now := time.Now()
	label = &model.Label{
		ID:        model.GenerateLabelID(),
		Name:      name,
		Project:   project,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.CreateLabel(label); err != nil {
		return nil, err
	}
	return label, nil
}

// AddLabelToItem attaches a label to an item.
// Creates the label if it doesn't exist.
func (db *DB) AddLabelToItem(itemID, project, labelName string) error {
	// Ensure label exists
	label, err := db.EnsureLabel(project, labelName)
	if err != nil {
		return fmt.Errorf("failed to ensure label: %w", err)
	}

	// Add association (ignore if already exists)
	_, err = db.Exec(`
		INSERT OR IGNORE INTO item_labels (item_id, label_id)
		VALUES (?, ?)
	`, itemID, label.ID)
	if err != nil {
		return fmt.Errorf("failed to add label to item: %w", err)
	}
	return nil
}

// RemoveLabelFromItem detaches a label from an item.
func (db *DB) RemoveLabelFromItem(itemID, project, labelName string) error {
	// Get label ID
	label, err := db.GetLabelByName(project, labelName)
	if err != nil {
		return err
	}

	result, err := db.Exec(`
		DELETE FROM item_labels WHERE item_id = ? AND label_id = ?
	`, itemID, label.ID)
	if err != nil {
		return fmt.Errorf("failed to remove label from item: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item does not have label: %s", labelName)
	}
	return nil
}

// GetItemLabels returns all labels attached to an item.
func (db *DB) GetItemLabels(itemID string) ([]model.Label, error) {
	rows, err := db.Query(`
		SELECT l.id, l.name, l.project, l.color, l.created_at, l.updated_at
		FROM labels l
		JOIN item_labels il ON il.label_id = l.id
		WHERE il.item_id = ?
		ORDER BY l.name
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get item labels: %w", err)
	}
	defer rows.Close()

	var labels []model.Label
	for rows.Next() {
		var l model.Label
		var color *string
		if err := rows.Scan(&l.ID, &l.Name, &l.Project, &color, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan label: %w", err)
		}
		if color != nil {
			l.Color = *color
		}
		labels = append(labels, l)
	}

	return labels, nil
}

// PopulateItemLabels fetches and attaches labels to a slice of items.
// This is an efficient batch operation that avoids N+1 queries.
func (db *DB) PopulateItemLabels(items []model.Item) error {
	if len(items) == 0 {
		return nil
	}

	// Build item ID list
	ids := make([]any, len(items))
	placeholders := ""
	for i, item := range items {
		ids[i] = item.ID
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
	}

	// Query all labels for these items in one go
	query := fmt.Sprintf(`
		SELECT il.item_id, l.name
		FROM item_labels il
		JOIN labels l ON il.label_id = l.id
		WHERE il.item_id IN (%s)
		ORDER BY il.item_id, l.name
	`, placeholders)

	rows, err := db.Query(query, ids...)
	if err != nil {
		return fmt.Errorf("failed to query item labels: %w", err)
	}
	defer rows.Close()

	// Build a map of item ID -> label names
	labelMap := make(map[string][]string)
	for rows.Next() {
		var itemID, labelName string
		if err := rows.Scan(&itemID, &labelName); err != nil {
			return fmt.Errorf("failed to scan label: %w", err)
		}
		labelMap[itemID] = append(labelMap[itemID], labelName)
	}

	// Attach labels to items
	for i := range items {
		items[i].Labels = labelMap[items[i].ID]
	}

	return nil
}

// SetLabelColor updates a label's color.
func (db *DB) SetLabelColor(project, name, color string) error {
	result, err := db.Exec(`
		UPDATE labels SET color = ?, updated_at = ?
		WHERE name = ? AND project = ?
	`, color, time.Now(), name, project)
	if err != nil {
		return fmt.Errorf("failed to update label color: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("label not found: %s", name)
	}
	return nil
}
