package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// CreateLabel fails if the name already exists in the project.
func (db *DB) CreateLabel(ctx context.Context, in model.NewLabel) error {
	_, err := db.insertLabel(ctx, in)
	return err
}

func (db *DB) insertLabel(ctx context.Context, in model.NewLabel) (string, error) {
	id := model.GenerateLabelID()
	now := time.Now()
	_, err := db.ExecContext(ctx, `
		INSERT INTO labels (id, name, project, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, in.Name, in.Project, in.Color, now, now)
	if err != nil {
		return "", fmt.Errorf("failed to create label %q: %w", in.Name, err)
	}
	return id, nil
}

// findLabelID returns the ID of a label by project and name, and whether it exists.
func (db *DB) findLabelID(ctx context.Context, project, name string) (string, bool, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM labels WHERE name = ? AND project = ?`, name, project).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to look up label %q: %w", name, err)
	}
	return id, true, nil
}

// ListLabels returns a project's labels sorted by name.
func (db *DB) ListLabels(ctx context.Context, project string) ([]model.Label, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, project, color, created_at, updated_at
		FROM labels WHERE project = ? ORDER BY name`, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var labels []model.Label
	for rows.Next() {
		var l model.Label
		var color sql.NullString
		if err := rows.Scan(&l.ID, &l.Name, &l.Project, &color, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan label: %w", err)
		}
		l.Color = color.String
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// RenameLabel changes a label's name.
func (db *DB) RenameLabel(ctx context.Context, project, oldName, newName string) error {
	result, err := db.ExecContext(ctx, `
		UPDATE labels SET name = ?, updated_at = ?
		WHERE name = ? AND project = ?`,
		newName, time.Now(), oldName, project)
	if err != nil {
		return fmt.Errorf("failed to rename label: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to rename label: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("label not found: %s", oldName)
	}
	return nil
}

// DeleteLabel removes the label and detaches it from every item.
func (db *DB) DeleteLabel(ctx context.Context, project, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var labelID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM labels WHERE name = ? AND project = ?`, name, project).Scan(&labelID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("label not found: %s", name)
	}
	if err != nil {
		return fmt.Errorf("failed to look up label %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM item_labels WHERE label_id = ?`, labelID); err != nil {
		return fmt.Errorf("failed to delete label associations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM labels WHERE id = ?`, labelID); err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// AddLabelToItem creates the label in project if missing. Idempotent.
func (db *DB) AddLabelToItem(ctx context.Context, itemID, project, name string) error {
	labelID, found, err := db.findLabelID(ctx, project, name)
	if err != nil {
		return err
	}
	if !found {
		if labelID, err = db.insertLabel(ctx, model.NewLabel{Project: project, Name: name}); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx,
		`INSERT OR IGNORE INTO item_labels (item_id, label_id) VALUES (?, ?)`,
		itemID, labelID)
	if err != nil {
		return fmt.Errorf("failed to add label %q to %s: %w", name, itemID, err)
	}
	return nil
}

// RemoveLabelFromItem fails if the item does not carry the label.
func (db *DB) RemoveLabelFromItem(ctx context.Context, itemID, project, name string) error {
	labelID, found, err := db.findLabelID(ctx, project, name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("label not found: %s", name)
	}
	result, err := db.ExecContext(ctx,
		`DELETE FROM item_labels WHERE item_id = ? AND label_id = ?`, itemID, labelID)
	if err != nil {
		return fmt.Errorf("failed to remove label from item: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to remove label from item: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("item does not have label: %s", name)
	}
	return nil
}
