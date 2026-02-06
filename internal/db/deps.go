package db

import (
	"fmt"
)

// AddDep adds a dependency between items.
func (db *DB) AddDep(itemID, dependsOnID string) error {
	// Verify both items exist
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE id IN (?, ?)`, itemID, dependsOnID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to verify items: %w", err)
	}
	if count != 2 {
		return fmt.Errorf("one or both items not found: %s, %s (use 'tasks list' to see available items)", itemID, dependsOnID)
	}

	_, err = db.Exec(`
		INSERT OR IGNORE INTO deps (item_id, depends_on) VALUES (?, ?)`,
		itemID, dependsOnID)
	if err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}
	return nil
}

// GetDeps returns the IDs of items that the given item depends on.
func (db *DB) GetDeps(itemID string) ([]string, error) {
	rows, err := db.Query(`SELECT depends_on FROM deps WHERE item_id = ?`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var deps []string
	for rows.Next() {
		var depID string
		if err := rows.Scan(&depID); err != nil {
			return nil, fmt.Errorf("failed to scan dependency: %w", err)
		}
		deps = append(deps, depID)
	}
	return deps, rows.Err()
}

// HasUnmetDeps returns true if the item has dependencies that are not done.
func (db *DB) HasUnmetDeps(itemID string) (bool, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM deps d
		JOIN items i ON d.depends_on = i.id
		WHERE d.item_id = ? AND i.status NOT IN ('done', 'canceled')`, itemID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check dependencies: %w", err)
	}
	return count > 0, nil
}

// DepEdge represents a dependency relationship with item details.
type DepEdge struct {
	ItemID          string
	ItemTitle       string
	ItemStatus      string
	DependsOnID     string
	DependsOnTitle  string
	DependsOnStatus string
}

// GetAllDeps returns all dependency edges with item details, optionally filtered by project.
func (db *DB) GetAllDeps(project string) ([]DepEdge, error) {
	query := `
		SELECT
			d.item_id, i1.title, i1.status,
			d.depends_on, i2.title, i2.status
		FROM deps d
		JOIN items i1 ON d.item_id = i1.id
		JOIN items i2 ON d.depends_on = i2.id`
	args := []any{}

	if project != "" {
		query += ` WHERE i1.project = ?`
		args = append(args, project)
	}
	query += ` ORDER BY i1.priority, i1.id`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query deps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var edges []DepEdge
	for rows.Next() {
		var e DepEdge
		if err := rows.Scan(&e.ItemID, &e.ItemTitle, &e.ItemStatus,
			&e.DependsOnID, &e.DependsOnTitle, &e.DependsOnStatus); err != nil {
			return nil, fmt.Errorf("failed to scan dep edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}
