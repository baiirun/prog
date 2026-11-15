package tasks

import (
	"context"

	"github.com/baiirun/prog/internal/model"
)

// Service is the API the CLI and TUI use. Methods with rules validate against
// a fresh Graph before writing; the rest pass straight through to the Store.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// Graph loads a fresh snapshot. Callers load once per command and read from it.
func (s *Service) Graph(ctx context.Context) (*Graph, error) {
	panic("not implemented")
}

// CreateItem validates type and status, normalizes the project and ensures it
// exists, then creates the item.
func (s *Service) CreateItem(ctx context.Context, item model.NewItem) (model.Item, error) {
	panic("not implemented")
}

// SetStatus changes an item's stored status.
// Ensures: an error for epics unless status is done or canceled, because
// derivation would hide any other stored value.
func (s *Service) SetStatus(ctx context.Context, id string, status model.Status) error {
	panic("not implemented")
}

// SetParent makes parentID the parent of id.
// Ensures: an error unless both exist and parentID is an epic.
func (s *Service) SetParent(ctx context.Context, id, parentID string) error {
	panic("not implemented")
}

// SetProject normalizes the project, ensures it exists, then moves the item.
func (s *Service) SetProject(ctx context.Context, id, project string) error {
	panic("not implemented")
}

// AddDep records that itemID depends on dependsOnID.
// Ensures: an error unless both items exist.
func (s *Service) AddDep(ctx context.Context, itemID, dependsOnID string) error {
	panic("not implemented")
}

// AddLabel attaches a label from the item's own project, creating it if missing.
func (s *Service) AddLabel(ctx context.Context, itemID, name string) error {
	panic("not implemented")
}

// RemoveLabel detaches a label from the item's own project.
func (s *Service) RemoveLabel(ctx context.Context, itemID, name string) error {
	panic("not implemented")
}

func (s *Service) SetTitle(ctx context.Context, id, title string) error {
	return s.store.SetTitle(ctx, id, title)
}

func (s *Service) SetDescription(ctx context.Context, id, text string) error {
	return s.store.SetDescription(ctx, id, text)
}

func (s *Service) AppendDescription(ctx context.Context, id, text string) error {
	return s.store.AppendDescription(ctx, id, text)
}

func (s *Service) SetDefinitionOfDone(ctx context.Context, id string, dod *string) error {
	return s.store.SetDefinitionOfDone(ctx, id, dod)
}

func (s *Service) DeleteItem(ctx context.Context, id string) error {
	return s.store.DeleteItem(ctx, id)
}

func (s *Service) AddLog(ctx context.Context, itemID, message string) error {
	return s.store.AddLog(ctx, itemID, message)
}

func (s *Service) Logs(ctx context.Context, itemID string) ([]model.Log, error) {
	return s.store.Logs(ctx, itemID)
}

func (s *Service) ListProjects(ctx context.Context) ([]string, error) {
	return s.store.ListProjects(ctx)
}

func (s *Service) CreateLabel(ctx context.Context, label model.NewLabel) error {
	return s.store.CreateLabel(ctx, label)
}

func (s *Service) ListLabels(ctx context.Context, project string) ([]model.Label, error) {
	return s.store.ListLabels(ctx, project)
}

func (s *Service) RenameLabel(ctx context.Context, project, oldName, newName string) error {
	return s.store.RenameLabel(ctx, project, oldName, newName)
}

func (s *Service) DeleteLabel(ctx context.Context, project, name string) error {
	return s.store.DeleteLabel(ctx, project, name)
}
