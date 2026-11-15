package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/baiirun/prog/internal/model"
)

// LoadGraph implements tasks.Store.LoadGraph.
func (db *DB) LoadGraph(ctx context.Context) ([]model.Item, []model.Dep, error) {
	items, err := db.loadAllItems(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := db.attachLabels(ctx, items); err != nil {
		return nil, nil, err
	}
	deps, err := db.loadAllDeps(ctx)
	if err != nil {
		return nil, nil, err
	}
	return items, deps, nil
}

func (db *DB) loadAllItems(ctx context.Context) ([]model.Item, error) {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at
		FROM items ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		var description, parentID, definitionOfDone sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Project, &item.Type, &item.Title, &description, &definitionOfDone,
			&item.Status, &item.Priority, &parentID, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		item.Description = description.String
		if parentID.Valid {
			item.ParentID = &parentID.String
		}
		if definitionOfDone.Valid {
			item.DefinitionOfDone = &definitionOfDone.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load items: %w", err)
	}
	return items, nil
}

func (db *DB) attachLabels(ctx context.Context, items []model.Item) error {
	rows, err := db.conn.QueryContext(ctx, `
		SELECT il.item_id, l.name
		FROM item_labels il JOIN labels l ON il.label_id = l.id
		ORDER BY il.item_id, l.name`)
	if err != nil {
		return fmt.Errorf("failed to load item labels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	labels := make(map[string][]string)
	for rows.Next() {
		var itemID, name string
		if err := rows.Scan(&itemID, &name); err != nil {
			return fmt.Errorf("failed to scan item label: %w", err)
		}
		labels[itemID] = append(labels[itemID], name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to load item labels: %w", err)
	}
	for i := range items {
		items[i].Labels = labels[items[i].ID]
	}
	return nil
}

func (db *DB) loadAllDeps(ctx context.Context) ([]model.Dep, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT item_id, depends_on FROM deps ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("failed to load deps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var deps []model.Dep
	for rows.Next() {
		var d model.Dep
		if err := rows.Scan(&d.ItemID, &d.DependsOn); err != nil {
			return nil, fmt.Errorf("failed to scan dep: %w", err)
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to load deps: %w", err)
	}
	return deps, nil
}
