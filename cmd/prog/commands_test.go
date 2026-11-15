package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/model"
)

// Guards: `add --parent` links the new item to the epic.
func TestAdd_ParentLinksToEpic(t *testing.T) {
	p := newProg(t)
	epic := p.ok("add", "Epic", "-p", "proj", "-e")
	task := p.ok("add", "Child", "-p", "proj", "--parent", epic)

	if got := p.show(task)["parent"]; got != epic {
		t.Errorf("parent = %#v, want %q", got, epic)
	}
}

// Guards: `add --parent` rejects a parent that is not an epic.
func TestAdd_ParentMustBeEpic(t *testing.T) {
	p := newProg(t)
	notEpic := p.ok("add", "Plain task", "-p", "proj")
	p.fails("add", "Child", "-p", "proj", "--parent", notEpic)
}

// Guards: `add --blocks X` makes X depend on the new item, not the reverse.
func TestAdd_BlocksMakesTargetDependOnNewItem(t *testing.T) {
	p := newProg(t)
	target := p.ok("add", "Target", "-p", "proj")
	blocker := p.ok("add", "Blocker", "-p", "proj", "--blocks", target)

	if deps := p.show(target)["dependencies"]; !slices.Equal(toStrings(deps), []string{blocker}) {
		t.Errorf("target dependencies = %#v, want [%s]", deps, blocker)
	}
	if got := ids(p.jsonArray("ready", "-p", "proj", "--json")); !slices.Equal(got, []string{blocker}) {
		t.Errorf("ready before blocker done = %v, want only [%s]", got, blocker)
	}

	p.ok("done", blocker)
	if got := ids(p.jsonArray("ready", "-p", "proj", "--json")); !slices.Equal(got, []string{target}) {
		t.Errorf("ready after blocker done = %v, want [%s]", got, target)
	}
}

// Guards: `add --blocks` with an unknown ID is an error, not a silent no-op.
func TestAdd_BlocksMissingTargetFails(t *testing.T) {
	p := newProg(t)
	p.fails("add", "Blocker", "-p", "proj", "--blocks", "ts-000000")
}

// Guards: -l, -d and --dod reach the stored item.
func TestAdd_LabelsDescriptionAndDoD(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Rich", "-p", "proj", "-l", "bug", "-l", "urgent",
		"-d", "the description", "--dod", "tests pass")

	got := p.show(id)
	labels := toStrings(got["labels"])
	slices.Sort(labels)
	if !slices.Equal(labels, []string{"bug", "urgent"}) {
		t.Errorf("labels = %v, want [bug urgent]", labels)
	}
	if got["description"] != "the description" {
		t.Errorf("description = %#v", got["description"])
	}
	if got["definition_of_done"] != "tests pass" {
		t.Errorf("definition_of_done = %#v", got["definition_of_done"])
	}
}

// Guards: -p is trimmed once at the root, so add and list agree on the name.
func TestAdd_ProjectWhitespaceTrimmed(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Spaced", "-p", "  proj  ")

	if got := ids(p.jsonArray("list", "-p", "proj", "--json")); !slices.Equal(got, []string{id}) {
		t.Errorf("list -p proj = %v, want [%s]", got, id)
	}
	if got := p.show(id)["project"]; got != "proj" {
		t.Errorf("project = %#v, want %q", got, "proj")
	}
}

// Guards: review moves in_progress to reviewing.
func TestReview_FromInProgress(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Work", "-p", "proj")
	p.ok("start", id)
	p.ok("review", id)

	if got := p.show(id)["status"]; got != "reviewing" {
		t.Errorf("status = %#v, want reviewing", got)
	}
}

// Guards: review is gated on in_progress and leaves other statuses alone.
func TestReview_RejectsNonInProgress(t *testing.T) {
	setups := map[string]func(p *progEnv, id string){
		"open":    func(p *progEnv, id string) {},
		"done":    func(p *progEnv, id string) { p.ok("done", id) },
		"blocked": func(p *progEnv, id string) { p.setStatus(id, model.StatusBlocked) },
	}
	for status, setup := range setups {
		t.Run(status, func(t *testing.T) {
			p := newProg(t)
			id := p.ok("add", "Work", "-p", "proj")
			setup(p, id)

			p.fails("review", id)
			if got := p.show(id)["status"]; got != status {
				t.Errorf("status after rejected review = %#v, want %q", got, status)
			}
		})
	}
}

