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
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
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
	StatusBlocked    Status = "blocked" // Derived state only — set by dep resolution and epic derivation, not by CLI
	StatusReviewing  Status = "reviewing"
	StatusDone       Status = "done"
	StatusCanceled   Status = "canceled"
)

func (s Status) IsValid() bool {
	return s == StatusOpen || s == StatusInProgress || s == StatusBlocked || s == StatusReviewing || s == StatusDone || s == StatusCanceled
}

// Item represents a task or epic in the system.
type Item struct {
	ID               string   // Unique identifier (ts-XXXXXX or ep-XXXXXX)
	Project          string   // Project scope (e.g., "gaia", "myapp")
	Type             ItemType // "task" or "epic"
	Title            string   // Short description
	Description      string   // Full context, notes, handoff info
	DefinitionOfDone *string  // Completion criteria for agents (natural language)
	Status           Status   // Current state
	Priority         int      // 1=high, 2=medium, 3=low
	ParentID         *string  // Optional parent epic ID
	Labels           []string // Attached label names (populated separately)
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Log is a timestamped audit trail entry for an item.
type Log struct {
	ID        int64
	ItemID    string
	Message   string
	CreatedAt time.Time
}

// Dep represents a dependency relationship where ItemID depends on DependsOn.
// ItemID is blocked until DependsOn reaches a terminal status ("done" or "canceled").
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

// LearningStatus represents the lifecycle state of a learning.
type LearningStatus string

const (
	LearningStatusActive   LearningStatus = "active"
	LearningStatusStale    LearningStatus = "stale"
	LearningStatusArchived LearningStatus = "archived"
)

func (s LearningStatus) IsValid() bool {
	return s == LearningStatusActive || s == LearningStatusStale || s == LearningStatusArchived
}

// Concept represents a knowledge category within a project.
type Concept struct {
	ID            string // con-XXXXXX
	Name          string
	Project       string
	Summary       string
	LastUpdated   time.Time
	LearningCount int // Derived from learning_concepts join
}

// Learning represents a piece of knowledge discovered during work.
type Learning struct {
	ID        string // lrn-XXXXXX
	Project   string
	CreatedAt time.Time
	UpdatedAt time.Time
	TaskID    *string // Optional link to the task that discovered this
	Summary   string  // One-liner
	Detail    string  // Full context
	Files     []string
	Status    LearningStatus
	Concepts  []string // Associated concept names
}

// GenerateLearningID returns a new learning ID with lrn- prefix and 6 hex chars.
func GenerateLearningID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return "lrn-" + hex.EncodeToString(b)
}

// GenerateConceptID returns a new concept ID with con- prefix and 6 hex chars.
func GenerateConceptID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return "con-" + hex.EncodeToString(b)
}

// Label represents a tag that can be attached to items for categorization.
// Labels are project-scoped and identified by name (IDs are internal).
type Label struct {
	ID        string // lbl-XXXXXX (internal)
	Name      string // User-facing identifier, unique per project
	Project   string
	Color     string // Optional hex color for UI display
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GenerateLabelID returns a new label ID with lbl- prefix and 6 hex chars.
func GenerateLabelID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return "lbl-" + hex.EncodeToString(b)
}
