package model

import "time"

type ItemType string

const (
	ItemTypeTask ItemType = "task"
	ItemTypeEpic ItemType = "epic"
)

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
)

type Item struct {
	ID          string
	Project     string
	Type        ItemType
	Title       string
	Description string
	Status      Status
	Priority    int
	ParentID    *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Log struct {
	ID        int64
	ItemID    string
	Message   string
	CreatedAt time.Time
}

type Dep struct {
	ItemID    string
	DependsOn string
}
