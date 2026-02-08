package db

import (
	"database/sql"
	"fmt"

	"github.com/baiirun/prog/internal/model"
)

// ListFilter contains optional filters for listing items.
type ListFilter struct {
	Project     string        // Filter by project
	Status      *model.Status // Filter by status
	Parent      string        // Filter by parent epic ID
	Type        string        // Filter by item type (task, epic)
	Blocking    string        // Show items that block this ID
	BlockedBy   string        // Show items blocked by this ID
	HasBlockers bool          // Show only items with unresolved blockers
	NoBlockers  bool          // Show only items with no blockers
	Labels      []string      // Filter by label names (AND - items must have all)
}

// ListItems returns items filtered by project and/or status.
func (db *DB) ListItems(project string, status *model.Status) ([]model.Item, error) {
	return db.ListItemsFiltered(ListFilter{Project: project, Status: status})
}

// ListItemsFiltered returns items matching the given filters.
func (db *DB) ListItemsFiltered(filter ListFilter) ([]model.Item, error) {
	query := `SELECT id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at FROM items WHERE 1=1`
	args := []any{}

	if filter.Project != "" {
		query += ` AND project = ?`
		args = append(args, filter.Project)
	}
	if filter.Status != nil {
		if !filter.Status.IsValid() {
			return nil, fmt.Errorf("invalid status: %s", *filter.Status)
		}
		query += ` AND status = ?`
		args = append(args, *filter.Status)
	}
	if filter.Parent != "" {
		query += ` AND parent_id = ?`
		args = append(args, filter.Parent)
	}
	if filter.Type != "" {
		itemType := model.ItemType(filter.Type)
		if !itemType.IsValid() {
			return nil, fmt.Errorf("invalid type: %s (valid: task, epic)", filter.Type)
		}
		query += ` AND type = ?`
		args = append(args, filter.Type)
	}
	if filter.Blocking != "" {
		// Items that block the given ID (i.e., items the given ID depends on)
		query += ` AND id IN (SELECT depends_on FROM deps WHERE item_id = ?)`
		args = append(args, filter.Blocking)
	}
	if filter.BlockedBy != "" {
		// Items blocked by the given ID (i.e., items that depend on the given ID)
		query += ` AND id IN (SELECT item_id FROM deps WHERE depends_on = ?)`
		args = append(args, filter.BlockedBy)
	}
	if filter.HasBlockers {
		// Items with unresolved blockers (dependencies that aren't done)
		query += ` AND id IN (SELECT d.item_id FROM deps d JOIN items i ON d.depends_on = i.id WHERE i.status NOT IN ('done', 'canceled'))`
	}
	if filter.NoBlockers {
		// Items with no blockers (either no deps, or all deps are done)
		query += ` AND id NOT IN (SELECT d.item_id FROM deps d JOIN items i ON d.depends_on = i.id WHERE i.status NOT IN ('done', 'canceled'))`
	}
	if len(filter.Labels) > 0 {
		// Items must have ALL specified labels (AND semantics)
		// Build placeholder list for IN clause
		placeholders := ""
		for i := range filter.Labels {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
		}
		query += fmt.Sprintf(` AND id IN (
			SELECT il.item_id FROM item_labels il
			JOIN labels l ON il.label_id = l.id
			WHERE l.name IN (%s)
			GROUP BY il.item_id
			HAVING COUNT(DISTINCT l.name) = ?
		)`, placeholders)
		for _, label := range filter.Labels {
			args = append(args, label)
		}
		args = append(args, len(filter.Labels))
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	return db.queryItems(query, args...)
}

// ReadyItems returns items that are open and have no unmet dependencies.
func (db *DB) ReadyItems(project string) ([]model.Item, error) {
	return db.ReadyItemsFiltered(project, nil)
}

// ReadyItemsFiltered returns ready items with optional label filtering.
func (db *DB) ReadyItemsFiltered(project string, labels []string) ([]model.Item, error) {
	query := `
		SELECT id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at
		FROM items
		WHERE status = 'open'
		  AND id NOT IN (
		    SELECT d.item_id FROM deps d
		    JOIN items i ON d.depends_on = i.id
		    WHERE i.status NOT IN ('done', 'canceled')
		  )`
	args := []any{}

	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}
	if len(labels) > 0 {
		// Items must have ALL specified labels (AND semantics)
		placeholders := ""
		for i := range labels {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
		}
		query += fmt.Sprintf(` AND id IN (
			SELECT il.item_id FROM item_labels il
			JOIN labels l ON il.label_id = l.id
			WHERE l.name IN (%s)
			GROUP BY il.item_id
			HAVING COUNT(DISTINCT l.name) = ?
		)`, placeholders)
		for _, label := range labels {
			args = append(args, label)
		}
		args = append(args, len(labels))
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	return db.queryItems(query, args...)
}

// StatusReport contains aggregated project status.
type StatusReport struct {
	Project        string
	Open           int
	InProgress     int
	Blocked        int
	Reviewing      int
	Done           int
	Canceled       int
	Ready          int
	RecentDone     []model.Item // last 3 completed
	InProgItems    []model.Item // current in-progress
	ReviewingItems []model.Item // awaiting merge
	BlockedItems   []model.Item // blocked with reasons
	ReadyItems     []model.Item // ready for work
}

// ProjectStatus returns an aggregated status report for a project.
func (db *DB) ProjectStatus(project string) (*StatusReport, error) {
	return db.ProjectStatusFiltered(project, nil)
}

// ProjectStatusFiltered returns an aggregated status report with optional label filtering.
func (db *DB) ProjectStatusFiltered(project string, labels []string) (*StatusReport, error) {
	report := &StatusReport{Project: project}

	// Build label subquery for reuse
	labelSubquery := ""
	labelArgs := []any{}
	if len(labels) > 0 {
		placeholders := ""
		for i := range labels {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
		}
		labelSubquery = fmt.Sprintf(` AND id IN (
			SELECT il.item_id FROM item_labels il
			JOIN labels l ON il.label_id = l.id
			WHERE l.name IN (%s)
			GROUP BY il.item_id
			HAVING COUNT(DISTINCT l.name) = ?
		)`, placeholders)
		for _, label := range labels {
			labelArgs = append(labelArgs, label)
		}
		labelArgs = append(labelArgs, len(labels))
	}

	// Count by status
	query := `SELECT status, COUNT(*) FROM items WHERE 1=1`
	args := []any{}
	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}
	if labelSubquery != "" {
		query += labelSubquery
		args = append(args, labelArgs...)
	}
	query += ` GROUP BY status`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to count statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan status count: %w", err)
		}
		switch model.Status(status) {
		case model.StatusOpen:
			report.Open = count
		case model.StatusInProgress:
			report.InProgress = count
		case model.StatusBlocked:
			report.Blocked = count
		case model.StatusReviewing:
			report.Reviewing = count
		case model.StatusDone:
			report.Done = count
		case model.StatusCanceled:
			report.Canceled = count
		}
	}

	// Get ready count and items
	readyItems, err := db.ReadyItemsFiltered(project, labels)
	if err != nil {
		return nil, err
	}
	report.Ready = len(readyItems)
	report.ReadyItems = readyItems

	// Get in-progress items
	inProgStatus := model.StatusInProgress
	report.InProgItems, err = db.ListItemsFiltered(ListFilter{Project: project, Status: &inProgStatus, Labels: labels})
	if err != nil {
		return nil, err
	}

	// Get reviewing items
	reviewingStatus := model.StatusReviewing
	report.ReviewingItems, err = db.ListItemsFiltered(ListFilter{Project: project, Status: &reviewingStatus, Labels: labels})
	if err != nil {
		return nil, err
	}

	// Get blocked items
	blockedStatus := model.StatusBlocked
	report.BlockedItems, err = db.ListItemsFiltered(ListFilter{Project: project, Status: &blockedStatus, Labels: labels})
	if err != nil {
		return nil, err
	}

	// Get recent done (last 3)
	recentQuery := `
		SELECT id, project, type, title, description, definition_of_done, status, priority, parent_id, created_at, updated_at
		FROM items WHERE status = 'done'`
	recentArgs := []any{}
	if project != "" {
		recentQuery += ` AND project = ?`
		recentArgs = append(recentArgs, project)
	}
	if labelSubquery != "" {
		recentQuery += labelSubquery
		recentArgs = append(recentArgs, labelArgs...)
	}
	recentQuery += ` ORDER BY updated_at DESC LIMIT 3`
	report.RecentDone, err = db.queryItems(recentQuery, recentArgs...)
	if err != nil {
		return nil, err
	}

	return report, nil
}

// ListProjects returns all project names from the projects table.
func (db *DB) ListProjects() ([]string, error) {
	rows, err := db.Query(`SELECT name FROM projects ORDER BY name`)
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

// queryItems is a helper to scan item rows.
func (db *DB) queryItems(query string, args ...any) ([]model.Item, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		var parentID, definitionOfDone sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Project, &item.Type, &item.Title, &item.Description, &definitionOfDone,
			&item.Status, &item.Priority, &parentID, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		if parentID.Valid {
			item.ParentID = &parentID.String
		}
		if definitionOfDone.Valid {
			item.DefinitionOfDone = &definitionOfDone.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
