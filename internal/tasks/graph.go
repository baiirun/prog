package tasks

import (
	"fmt"
	"slices"
	"sort"

	"github.com/baiirun/prog/internal/model"
)

// Graph is an immutable snapshot of every item and dependency edge, with
// effective statuses computed once in NewGraph. Every read the CLI and TUI
// make goes through a Graph, so derivation has exactly one implementation.
//
// Items returned by Graph methods carry effective status and populated
// Labels. Callers must not mutate their slices.
type Graph struct {
	items      map[string]model.Item // effective status applied
	order      []string              // item IDs in LoadGraph order
	deps       map[string][]string   // item ID -> depends-on IDs, dangling included
	dependents map[string][]string   // depends-on ID -> item IDs, existing endpoints only
}

// NewGraph validates records and derives effective statuses.
// Ensures: an error for any item whose stored type or status is invalid.
// Must not: fail on dangling parent or dependency references; they are
// ignored for derivation and resolution, matching the old SQL JOIN semantics.
func NewGraph(items []model.Item, deps []model.Dep) (*Graph, error) {
	g := &Graph{
		items:      make(map[string]model.Item, len(items)),
		order:      make([]string, 0, len(items)),
		deps:       make(map[string][]string),
		dependents: make(map[string][]string),
	}
	childStatuses := make(map[string][]model.Status)
	for _, item := range items {
		if !item.Type.IsValid() {
			return nil, fmt.Errorf("item %s has invalid stored type %q", item.ID, item.Type)
		}
		if !item.Status.IsValid() {
			return nil, fmt.Errorf("item %s has invalid stored status %q", item.ID, item.Status)
		}
		if _, dup := g.items[item.ID]; dup {
			return nil, fmt.Errorf("duplicate item ID %s", item.ID)
		}
		g.items[item.ID] = item
		g.order = append(g.order, item.ID)
		if item.ParentID != nil {
			childStatuses[*item.ParentID] = append(childStatuses[*item.ParentID], item.Status)
		}
	}

	for id, item := range g.items {
		if item.Type == model.ItemTypeEpic {
			item.Status = deriveEpicStatus(item.Status, childStatuses[id])
			g.items[id] = item
		}
	}

	for _, d := range deps {
		g.deps[d.ItemID] = append(g.deps[d.ItemID], d.DependsOn)
		if g.exists(d.ItemID) && g.exists(d.DependsOn) {
			g.dependents[d.DependsOn] = append(g.dependents[d.DependsOn], d.ItemID)
		}
	}
	return g, nil
}

// deriveEpicStatus is the epic status rule. children holds the stored status
// of each direct child (nested epics contribute their stored status, not
// their derived one, matching the old SQL).
//
//   - stored done/canceled wins (manual force-close)
//   - no children: draft
//   - all children done/canceled: done
//   - every unresolved child blocked: blocked
//   - any child in_progress, reviewing or done: in_progress
//   - every unresolved child draft: draft
//   - otherwise: open
//
// Requires: every status is valid (NewGraph checks); panics otherwise.
func deriveEpicStatus(stored model.Status, children []model.Status) model.Status {
	if isTerminal(stored) {
		return stored
	}
	if len(children) == 0 {
		return model.StatusDraft
	}

	count := make(map[model.Status]int, 7)
	for _, s := range children {
		if !s.IsValid() {
			panic(fmt.Sprintf("deriveEpicStatus: invalid child status %q", s))
		}
		count[s]++
	}

	unresolved := len(children) - count[model.StatusDone] - count[model.StatusCanceled]
	switch {
	case unresolved == 0:
		return model.StatusDone
	case count[model.StatusBlocked] == unresolved:
		return model.StatusBlocked
	case count[model.StatusInProgress] > 0 || count[model.StatusReviewing] > 0 || count[model.StatusDone] > 0:
		return model.StatusInProgress
	case count[model.StatusDraft] == unresolved:
		return model.StatusDraft
	default:
		return model.StatusOpen
	}
}

func isTerminal(s model.Status) bool {
	return s == model.StatusDone || s == model.StatusCanceled
}

func (g *Graph) exists(id string) bool {
	_, ok := g.items[id]
	return ok
}

// Item returns one item with its effective status.
// Ensures: a "not found" error that names the ID when it does not exist.
func (g *Graph) Item(id string) (model.Item, error) {
	item, ok := g.items[id]
	if !ok {
		return model.Item{}, fmt.Errorf("item not found: %s (use 'prog list' to see available items)", id)
	}
	return item, nil
}

// DepsOf returns the IDs id depends on, sorted, including targets that no
// longer exist. Empty (not nil) when there are none.
func (g *Graph) DepsOf(id string) []string {
	out := append([]string{}, g.deps[id]...)
	slices.Sort(out)
	return out
}

// hasUnresolvedDep reports whether any existing dependency target of id is
// not in a terminal effective status.
func (g *Graph) hasUnresolvedDep(id string) bool {
	for _, target := range g.deps[id] {
		if dep, ok := g.items[target]; ok && !isTerminal(dep.Status) {
			return true
		}
	}
	return false
}

// ListFilter selects items for List. Zero values mean "no filter".
type ListFilter struct {
	Project     string
	Status      *model.Status // matched against effective status
	Parent      string        // parent epic ID
	Type        string        // "task" or "epic"
	Blocking    string        // items that this ID depends on
	BlockedBy   string        // items that depend on this ID
	HasBlockers bool          // items with at least one unresolved dependency
	NoBlockers  bool          // items with no unresolved dependency
	Labels      []string      // items carrying all of these labels
}

