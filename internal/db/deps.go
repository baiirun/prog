package db

import (
	"context"
	"fmt"
)

// AddDep records that itemID depends on dependsOnID. Idempotent.
func (db *DB) AddDep(ctx context.Context, itemID, dependsOnID string) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO deps (item_id, depends_on) VALUES (?, ?)`,
		itemID, dependsOnID)
	if err != nil {
		return fmt.Errorf("failed to add dependency %s -> %s: %w", itemID, dependsOnID, err)
	}
	return nil
}
