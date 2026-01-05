package db

import (
	"database/sql"
	"fmt"

	"github.com/baiirun/dotworld-tasks/internal/model"
)

// ListItems returns items filtered by project and/or status.
func (db *DB) ListItems(project string, status *model.Status) ([]model.Item, error) {
	query := `SELECT id, project, type, title, description, status, priority, parent_id, created_at, updated_at FROM items WHERE 1=1`
	args := []any{}

	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}
	if status != nil {
		if !status.IsValid() {
			return nil, fmt.Errorf("invalid status: %s", *status)
		}
		query += ` AND status = ?`
		args = append(args, *status)
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	return db.queryItems(query, args...)
}

// ReadyItems returns items that are open and have no unmet dependencies.
func (db *DB) ReadyItems(project string) ([]model.Item, error) {
	query := `
		SELECT id, project, type, title, description, status, priority, parent_id, created_at, updated_at
		FROM items
		WHERE status = 'open'
		  AND id NOT IN (
		    SELECT d.item_id FROM deps d
		    JOIN items i ON d.depends_on = i.id
		    WHERE i.status != 'done'
		  )`
	args := []any{}

	if project != "" {
		query += ` AND project = ?`
		args = append(args, project)
	}
	query += ` ORDER BY priority ASC, created_at ASC`

	return db.queryItems(query, args...)
}

// StatusReport contains aggregated project status.
type StatusReport struct {
	Project      string
	Open         int
	InProgress   int
	Blocked      int
	Done         int
	Canceled     int
	Ready        int
	RecentDone   []model.Item // last 3 completed
	InProgItems  []model.Item // current in-progress
	BlockedItems []model.Item // blocked with reasons
	ReadyItems   []model.Item // ready for work
}

// ProjectStatus returns an aggregated status report for a project.
func (db *DB) ProjectStatus(project string) (*StatusReport, error) {
	report := &StatusReport{Project: project}

	// Count by status
	query := `SELECT status, COUNT(*) FROM items`
	args := []any{}
	if project != "" {
		query += ` WHERE project = ?`
		args = append(args, project)
	}
	query += ` GROUP BY status`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to count statuses: %w", err)
	}
	defer rows.Close()

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
		case model.StatusDone:
			report.Done = count
		case model.StatusCanceled:
			report.Canceled = count
		}
	}

	// Get ready count and items
	readyItems, err := db.ReadyItems(project)
	if err != nil {
		return nil, err
	}
	report.Ready = len(readyItems)
	report.ReadyItems = readyItems

	// Get in-progress items
	inProgStatus := model.StatusInProgress
	report.InProgItems, err = db.ListItems(project, &inProgStatus)
	if err != nil {
		return nil, err
	}

	// Get blocked items
	blockedStatus := model.StatusBlocked
	report.BlockedItems, err = db.ListItems(project, &blockedStatus)
	if err != nil {
		return nil, err
	}

	// Get recent done (last 3)
	recentQuery := `
		SELECT id, project, type, title, description, status, priority, parent_id, created_at, updated_at
		FROM items WHERE status = 'done'`
	recentArgs := []any{}
	if project != "" {
		recentQuery += ` AND project = ?`
		recentArgs = append(recentArgs, project)
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
	defer rows.Close()

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
	defer rows.Close()

	var items []model.Item
	for rows.Next() {
		var item model.Item
		var parentID sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Project, &item.Type, &item.Title, &item.Description,
			&item.Status, &item.Priority, &parentID, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}
		if parentID.Valid {
			item.ParentID = &parentID.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
