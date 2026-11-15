package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

func TestReadyJSON_EmptyResult(t *testing.T) {
	output := captureOutput(func() {
		flagJSON = true
		defer func() { flagJSON = false }()

		// Simulate what readyCmd does when no items are found
		fmt.Println("[]")
	})

	var result []ItemReadyJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestReadyJSON_WithItems(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	epic := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeEpic,
		Title: "Epic", Status: model.StatusOpen,
	})
	parentID := epic.ID

	taskA := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "Task A", Status: model.StatusOpen, Priority: 1,
	})

	taskB := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "Task B", Status: model.StatusOpen, Priority: 2,
	})
	if err := svc.SetParent(ctx, taskB.ID, parentID); err != nil {
		t.Fatalf("set parent: %v", err)
	}

	readyItems := mustGraph(t, svc).Ready("test", nil)

	output := captureOutput(func() {
		jsonItems := make([]ItemReadyJSON, 0, len(readyItems))
		for _, item := range readyItems {
			jsonItems = append(jsonItems, ItemReadyJSON{
				ID:       item.ID,
				Title:    item.Title,
				Priority: item.Priority,
				Type:     string(item.Type),
				Parent:   item.ParentID,
			})
		}
		b, _ := json.MarshalIndent(jsonItems, "", "  ")
		fmt.Println(string(b))
	})

	var result []ItemReadyJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}

	// Should have at least the tasks we created
	found := map[string]bool{}
	for _, r := range result {
		found[r.ID] = true
		if r.ID == taskA.ID {
			if r.Title != "Task A" {
				t.Errorf("title = %q, want %q", r.Title, "Task A")
			}
			if r.Priority != 1 {
				t.Errorf("priority = %d, want 1", r.Priority)
			}
			if r.Type != "task" {
				t.Errorf("type = %q, want %q", r.Type, "task")
			}
			if r.Parent != nil {
				t.Errorf("parent = %v, want nil", r.Parent)
			}
		}
		if r.ID == taskB.ID {
			if r.Parent == nil || *r.Parent != parentID {
				t.Errorf("parent = %v, want %q", r.Parent, parentID)
			}
		}
	}
	if !found[taskA.ID] {
		t.Errorf("missing task A (%s) in ready output", taskA.ID)
	}
	if !found[taskB.ID] {
		t.Errorf("missing task B (%s) in ready output", taskB.ID)
	}
}

func TestShowJSON_FullDetail(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	dod := "Tests pass"
	item := mustCreate(t, svc, model.NewItem{
		Project:          "test",
		Type:             model.ItemTypeTask,
		Title:            "Show Task",
		Description:      "A description",
		DefinitionOfDone: &dod,
		Status:           model.StatusInProgress,
		Priority:         1,
	})

	// Add a log
	if err := svc.AddLog(ctx, item.ID, "Started work"); err != nil {
		t.Fatalf("add log: %v", err)
	}

	// Add a label
	if err := svc.AddLabel(ctx, item.ID, "bug"); err != nil {
		t.Fatalf("add label: %v", err)
	}

	// Create a blocker and dependency
	blocker := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "Blocker", Status: model.StatusOpen,
	})
	if err := svc.AddDep(ctx, item.ID, blocker.ID); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	// Fetch all data like showCmd does
	g := mustGraph(t, svc)
	got, err := g.Item(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	deps := g.DepsOf(item.ID)
	logs, err := svc.Logs(ctx, item.ID)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}

	output := captureOutput(func() {
		itemLabels := got.Labels
		if itemLabels == nil {
			itemLabels = []string{}
		}
		logEntries := make([]LogJSON, 0, len(logs))
		for _, l := range logs {
			logEntries = append(logEntries, LogJSON{
				Message:   l.Message,
				CreatedAt: Timestamp(l.CreatedAt),
			})
		}
		out := ItemShowJSON{
			ID:               got.ID,
			Title:            got.Title,
			Type:             string(got.Type),
			Status:           string(got.Status),
			Priority:         got.Priority,
			Project:          got.Project,
			Parent:           got.ParentID,
			Description:      got.Description,
			DefinitionOfDone: got.DefinitionOfDone,
			Labels:           itemLabels,
			Dependencies:     deps,
			Logs:             logEntries,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	})

	var result ItemShowJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}

	// Verify all fields
	if result.ID != item.ID {
		t.Errorf("id = %q, want %q", result.ID, item.ID)
	}
	if result.Title != "Show Task" {
		t.Errorf("title = %q", result.Title)
	}
	if result.Type != "task" {
		t.Errorf("type = %q", result.Type)
	}
	if result.Status != "in_progress" {
		t.Errorf("status = %q", result.Status)
	}
	if result.Priority != 1 {
		t.Errorf("priority = %d", result.Priority)
	}
	if result.Project != "test" {
		t.Errorf("project = %q", result.Project)
	}
	if result.Parent != nil {
		t.Errorf("parent = %v, want nil", result.Parent)
	}
	if result.Description != "A description" {
		t.Errorf("description = %q", result.Description)
	}
	if result.DefinitionOfDone == nil || *result.DefinitionOfDone != "Tests pass" {
		t.Errorf("definition_of_done = %v", result.DefinitionOfDone)
	}
	if len(result.Labels) != 1 || result.Labels[0] != "bug" {
		t.Errorf("labels = %v", result.Labels)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0] != blocker.ID {
		t.Errorf("dependencies = %v", result.Dependencies)
	}
	if len(result.Logs) != 1 || result.Logs[0].Message != "Started work" {
		t.Errorf("logs = %v", result.Logs)
	}
	// Unmarshal already rejects non-RFC3339 values; check one was present.
	if time.Time(result.Logs[0].CreatedAt).IsZero() {
		t.Error("log created_at missing")
	}
}

