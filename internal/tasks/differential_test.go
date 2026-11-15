package tasks

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/db"
	"github.com/baiirun/prog/internal/model"
)

// Temporary: lives only in the step-1 commit and is deleted in step 2 along
// with the SQL derivation it compares against.
//
// Builds random graphs (epics, nested epics, cross-project parents and deps,
// dangling deps, labels, every status) in SQLite and asserts that
// db.ListItemsFiltered (random filter combinations), ReadyItemsFiltered,
// ProjectStatusFiltered (except RecentDone, which changes on purpose),
// GetAllDeps and HasUnmetDeps agree with the Graph built from db.LoadGraph.
//
// With PROG_DIFF_DB set to a copy of a real database, runs the same
// comparison against it instead of random data.
func TestGraphMatchesSQL(t *testing.T) {
	if path := os.Getenv("PROG_DIFF_DB"); path != "" {
		database, err := db.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = database.Close() }()
		compareAll(t, database, rand.New(rand.NewPCG(1, 1)))
		return
	}
	for seed := range uint64(40) {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, seed))
			database := randomDB(t, rng)
			compareAll(t, database, rng)
		})
	}
}

var (
	diffProjects = []string{"alpha", "beta", "gamma"}
	diffLabels   = []string{"bug", "ui", "infra"}
	allStatuses  = []model.Status{model.StatusDraft, model.StatusOpen, model.StatusInProgress, model.StatusBlocked, model.StatusReviewing, model.StatusDone, model.StatusCanceled}
)

