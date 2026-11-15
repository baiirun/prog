package db

import (
	"context"
	"fmt"

	"github.com/baiirun/prog/internal/model"
)

// AddLog adds a log entry to an item.
func (db *DB) AddLog(ctx context.Context, itemID, message string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO logs (item_id, message) VALUES (?, ?)`, itemID, message)
	if err != nil {
		return fmt.Errorf("failed to add log to %s: %w", itemID, err)
	}
	return nil
}

// Logs returns an item's logs, oldest first.
func (db *DB) Logs(ctx context.Context, itemID string) ([]model.Log, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, item_id, message, created_at
		FROM logs WHERE item_id = ? ORDER BY created_at ASC, id ASC`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for %s: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	var logs []model.Log
	for rows.Next() {
		var log model.Log
		if err := rows.Scan(&log.ID, &log.ItemID, &log.Message, &log.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}
