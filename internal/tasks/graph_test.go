package tasks

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
)

// Protects the epic rule itself.
func TestDeriveEpicStatus(t *testing.T) {
	const (
		draft = model.StatusDraft
		open  = model.StatusOpen
		prog  = model.StatusInProgress
		blk   = model.StatusBlocked
		rev   = model.StatusReviewing
		done  = model.StatusDone
		canc  = model.StatusCanceled
	)
	tests := []struct {
		name     string
		stored   model.Status
		children []model.Status
		want     model.Status
	}{
		{"no children", open, nil, draft},
		{"all done", open, []model.Status{done, done}, done},
		{"all canceled", open, []model.Status{canc}, done},
		{"done and canceled", open, []model.Status{done, canc}, done},
		{"in progress child", open, []model.Status{prog, open}, prog},
		{"reviewing child", open, []model.Status{rev, open}, prog},
		{"done with open remaining", open, []model.Status{done, open}, prog},
		{"all blocked", open, []model.Status{blk, blk}, blk},
		{"blocked with done", open, []model.Status{done, blk}, blk},
		{"all draft", open, []model.Status{draft, draft}, draft},
		{"draft and open", open, []model.Status{draft, open}, open},
		{"all open", open, []model.Status{open, open}, open},
		{"force-closed done", done, []model.Status{open}, done},
		{"force-closed canceled", canc, []model.Status{open}, canc},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveEpicStatus(tt.stored, tt.children); got != tt.want {
				t.Errorf("deriveEpicStatus(%s, %v) = %s, want %s", tt.stored, tt.children, got, tt.want)
			}
		})
	}
}

// item builds a stored record. Timestamps are offsets so ordering is explicit.
func item(id, project string, typ model.ItemType, status model.Status, opts ...func(*model.Item)) model.Item {
	it := model.Item{ID: id, Project: project, Type: typ, Title: id, Status: status, Priority: 2}
	for _, o := range opts {
		o(&it)
	}
	return it
}

func parent(id string) func(*model.Item) { return func(it *model.Item) { it.ParentID = &id } }

func updated(minutes int) func(*model.Item) {
	return func(it *model.Item) { it.UpdatedAt = time.Unix(0, 0).Add(time.Duration(minutes) * time.Minute) }
}

func mustGraph(t *testing.T, items []model.Item, deps ...model.Dep) *Graph {
	t.Helper()
	g, err := NewGraph(items, deps)
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	return g
}

