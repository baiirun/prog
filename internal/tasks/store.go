// Package tasks holds prog's domain rules: epic status derivation, dependency
// resolution, readiness, status reports and write validation. Storage is
// behind Store, so a backend only maps records and never re-implements rules.
//
// Invariants:
//   - An item's effective status is its derived status (see deriveEpicStatus).
//     Only Graph computes it; no backend or caller derives status itself.
//   - A dependency is resolved exactly when its target's effective status is
//     done or canceled. Edges whose target does not exist never block.
//   - Derivation and resolution read the whole graph: parents, children and
//     dependencies may span projects.
package tasks

import (
	"context"

	"github.com/baiirun/prog/internal/model"
)

// Store persists prog's records. It holds no domain rules: implementations
// store what they are given and return *NotFoundError for any missing item or
// label. Service is the only consumer.
type Store interface {
	// LoadGraph returns every item, with stored (not derived) status and
	// Labels populated and sorted by name, and every dependency edge,
	// including edges whose target no longer exists.
	LoadGraph(ctx context.Context) ([]model.Item, []model.Dep, error)

	// CreateItem stores a new item and returns it with its assigned ID.
	// Requires: the item's project exists (see EnsureProject), and its
	// ParentID, if set, names an existing item.
	CreateItem(ctx context.Context, item model.NewItem) (model.Item, error)
	// DeleteItem atomically removes an item with its logs, labels and
	// dependency edges. Fails, deleting nothing, if the item has children.
	DeleteItem(ctx context.Context, id string) error

	// The setters below fail when id does not exist and bump UpdatedAt.
	SetStatus(ctx context.Context, id string, status model.Status) error
	SetTitle(ctx context.Context, id, title string) error
	SetDescription(ctx context.Context, id, text string) error
	// AppendDescription appends "\n\n"+text atomically with respect to
	// concurrent appends to the same item.
	AppendDescription(ctx context.Context, id, text string) error
	// SetDefinitionOfDone sets the DoD; nil clears it.
	SetDefinitionOfDone(ctx context.Context, id string, dod *string) error
	SetParent(ctx context.Context, id, parentID string) error
	// SetProject requires: the project exists (see EnsureProject).
	SetProject(ctx context.Context, id, project string) error

	// AddDep records that itemID depends on dependsOnID. Idempotent.
	AddDep(ctx context.Context, itemID, dependsOnID string) error

	AddLog(ctx context.Context, itemID, message string) error
	// Logs returns an item's logs, oldest first.
	Logs(ctx context.Context, itemID string) ([]model.Log, error)

	// EnsureProject creates the project if missing. Idempotent.
	EnsureProject(ctx context.Context, name string) error
	// ListProjects returns project names sorted by name.
	ListProjects(ctx context.Context) ([]string, error)

	// CreateLabel fails if the name already exists in the project.
	CreateLabel(ctx context.Context, label model.NewLabel) error
	// ListLabels returns a project's labels sorted by name.
	ListLabels(ctx context.Context, project string) ([]model.Label, error)
	RenameLabel(ctx context.Context, project, oldName, newName string) error
	// DeleteLabel removes the label and detaches it from every item.
	DeleteLabel(ctx context.Context, project, name string) error
	// AddLabelToItem creates the label in project if missing. Idempotent.
	AddLabelToItem(ctx context.Context, itemID, project, name string) error
	// RemoveLabelFromItem fails if the item does not carry the label.
	RemoveLabelFromItem(ctx context.Context, itemID, project, name string) error
}