// Guards: show --json carries every detail field, with UTC timestamps.
func TestShowJSON_FullDetail(t *testing.T) {
	p := newProg(t)
	epic := p.ok("add", "Epic", "-p", "proj", "-e")
	dep := p.ok("add", "Dep", "-p", "proj")
	id := p.ok("add", "Show me", "-p", "proj", "--parent", epic, "-l", "bug",
		"-d", "desc", "--dod", "done when green", "--priority", "1")
	p.ok("blocks", dep, id)
	p.ok("log", id, "first", "entry")

	got := p.show(id)
	want := map[string]any{
		"id": id, "title": "Show me", "type": "task", "status": "open",
		"priority": float64(1), "project": "proj", "parent": epic,
		"description": "desc", "definition_of_done": "done when green",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	if l := toStrings(got["labels"]); !slices.Equal(l, []string{"bug"}) {
		t.Errorf("labels = %v, want [bug]", l)
	}
	if d := toStrings(got["dependencies"]); !slices.Equal(d, []string{dep}) {
		t.Errorf("dependencies = %v, want [%s]", d, dep)
	}
	assertUTCTimestamp(t, got, "created_at")
	assertUTCTimestamp(t, got, "updated_at")

	logs, _ := got["logs"].([]any)
	if len(logs) != 1 {
		t.Fatalf("logs = %#v, want one entry", got["logs"])
	}
	entry, _ := logs[0].(map[string]any)
	if entry["message"] != "first entry" {
		t.Errorf("log message = %#v, want %q", entry["message"], "first entry")
	}
	assertUTCTimestamp(t, entry, "created_at")
}

// Guards: show --json emits [] rather than null for empty collections.
func TestShowJSON_EmptyCollectionsAreArrays(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Bare", "-p", "proj")

	got := p.show(id)
	for _, field := range []string{"labels", "dependencies", "logs"} {
		assertEmptyArray(t, got, field)
	}
	if got["parent"] != nil || got["definition_of_done"] != nil {
		t.Errorf("parent/definition_of_done = %#v/%#v, want null", got["parent"], got["definition_of_done"])
	}
}

// Guards: show reports an epic's derived status, not its stored one.
func TestShowJSON_EpicEffectiveStatus(t *testing.T) {
	p := newProg(t)
	epic := p.ok("add", "Epic", "-p", "proj", "-e")
	child := p.ok("add", "Only child", "-p", "proj", "--parent", epic)
	p.ok("done", child)

	if got := p.show(epic)["status"]; got != "done" {
		t.Errorf("epic status = %#v, want done (all children done)", got)
	}
}

// Guards: list --json carries the list schema (timestamps, [] collections,
// no logs) for real items.
func TestListJSON_Fields(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Listed", "-p", "proj", "--priority", "3")

	arr := p.jsonArray("list", "-p", "proj", "--json")
	if len(arr) != 1 {
		t.Fatalf("list returned %d items, want 1", len(arr))
	}
	got := arr[0]
	want := map[string]any{
		"id": id, "title": "Listed", "type": "task", "status": "open",
		"priority": float64(3), "project": "proj",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	assertEmptyArray(t, got, "labels")
	assertEmptyArray(t, got, "dependencies")
	assertUTCTimestamp(t, got, "created_at")
	assertUTCTimestamp(t, got, "updated_at")
	if _, hasLogs := got["logs"]; hasLogs {
		t.Error("list JSON must not include logs")
	}
}

// Guards: list and ready print [] (not null or text) when nothing matches.
func TestJSON_EmptyResultsAreArrays(t *testing.T) {
	p := newProg(t)
	p.ok("add", "Elsewhere", "-p", "other")

	for _, args := range [][]string{
		{"list", "-p", "proj", "--json"},
		{"ready", "-p", "proj", "--json"},
	} {
		if arr := p.jsonArray(args...); len(arr) != 0 {
			t.Errorf("prog %s = %v, want []", strings.Join(args, " "), arr)
		}
	}
}

// Guards: ready lists only unblocked tasks (no epics), highest priority first,
// with the ready schema.
func TestReadyJSON_FiltersAndOrders(t *testing.T) {
	p := newProg(t)
	epic := p.ok("add", "Epic", "-p", "proj", "-e")
	low := p.ok("add", "Low", "-p", "proj", "--priority", "3")
	high := p.ok("add", "High", "-p", "proj", "--priority", "1", "--parent", epic)
	mid := p.ok("add", "Mid", "-p", "proj", "--priority", "2")
	blocked := p.ok("add", "Blocked", "-p", "proj", "--priority", "1")
	p.ok("blocks", low, blocked)

	arr := p.jsonArray("ready", "-p", "proj", "--json")
	if got, want := ids(arr), []string{high, mid, low}; !slices.Equal(got, want) {
		t.Fatalf("ready = %v, want %v (epic %s and blocked %s excluded)", got, want, epic, blocked)
	}
	first := arr[0]
	want := map[string]any{"title": "High", "priority": float64(1), "type": "task", "parent": epic}
	for k, v := range want {
		if first[k] != v {
			t.Errorf("%s = %#v, want %#v", k, first[k], v)
		}
	}
	if _, ok := arr[1]["parent"]; !ok || arr[1]["parent"] != nil {
		t.Errorf("parent for unparented task = %#v, want explicit null", arr[1]["parent"])
	}
}

// Guards: invalid list filters are rejected by the command.
func TestList_RejectsInvalidFlags(t *testing.T) {
	p := newProg(t)
	for _, args := range [][]string{
		{"list", "--type", "bogus"},
		{"list", "--status", "bogus"},
		{"list", "--has-blockers", "--no-blockers"},
	} {
		t.Run(strings.Join(args[1:], " "), func(t *testing.T) {
			p.fails(args...)
		})
	}
}

// Guards: deleting a labeled task removes it (labels don't block deletion).
func TestDelete_LabeledTask(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Doomed", "-p", "proj", "-l", "bug")
	p.ok("delete", id)
	p.fails("show", id)
}

// Guards: an epic with children cannot be deleted and survives the attempt.
func TestDelete_EpicWithChildFails(t *testing.T) {
	p := newProg(t)
	epic := p.ok("add", "Epic", "-p", "proj", "-e")
	p.ok("add", "Child", "-p", "proj", "--parent", epic)

	p.fails("delete", epic)
	if got := p.show(epic)["id"]; got != epic {
		t.Errorf("epic missing after rejected delete")
	}
}

// Guards: prime loads the real database and surfaces in-progress work.
func TestPrime_ShowsInProgressTask(t *testing.T) {
	p := newProg(t)
	id := p.ok("add", "Prime me", "-p", "proj")
	p.ok("start", id)

	out := p.ok("prime")
	if !strings.Contains(out, id) || !strings.Contains(out, "Prime me") {
		t.Errorf("prime output missing in-progress task %s:\n%s", id, out)
	}
	if strings.Contains(out, "No database connection") {
		t.Error("prime fell back to the no-database variant")
	}
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}
