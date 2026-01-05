// Package model defines the core data types for the tasks system.
package model

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// GenerateID returns a new ID with a type-specific prefix and 6 hex chars.
//
// Prefixes by item type:
//   - task: "ts-" (e.g., ts-a1b2c3)
//   - epic: "ep-" (e.g., ep-a1b2c3)
func GenerateID(itemType ItemType) string {
	prefix := "ts-"
	if itemType == ItemTypeEpic {
		prefix = "ep-"
	}
	b := make([]byte, 3)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

type ItemType string

const (
	ItemTypeTask ItemType = "task"
	ItemTypeEpic ItemType = "epic"
)

func (t ItemType) IsValid() bool {
	return t == ItemTypeTask || t == ItemTypeEpic
}

type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
	StatusCanceled   Status = "canceled"
)

func (s Status) IsValid() bool {
	return s == StatusOpen || s == StatusInProgress || s == StatusBlocked || s == StatusDone || s == StatusCanceled
}

// Item represents a task or epic in the system.
type Item struct {
	ID          string   // Unique identifier (ts-XXXXXX or ep-XXXXXX)
	Project     string   // Project scope (e.g., "gaia", "myapp")
	Type        ItemType // "task" or "epic"
	Title       string   // Short description
	Description string   // Full context, notes, handoff info
	Status      Status   // Current state
	Priority    int      // 1=high, 2=medium, 3=low
	ParentID    *string  // Optional parent epic ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Log is a timestamped audit trail entry for an item.
type Log struct {
	ID        int64
	ItemID    string
	Message   string
	CreatedAt time.Time
}

// Dep represents a dependency relationship where ItemID depends on DependsOn.
// ItemID is blocked until DependsOn has status "done".
type Dep struct {
	ItemID    string
	DependsOn string
}

// Project represents a named project that groups related items.
type Project struct {
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