// List returns matching items ordered by priority, then CreatedAt, then ID.
// Ensures: an error for an invalid Status or Type in the filter.
func (g *Graph) List(f ListFilter) ([]model.Item, error) {
	if f.Status != nil && !f.Status.IsValid() {
		return nil, fmt.Errorf("invalid status: %s", *f.Status)
	}
	if f.Type != "" && !model.ItemType(f.Type).IsValid() {
		return nil, fmt.Errorf("invalid type: %s (valid: task, epic)", f.Type)
	}

	var blocking, blockedBy map[string]bool
	if f.Blocking != "" {
		blocking = setOf(g.deps[f.Blocking])
	}
	if f.BlockedBy != "" {
		blockedBy = setOf(g.dependents[f.BlockedBy])
	}

	return g.selectSorted(func(item model.Item) bool {
		switch {
		case f.Project != "" && item.Project != f.Project,
			f.Status != nil && item.Status != *f.Status,
			f.Parent != "" && (item.ParentID == nil || *item.ParentID != f.Parent),
			f.Type != "" && string(item.Type) != f.Type,
			blocking != nil && !blocking[item.ID],
			blockedBy != nil && !blockedBy[item.ID],
			f.HasBlockers && !g.hasUnresolvedDep(item.ID),
			f.NoBlockers && g.hasUnresolvedDep(item.ID),
			!hasAllLabels(item, f.Labels):
			return false
		}
		return true
	}), nil
}

// Ready returns open tasks (never epics) with no unresolved dependency,
// ordered like List. project "" means all projects.
func (g *Graph) Ready(project string, labels []string) []model.Item {
	return g.selectSorted(func(item model.Item) bool {
		return item.Type == model.ItemTypeTask &&
			item.Status == model.StatusOpen &&
			(project == "" || item.Project == project) &&
			hasAllLabels(item, labels) &&
			!g.hasUnresolvedDep(item.ID)
	})
}

func (g *Graph) selectSorted(keep func(model.Item) bool) []model.Item {
	var out []model.Item
	for _, id := range g.order {
		if item := g.items[id]; keep(item) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	})
	return out
}

func hasAllLabels(item model.Item, labels []string) bool {
	for _, l := range labels {
		if !slices.Contains(item.Labels, l) {
			return false
		}
	}
	return true
}

func setOf(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// StatusReport summarizes a project by effective status.
type StatusReport struct {
	Project        string
	Draft          int
	Open           int
	InProgress     int
	Blocked        int
	Reviewing      int
	Done           int
	Canceled       int
	Ready          int
	DraftItems     []model.Item
	RecentDone     []model.Item // up to 3 effective-done items, newest UpdatedAt first
	InProgItems    []model.Item
	ReviewingItems []model.Item
	BlockedItems   []model.Item
	ReadyItems     []model.Item
}

const recentDoneLimit = 3

// Report builds a StatusReport. project "" means all projects.
func (g *Graph) Report(project string, labels []string) StatusReport {
	report := StatusReport{Project: project}
	var done []model.Item
	// The filter is always valid, so List cannot fail.
	items, _ := g.List(ListFilter{Project: project, Labels: labels})
	for _, item := range items {
		switch item.Status {
		case model.StatusDraft:
			report.Draft++
			report.DraftItems = append(report.DraftItems, item)
		case model.StatusOpen:
			report.Open++
		case model.StatusInProgress:
			report.InProgress++
			report.InProgItems = append(report.InProgItems, item)
		case model.StatusBlocked:
			report.Blocked++
			report.BlockedItems = append(report.BlockedItems, item)
		case model.StatusReviewing:
			report.Reviewing++
			report.ReviewingItems = append(report.ReviewingItems, item)
		case model.StatusDone:
			report.Done++
			done = append(done, item)
		case model.StatusCanceled:
			report.Canceled++
		default:
			panic(fmt.Sprintf("Report: item %s has invalid status %q after NewGraph", item.ID, item.Status))
		}
	}

	sort.SliceStable(done, func(i, j int) bool { return done[i].UpdatedAt.After(done[j].UpdatedAt) })
	report.RecentDone = done[:min(len(done), recentDoneLimit)]

	report.ReadyItems = g.Ready(project, labels)
	report.Ready = len(report.ReadyItems)
	return report
}

// DepEdge is one dependency with both endpoints' details.
type DepEdge struct {
	ItemID          string
	ItemTitle       string
	ItemStatus      model.Status // effective
	DependsOnID     string
	DependsOnTitle  string
	DependsOnStatus model.Status // effective
}

// Edges returns edges whose endpoints both exist and whose dependent item is
// in project ("" means all), ordered by the dependent's priority, then its
// ID, then the dependency's ID.
func (g *Graph) Edges(project string) []DepEdge {
	var edges []DepEdge
	for _, id := range g.order {
		item := g.items[id]
		if project != "" && item.Project != project {
			continue
		}
		for _, target := range g.deps[id] {
			dep, ok := g.items[target]
			if !ok {
				continue
			}
			edges = append(edges, DepEdge{
				ItemID: item.ID, ItemTitle: item.Title, ItemStatus: item.Status,
				DependsOnID: dep.ID, DependsOnTitle: dep.Title, DependsOnStatus: dep.Status,
			})
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		a, b := g.items[edges[i].ItemID], g.items[edges[j].ItemID]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return edges[i].DependsOnID < edges[j].DependsOnID
	})
	return edges
}
