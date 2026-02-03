package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// CreateItem inserts a new item into the database.
// If the item has a project, it will be auto-created if it doesn't exist.
func (db *DB) CreateItem(item *model.Item) error {
	if !item.Type.IsValid() {
		return fmt.Errorf("invalid item type: %s", item.Type)
	}
	if !item.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", item.Status)
	}

	// Auto-create project if specified
	if item.Project != "" {
		if err := db.EnsureProject(item.Project); err != nil {
			return err
		}
	}

	_, err := db.Exec(`
		INSERT INTO items (id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Project, item.Type, item.Title, item.Description, item.DefinitionOfDone,
		item.Status, item.Priority, item.ParentID, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}
	return nil
}

// GetItem retrieves an item by ID.
func (db *DB) GetItem(id string) (*model.Item, error) {
	row := db.QueryRow(`
		SELECT id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at
		FROM items WHERE id = ?`, id)

	item := &model.Item{}
	var parentID, definitionOfDone sql.NullString
	err := row.Scan(
		&item.ID, &item.Project, &item.Type, &item.Title, &item.Description, &definitionOfDone,
		&item.Status, &item.Priority, &parentID, &item.CreatedAt, &item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get item: %w", err)
	}

	if parentID.Valid {
		item.ParentID = &parentID.String
	}
	if definitionOfDone.Valid {
		item.DefinitionOfDone = &definitionOfDone.String
	}
	return item, nil
}

// UpdateStatus changes an item's status.
func (db *DB) UpdateStatus(id string, status model.Status) error {
	if !status.IsValid() {
		return fmt.Errorf("invalid status: %s", status)
	}

	result, err := db.Exec(`
		UPDATE items SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", id)
	}
	return nil
}

// AppendDescription appends text to an item's description.
func (db *DB) AppendDescription(id string, text string) error {
	result, err := db.Exec(`
		UPDATE items
		SET description = COALESCE(description, '') || ? || char(10) || ?,
		    updated_at = ?
		WHERE id = ?`,
		"\n", text, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to append description: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", id)
	}
	return nil
}

// SetParent sets an item's parent to an epic.
func (db *DB) SetParent(itemID, parentID string) error {
	// Verify parent exists and is an epic
	var itemType string
	err := db.QueryRow(`SELECT type FROM items WHERE id = ?`, parentID).Scan(&itemType)
	if err != nil {
		return fmt.Errorf("parent not found: %s (use 'tasks list' to see available items)", parentID)
	}
	if itemType != string(model.ItemTypeEpic) {
		return fmt.Errorf("parent must be an epic, got %s", itemType)
	}

	// Update the item's parent
	result, err := db.Exec(`
		UPDATE items SET parent_id = ?, updated_at = ? WHERE id = ?`,
		parentID, time.Now(), itemID)
	if err != nil {
		return fmt.Errorf("failed to set parent: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", itemID)
	}
	return nil
}

// SetProject changes an item's project.
func (db *DB) SetProject(id string, project string) error {
	// Auto-create project if specified
	if project != "" {
		if err := db.EnsureProject(project); err != nil {
			return err
		}
	}

	result, err := db.Exec(`
		UPDATE items SET project = ?, updated_at = ? WHERE id = ?`,
		project, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set project: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'prog list' to see available items)", id)
	}
	return nil
}

// SetDescription replaces an item's description entirely.
func (db *DB) SetDescription(id string, text string) error {
	result, err := db.Exec(`
		UPDATE items
		SET description = ?,
		    updated_at = ?
		WHERE id = ?`,
		text, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set description: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", id)
	}
	return nil
}

// SetTitle replaces an item's title.
func (db *DB) SetTitle(id string, title string) error {
	result, err := db.Exec(`
		UPDATE items
		SET title = ?,
		    updated_at = ?
		WHERE id = ?`,
		title, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set title: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'prog list' to see available items)", id)
	}
	return nil
}

// SetDefinitionOfDone sets or clears an item's definition of done.
// Pass nil to clear the DoD.
func (db *DB) SetDefinitionOfDone(id string, dod *string) error {
	result, err := db.Exec(`
		UPDATE items
		SET definition_of_done = ?,
		    updated_at = ?
		WHERE id = ?`,
		dod, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set definition of done: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("item not found: %s (use 'prog list' to see available items)", id)
	}
	return nil
}

// DeleteItem removes an item and its associated logs and dependencies.
func (db *DB) DeleteItem(id string) error {
	// Check if item exists first
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE id = ?`, id).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check item: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("item not found: %s (use 'tasks list' to see available items)", id)
	}

	// Delete logs
	_, err = db.Exec(`DELETE FROM logs WHERE item_id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete logs: %w", err)
	}

	// Delete dependencies (both directions)
	_, err = db.Exec(`DELETE FROM deps WHERE item_id = ? OR depends_on = ?`, id, id)
	if err != nil {
		return fmt.Errorf("failed to delete dependencies: %w", err)
	}

	// Delete the item
	_, err = db.Exec(`DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	return nil
}