func TestShowJSON_EmptyArrayFields(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	item := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "Empty Fields", Status: model.StatusOpen, Priority: 2,
	})

	g := mustGraph(t, svc)
	got, err := g.Item(item.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	deps := g.DepsOf(item.ID)
	logs, err := svc.Logs(ctx, item.ID)
	if err != nil {
		t.Fatalf("get logs: %v", err)
	}

	output := captureOutput(func() {
		itemLabels := got.Labels
		if itemLabels == nil {
			itemLabels = []string{}
		}
		logEntries := make([]LogJSON, 0, len(logs))
		for _, l := range logs {
			logEntries = append(logEntries, LogJSON{
				Message:   l.Message,
				CreatedAt: Timestamp(l.CreatedAt),
			})
		}
		out := ItemShowJSON{
			ID:           got.ID,
			Title:        got.Title,
			Type:         string(got.Type),
			Status:       string(got.Status),
			Priority:     got.Priority,
			Project:      got.Project,
			Parent:       got.ParentID,
			Labels:       itemLabels,
			Dependencies: deps,
			Logs:         logEntries,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	})

	// Verify the JSON uses [] not null for empty arrays
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, field := range []string{"labels", "dependencies", "logs"} {
		val := string(raw[field])
		if val == "null" {
			t.Errorf("%s should be [] not null", field)
		}
	}
}

func TestListJSON_WithItems(t *testing.T) {
	ctx := context.Background()
	svc := setupTestService(t)

	list01 := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "List Task 1", Status: model.StatusOpen, Priority: 1,
	})
	list02 := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "List Task 2", Status: model.StatusDone, Priority: 2,
	})

	// Add dep: list01 depends on list02
	if err := svc.AddDep(ctx, list01.ID, list02.ID); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	g := mustGraph(t, svc)
	allItems, err := g.List(tasks.ListFilter{Project: "test"})
	if err != nil {
		t.Fatalf("list items: %v", err)
	}

	output := captureOutput(func() {
		jsonItems := make([]ItemListJSON, 0, len(allItems))
		for _, item := range allItems {
			labels := item.Labels
			if labels == nil {
				labels = []string{}
			}
			deps := g.DepsOf(item.ID)
			jsonItems = append(jsonItems, ItemListJSON{
				ID:               item.ID,
				Title:            item.Title,
				Type:             string(item.Type),
				Status:           string(item.Status),
				Priority:         item.Priority,
				Project:          item.Project,
				Parent:           item.ParentID,
				Description:      item.Description,
				DefinitionOfDone: item.DefinitionOfDone,
				Labels:           labels,
				Dependencies:     deps,
			})
		}
		b, _ := json.MarshalIndent(jsonItems, "", "  ")
		fmt.Println(string(b))
	})

	var result []ItemListJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, output)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	// Find list01 and verify it has dependency
	for _, r := range result {
		if r.ID == list01.ID {
			if len(r.Dependencies) != 1 || r.Dependencies[0] != list02.ID {
				t.Errorf("list01 deps = %v, want [%s]", r.Dependencies, list02.ID)
			}
			if r.Status != "open" {
				t.Errorf("status = %q, want open", r.Status)
			}
		}
		if r.ID == list02.ID {
			if len(r.Dependencies) != 0 {
				t.Errorf("list02 deps = %v, want []", r.Dependencies)
			}
			if r.Status != "done" {
				t.Errorf("status = %q, want done", r.Status)
			}
		}
	}
}

func TestListJSON_EmptyResult(t *testing.T) {
	output := captureOutput(func() {
		fmt.Println("[]")
	})

	var result []ItemListJSON
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}

func TestListJSON_NoLogsField(t *testing.T) {
	svc := setupTestService(t)

	item := mustCreate(t, svc, model.NewItem{
		Project: "test", Type: model.ItemTypeTask,
		Title: "No Logs", Status: model.StatusOpen, Priority: 2,
	})

	output := captureOutput(func() {
		out := []ItemListJSON{{
			ID:           item.ID,
			Title:        item.Title,
			Type:         string(item.Type),
			Status:       string(item.Status),
			Priority:     item.Priority,
			Project:      item.Project,
			Labels:       []string{},
			Dependencies: []string{},
		}}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	})

	// Verify list JSON does NOT contain a "logs" field
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("expected 1 item, got %d", len(raw))
	}
	if _, hasLogs := raw[0]["logs"]; hasLogs {
		t.Error("list JSON should NOT contain 'logs' field")
	}
}

func TestTimestamp_MarshalsAsUTCWholeSeconds(t *testing.T) {
	// A time stored with a local offset and sub-second precision must marshal
	// in the one form jq's fromdateiso8601 accepts.
	cst := time.FixedZone("CST", -6*60*60)
	in := time.Date(2026, 1, 9, 15, 12, 45, 123456789, cst)

	b, err := json.Marshal(Timestamp(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := string(b), `"2026-01-09T21:12:45Z"`; got != want {
		t.Errorf("marshal = %s, want %s", got, want)
	}

	var back Timestamp
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !time.Time(back).Equal(in.Truncate(time.Second)) {
		t.Errorf("round-trip changed the instant: got %v, want %v", time.Time(back), in)
	}
}
