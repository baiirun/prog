package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

// CreateItem stores a new item and returns it with its assigned ID.
// Requires: the item's project exists (see EnsureProject).
func (db *DB) CreateItem(ctx context.Context, in model.NewItem) (model.Item, error) {
	now := time.Now()
	item := model.Item{
		ID:               model.GenerateID(in.Type),
		Project:          in.Project,
		Type:             in.Type,
		Title:            in.Title,
		Description:      in.Description,
		DefinitionOfDone: in.DefinitionOfDone,
		Status:           in.Status,
		Priority:         in.Priority,
		ParentID:         in.ParentID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO items (id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.Project, item.Type, item.Title, item.Description, item.DefinitionOfDone,
		item.Status, item.Priority, item.ParentID, item.CreatedAt, item.UpdatedAt,
	)
	if err != nil {
		return model.Item{}, fmt.Errorf("failed to create item: %w", err)
	}
	return item, nil
}

// SetStatus changes an item's stored status.
func (db *DB) SetStatus(ctx context.Context, id string, status model.Status) error {
	return db.updateItem(ctx, id, columnStatus, status)
}

// SetTitle replaces an item's title.
func (db *DB) SetTitle(ctx context.Context, id, title string) error {
	return db.updateItem(ctx, id, columnTitle, title)
}

// SetDescription replaces an item's description.
func (db *DB) SetDescription(ctx context.Context, id, text string) error {
	return db.updateItem(ctx, id, columnDescription, text)
}

// SetDefinitionOfDone sets the DoD; nil clears it.
func (db *DB) SetDefinitionOfDone(ctx context.Context, id string, dod *string) error {
	return db.updateItem(ctx, id, columnDefinitionOfDone, dod)
}

// SetParent sets an item's parent.
func (db *DB) SetParent(ctx context.Context, id, parentID string) error {
	return db.updateItem(ctx, id, columnParentID, parentID)
}

// SetProject moves an item. Requires: the project exists (see EnsureProject).
func (db *DB) SetProject(ctx context.Context, id, project string) error {
	return db.updateItem(ctx, id, columnProject, project)
}

// itemColumn is a settable items column. It is spliced into SQL, so only the
// constants below may exist; a runtime string needs an explicit conversion.
type itemColumn string

const (
	columnStatus           itemColumn = "status"
	columnTitle            itemColumn = "title"
	columnDescription      itemColumn = "description"
	columnDefinitionOfDone itemColumn = "definition_of_done"
	columnParentID         itemColumn = "parent_id"
	columnProject          itemColumn = "project"
)

// updateItem sets one column and bumps updated_at.
func (db *DB) updateItem(ctx context.Context, id string, column itemColumn, value any) error {
	result, err := db.conn.ExecContext(ctx,
		`UPDATE items SET `+string(column)+` = ?, updated_at = ? WHERE id = ?`,
		value, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set %s for %s: %w", column, id, err)
	}
	return requireRow(result, id)
}

// AppendDescription appends "\n\n"+text in a single UPDATE, so concurrent
// appends to the same item do not overwrite each other.
func (db *DB) AppendDescription(ctx context.Context, id, text string) error {
	result, err := db.conn.ExecContext(ctx, `
		UPDATE items
		SET description = COALESCE(description, '') || char(10) || char(10) || ?,
		    updated_at = ?
		WHERE id = ?`,
		text, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to append description: %w", err)
	}
	return requireRow(result, id)
}

// DeleteItem removes an item with its logs, labels and dependency edges in
// one transaction. It refuses an epic that still has children, because their
// parent_id would dangle.
func (db *DB) DeleteItem(ctx context.Context, id string) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete of %s: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	var children int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE parent_id = ?`, id).Scan(&children); err != nil {
		return fmt.Errorf("failed to check children of %s: %w", id, err)
	}
	if children > 0 {
		return fmt.Errorf("cannot delete %s: it has %d child item(s); delete or reparent them first", id, children)
	}

	for _, stmt := range []string{
		`DELETE FROM logs WHERE item_id = ?`,
		`DELETE FROM item_labels WHERE item_id = ?`,
		`DELETE FROM deps WHERE item_id = ?1 OR depends_on = ?1`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
			return fmt.Errorf("failed to delete %s: %w", id, err)
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete %s: %w", id, err)
	}
	if err := requireRow(result, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete of %s: %w", id, err)
	}
	return nil
}

func requireRow(result sql.Result, id string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check update of %s: %w", id, err)
	}
	if rows == 0 {
		return &tasks.NotFoundError{Record: tasks.RecordItem, ID: id}
	}
	return nil
}