func ids(items []model.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

const (
	task = model.ItemTypeTask
	epic = model.ItemTypeEpic
)

// Protects the whole-graph decision: a project-scoped snapshot would miss a
// child or blocker in another project (the live DB has both).
func TestGraph_CrossProjectChildAndBlocker(t *testing.T) {
	g := mustGraph(t, []model.Item{
		item("ep-a", "alpha", epic, model.StatusOpen),
		item("ts-child", "beta", task, model.StatusInProgress, parent("ep-a")),
		item("ts-blocker", "beta", task, model.StatusOpen),
		item("ts-waiting", "alpha", task, model.StatusOpen),
	}, model.Dep{ItemID: "ts-waiting", DependsOn: "ts-blocker"})

	e, _ := g.Item("ep-a")
	if e.Status != model.StatusInProgress {
		t.Errorf("epic with in-progress child in another project = %s, want in_progress", e.Status)
	}
	if got := ids(g.Ready("alpha", nil)); len(got) != 0 {
		t.Errorf("Ready(alpha) = %v, want none: ts-waiting is blocked from project beta", got)
	}
}

// Protects the dangling-edge rule: an edge to a deleted item never blocks,
// but DepsOf still reports it.
func TestGraph_DanglingDepDoesNotBlock(t *testing.T) {
	g := mustGraph(t, []model.Item{item("ts-a", "p", task, model.StatusOpen)},
		model.Dep{ItemID: "ts-a", DependsOn: "ts-gone"})

	if got := ids(g.Ready("p", nil)); len(got) != 1 || got[0] != "ts-a" {
		t.Errorf("Ready = %v, want [ts-a]", got)
	}
	if got := g.DepsOf("ts-a"); len(got) != 1 || got[0] != "ts-gone" {
		t.Errorf("DepsOf = %v, want [ts-gone]", got)
	}
	if got := g.Edges(""); len(got) != 0 {
		t.Errorf("Edges = %v, want none for a dangling edge", got)
	}
}

// Protects the one intentional behavior change: RecentDone uses effective
// status (derived-done epics count, canceled items don't crowd out done ones).
func TestReport_RecentDoneUsesEffectiveStatus(t *testing.T) {
	items := []model.Item{
		item("ep-derived", "p", epic, model.StatusOpen, updated(50)),
		item("ts-child", "p", task, model.StatusDone, parent("ep-derived"), updated(1)),
		item("ts-old", "p", task, model.StatusDone, updated(2)),
		item("ts-new", "p", task, model.StatusDone, updated(40)),
	}
	for i := range 10 {
		items = append(items, item(fmt.Sprintf("ts-canc%d", i), "p", task, model.StatusCanceled, updated(100+i)))
	}
	report := mustGraph(t, items).Report("p", nil)

	want := []string{"ep-derived", "ts-new", "ts-old"}
	if got := ids(report.RecentDone); !slices.Equal(got, want) {
		t.Errorf("RecentDone = %v, want %v", got, want)
	}
	if report.Done != 4 || report.Canceled != 10 {
		t.Errorf("Done=%d Canceled=%d, want 4 and 10", report.Done, report.Canceled)
	}
}

// Protects resolution == effective terminal status: a task blocked on an epic
// becomes ready once every child is done, without touching the epic.
func TestReady_EpicDependencyResolvesFromChildren(t *testing.T) {
	build := func(childStatus model.Status) *Graph {
		return mustGraph(t, []model.Item{
			item("ep-a", "p", epic, model.StatusOpen),
			item("ts-c1", "p", task, model.StatusDone, parent("ep-a")),
			item("ts-c2", "p", task, childStatus, parent("ep-a")),
			item("ts-after", "p", task, model.StatusOpen),
		}, model.Dep{ItemID: "ts-after", DependsOn: "ep-a"})
	}

	if got := ids(build(model.StatusInProgress).Ready("p", nil)); slices.Contains(got, "ts-after") {
		t.Errorf("Ready = %v; ts-after must wait while an epic child is in progress", got)
	}
	if got := ids(build(model.StatusCanceled).Ready("p", nil)); !slices.Contains(got, "ts-after") {
		t.Errorf("Ready = %v; ts-after must be ready once every epic child is done or canceled", got)
	}
}

// Protects fail-closed parsing at the boundary.
func TestNewGraph_RejectsInvalidStoredStatus(t *testing.T) {
	_, err := NewGraph([]model.Item{item("ts-a", "p", task, "bogus")}, nil)
	if err == nil {
		t.Fatal("NewGraph accepted an invalid stored status")
	}
	if _, err := NewGraph([]model.Item{item("ts-a", "p", "story", model.StatusOpen)}, nil); err == nil {
		t.Fatal("NewGraph accepted an invalid stored type")
	}
}

// Protects `prog graph` output order: deps under one item are listed by ID,
// so output does not depend on insertion order.
func TestEdges_OrderedByDependentThenDependency(t *testing.T) {
	g := mustGraph(t, []model.Item{
		item("ts-b", "p", task, model.StatusOpen),
		item("ts-a", "p", task, model.StatusOpen),
		item("ts-z", "p", task, model.StatusOpen),
		item("ts-y", "p", task, model.StatusOpen),
	},
		model.Dep{ItemID: "ts-b", DependsOn: "ts-z"},
		model.Dep{ItemID: "ts-b", DependsOn: "ts-y"},
		model.Dep{ItemID: "ts-a", DependsOn: "ts-z"},
	)
	var got []string
	for _, e := range g.Edges("") {
		got = append(got, e.ItemID+">"+e.DependsOnID)
	}
	want := []string{"ts-a>ts-z", "ts-b>ts-y", "ts-b>ts-z"}
	if !slices.Equal(got, want) {
		t.Errorf("Edges = %v, want %v", got, want)
	}
}

func labels(names ...string) func(*model.Item) { return func(it *model.Item) { it.Labels = names } }

// Protects `-l a -l b` semantics in list, ready and status: an item must carry
// every requested label, not any of them.
func TestLabelFilter_RequiresAllLabels(t *testing.T) {
	g := mustGraph(t, []model.Item{
		item("ts-both", "p", task, model.StatusOpen, labels("bug", "ui")),
		item("ts-bug", "p", task, model.StatusOpen, labels("bug")),
		item("ts-none", "p", task, model.StatusOpen),
	})
	want := []string{"ts-both"}
	if got := ids(g.List(ListFilter{Labels: []string{"bug", "ui"}})); !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if got := ids(g.Ready("p", []string{"bug", "ui"})); !slices.Equal(got, want) {
		t.Errorf("Ready = %v, want %v", got, want)
	}
	if r := g.Report("p", []string{"bug", "ui"}); r.Open != 1 || r.Ready != 1 {
		t.Errorf("Report Open=%d Ready=%d, want 1 and 1", r.Open, r.Ready)
	}
}

// Protects which task `prog ready` offers first among equal priorities:
// oldest CreatedAt, then lowest ID.
func TestReady_TieBreakByCreatedAtThenID(t *testing.T) {
	created := func(minutes int) func(*model.Item) {
		return func(it *model.Item) { it.CreatedAt = time.Unix(0, 0).Add(time.Duration(minutes) * time.Minute) }
	}
	g := mustGraph(t, []model.Item{
		item("ts-c", "p", task, model.StatusOpen, created(1)),
		item("ts-b", "p", task, model.StatusOpen, created(1)),
		item("ts-a", "p", task, model.StatusOpen, created(2)),
		item("ts-urgent", "p", task, model.StatusOpen, created(9), func(it *model.Item) { it.Priority = 1 }),
	})
	want := []string{"ts-urgent", "ts-b", "ts-c", "ts-a"}
	if got := ids(g.Ready("p", nil)); !slices.Equal(got, want) {
		t.Errorf("Ready = %v, want %v", got, want)
	}
}
