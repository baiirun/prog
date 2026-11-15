package db

import (
	"context"
	"fmt"
	"time"
)

// EnsureProject creates the project if missing. Idempotent.
func (db *DB) EnsureProject(ctx context.Context, name string) error {
	now := time.Now()
	_, err := db.ExecContext(ctx, `
		INSERT INTO projects (name, created_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(name) DO NOTHING`,
		name, now, now,
	)
	if err != nil {
		return fmt.Errorf("failed to ensure project %q: %w", name, err)
	}
	return nil
}

// ListProjects returns project names sorted by name.
func (db *DB) ListProjects(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var projects []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		projects = append(projects, name)
	}
	return projects, rows.Err()
}