func randomDB(t *testing.T, rng *rand.Rand) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "diff.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Init(); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var all, epics []string
	n := 5 + rng.IntN(30)
	for i := range n {
		typ := model.ItemTypeTask
		if rng.IntN(4) == 0 {
			typ = model.ItemTypeEpic
		}
		it := &model.Item{
			ID:        model.GenerateID(typ),
			Project:   diffProjects[rng.IntN(len(diffProjects))],
			Type:      typ,
			Title:     fmt.Sprintf("item %d", i),
			Status:    allStatuses[rng.IntN(len(allStatuses))],
			Priority:  1 + rng.IntN(3),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
			UpdatedAt: base.Add(time.Duration(rng.IntN(1000)) * time.Minute),
		}
		if len(epics) > 0 && rng.IntN(2) == 0 {
			p := epics[rng.IntN(len(epics))]
			it.ParentID = &p
		}
		if err := database.CreateItem(it); err != nil {
			t.Fatal(err)
		}
		for _, l := range diffLabels {
			if rng.IntN(3) == 0 {
				if err := database.AddLabelToItem(it.ID, it.Project, l); err != nil {
					t.Fatal(err)
				}
			}
		}
		all = append(all, it.ID)
		if typ == model.ItemTypeEpic {
			epics = append(epics, it.ID)
		}
	}
	for range rng.IntN(2 * n) {
		a, b := all[rng.IntN(n)], all[rng.IntN(n)]
		if a != b {
			if err := database.AddDep(a, b); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Dangling edges only exist in legacy data, so bypass the foreign key.
	ctx := context.Background()
	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatal(err)
	}
	for range rng.IntN(3) {
		if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO deps (item_id, depends_on) VALUES (?, 'ts-gone')`, all[rng.IntN(n)]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	return database
}

func compareAll(t *testing.T, database *db.DB, rng *rand.Rand) {
	t.Helper()
	items, deps, err := database.LoadGraph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewGraph(items, deps)
	if err != nil {
		t.Fatal(err)
	}
	var allIDs, epicIDs []string
	projects := []string{""}
	labelNames := map[string]bool{}
	for _, it := range items {
		allIDs = append(allIDs, it.ID)
		if it.Type == model.ItemTypeEpic {
			epicIDs = append(epicIDs, it.ID)
		}
		if !slices.Contains(projects, it.Project) {
			projects = append(projects, it.Project)
		}
		for _, l := range it.Labels {
			labelNames[l] = true
		}
	}
	var labels []string
	for l := range labelNames {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	pick := func(xs []string) string {
		if len(xs) == 0 || rng.IntN(3) != 0 {
			return ""
		}
		return xs[rng.IntN(len(xs))]
	}
	pickLabels := func() []string {
		var out []string
		for _, l := range labels {
			if rng.IntN(6) == 0 {
				out = append(out, l)
			}
		}
		return out
	}

	for _, id := range allIDs {
		want, err := database.GetItem(id)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := g.Item(id)
		if got.Status != want.Status {
			t.Errorf("Item(%s).Status = %s, SQL %s", id, got.Status, want.Status)
		}
		wantUnmet, err := database.HasUnmetDeps(id)
		if err != nil {
			t.Fatal(err)
		}
		if g.hasUnresolvedDep(id) != wantUnmet {
			t.Errorf("hasUnresolvedDep(%s) = %v, SQL %v", id, g.hasUnresolvedDep(id), wantUnmet)
		}
		wantDeps, _ := database.GetDeps(id)
		if !slices.Equal(g.DepsOf(id), append([]string{}, wantDeps...)) {
			t.Errorf("DepsOf(%s) = %v, SQL %v", id, g.DepsOf(id), wantDeps)
		}
	}

	for range 300 {
		var status *model.Status
		if rng.IntN(3) == 0 {
			s := allStatuses[rng.IntN(len(allStatuses))]
			status = &s
		}
		f := ListFilter{
			Project:     projects[rng.IntN(len(projects))],
			Status:      status,
			Parent:      pick(epicIDs),
			Type:        pick([]string{"task", "epic"}),
			Blocking:    pick(allIDs),
			BlockedBy:   pick(allIDs),
			HasBlockers: rng.IntN(5) == 0,
			NoBlockers:  rng.IntN(5) == 0,
			Labels:      pickLabels(),
		}
		want, err := database.ListItemsFiltered(db.ListFilter(f))
		if err != nil {
			t.Fatal(err)
		}
		got, err := g.List(f)
		if err != nil {
			t.Fatal(err)
		}
		assertSameItems(t, fmt.Sprintf("List(%+v)", f), got, want)
	}

	for _, p := range projects {
		for range 5 {
			ls := pickLabels()
			wantReady, err := database.ReadyItemsFiltered(p, ls)
			if err != nil {
				t.Fatal(err)
			}
			assertSameItems(t, fmt.Sprintf("Ready(%q, %v)", p, ls), g.Ready(p, ls), wantReady)

			want, err := database.ProjectStatusFiltered(p, ls)
			if err != nil {
				t.Fatal(err)
			}
			got := g.Report(p, ls)
			ctx := fmt.Sprintf("Report(%q, %v)", p, ls)
			gotCounts := []int{got.Draft, got.Open, got.InProgress, got.Blocked, got.Reviewing, got.Done, got.Canceled, got.Ready}
			wantCounts := []int{want.Draft, want.Open, want.InProgress, want.Blocked, want.Reviewing, want.Done, want.Canceled, want.Ready}
			if !slices.Equal(gotCounts, wantCounts) {
				t.Errorf("%s counts = %v, SQL %v", ctx, gotCounts, wantCounts)
			}
			assertSameItems(t, ctx+".DraftItems", got.DraftItems, want.DraftItems)
			assertSameItems(t, ctx+".InProgItems", got.InProgItems, want.InProgItems)
			assertSameItems(t, ctx+".ReviewingItems", got.ReviewingItems, want.ReviewingItems)
			assertSameItems(t, ctx+".BlockedItems", got.BlockedItems, want.BlockedItems)
			assertSameItems(t, ctx+".ReadyItems", got.ReadyItems, want.ReadyItems)
		}

		wantEdges, err := database.GetAllDeps(p)
		if err != nil {
			t.Fatal(err)
		}
		var w, gl []string
		for _, e := range wantEdges {
			w = append(w, fmt.Sprintf("%s[%s]->%s[%s]", e.ItemID, e.ItemStatus, e.DependsOnID, e.DependsOnStatus))
		}
		for _, e := range g.Edges(p) {
			gl = append(gl, fmt.Sprintf("%s[%s]->%s[%s]", e.ItemID, e.ItemStatus, e.DependsOnID, e.DependsOnStatus))
		}
		sort.Strings(w)
		sort.Strings(gl)
		if !slices.Equal(gl, w) {
			t.Errorf("Edges(%q) = %v, SQL %v", p, gl, w)
		}
	}
}

// assertSameItems compares ID and effective status. Order must match except
// among items whose (priority, created_at) tie, which SQL leaves unordered.
func assertSameItems(t *testing.T, what string, got, want []model.Item) {
	t.Helper()
	g, w := canonical(got), canonical(want)
	if g != w {
		t.Errorf("%s mismatch\n got: %s\nwant: %s", what, g, w)
		return
	}
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		if a.Priority > b.Priority || (a.Priority == b.Priority && a.CreatedAt.After(b.CreatedAt)) {
			t.Errorf("%s not ordered by priority, created_at at %s, %s", what, a.ID, b.ID)
		}
	}
	for i := range want {
		if want[i].Priority != got[i].Priority || !want[i].CreatedAt.Equal(got[i].CreatedAt) {
			t.Errorf("%s order differs from SQL at %d: got %s, SQL %s", what, i, got[i].ID, want[i].ID)
			return
		}
	}
}

func canonical(items []model.Item) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, it.ID+"="+string(it.Status))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
