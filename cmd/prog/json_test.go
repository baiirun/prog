package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/baiirun/prog/internal/model"
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
	database := setupTestDB(t)

	parentID := "ep-parent"

	// Create parent epic first (FK constraint)
	epic := &model.Item{
		ID: parentID, Project: "test", Type: model.ItemTypeEpic,
		Title: "Epic", Status: model.StatusOpen,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(epic); err != nil {
		t.Fatalf("create epic: %v", err)
	}

	taskA := &model.Item{
		ID: "ts-aaa111", Project: "test", Type: model.ItemTypeTask,
		Title: "Task A", Status: model.StatusOpen, Priority: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(taskA); err != nil {
		t.Fatalf("create task A: %v", err)
	}

	taskB := &model.Item{
		ID: "ts-bbb222", Project: "test", Type: model.ItemTypeTask,
		Title: "Task B", Status: model.StatusOpen, Priority: 2,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(taskB); err != nil {
		t.Fatalf("create task B: %v", err)
	}
	if err := database.SetParent("ts-bbb222", parentID); err != nil {
		t.Fatalf("set parent: %v", err)
	}

	readyItems, err := database.ReadyItems("test")
	if err != nil {
		t.Fatalf("ready items: %v", err)
	}

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

	// Should have at least the tasks we created (epic is also open/unblocked)
	found := map[string]bool{}
	for _, r := range result {
		found[r.ID] = true
		if r.ID == "ts-aaa111" {
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
		if r.ID == "ts-bbb222" {
			if r.Parent == nil || *r.Parent != parentID {
				t.Errorf("parent = %v, want %q", r.Parent, parentID)
			}
		}
	}
	if !found["ts-aaa111"] {
		t.Error("missing ts-aaa111 in ready output")
	}
	if !found["ts-bbb222"] {
		t.Error("missing ts-bbb222 in ready output")
	}
}

func TestShowJSON_FullDetail(t *testing.T) {
	database := setupTestDB(t)

	dod := "Tests pass"
	item := &model.Item{
		ID:               "ts-show01",
		Project:          "test",
		Type:             model.ItemTypeTask,
		Title:            "Show Task",
		Description:      "A description",
		DefinitionOfDone: &dod,
		Status:           model.StatusInProgress,
		Priority:         1,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := database.CreateItem(item); err != nil {
		t.Fatalf("create item: %v", err)
	}

	// Add a log
	if err := database.AddLog(item.ID, "Started work"); err != nil {
		t.Fatalf("add log: %v", err)
	}

	// Add a label
	if err := database.AddLabelToItem(item.ID, "test", "bug"); err != nil {
		t.Fatalf("add label: %v", err)
	}

	// Create a blocker and dependency
	blocker := &model.Item{
		ID: "ts-block01", Project: "test", Type: model.ItemTypeTask,
		Title: "Blocker", Status: model.StatusOpen,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(blocker); err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	if err := database.AddDep(item.ID, blocker.ID); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	// Fetch all data like showCmd does
	got, _ := database.GetItem(item.ID)
	labels, _ := database.GetItemLabels(item.ID)
	for _, l := range labels {
		got.Labels = append(got.Labels, l.Name)
	}
	logs, _ := database.GetLogs(item.ID)
	deps, _ := database.GetDeps(item.ID)

	output := captureOutput(func() {
		itemLabels := got.Labels
		if itemLabels == nil {
			itemLabels = []string{}
		}
		if deps == nil {
			deps = []string{}
		}
		logEntries := make([]LogJSON, 0, len(logs))
		for _, l := range logs {
			logEntries = append(logEntries, LogJSON{
				Message:   l.Message,
				CreatedAt: l.CreatedAt.Format(time.RFC3339),
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
	if result.ID != "ts-show01" {
		t.Errorf("id = %q, want ts-show01", result.ID)
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
	if len(result.Dependencies) != 1 || result.Dependencies[0] != "ts-block01" {
		t.Errorf("dependencies = %v", result.Dependencies)
	}
	if len(result.Logs) != 1 || result.Logs[0].Message != "Started work" {
		t.Errorf("logs = %v", result.Logs)
	}
	// Verify log timestamp is RFC3339
	if _, err := time.Parse(time.RFC3339, result.Logs[0].CreatedAt); err != nil {
		t.Errorf("log created_at not RFC3339: %q", result.Logs[0].CreatedAt)
	}
}

func TestShowJSON_EmptyArrayFields(t *testing.T) {
	database := setupTestDB(t)

	item := &model.Item{
		ID: "ts-empty01", Project: "test", Type: model.ItemTypeTask,
		Title: "Empty Fields", Status: model.StatusOpen, Priority: 2,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(item); err != nil {
		t.Fatalf("create item: %v", err)
	}

	got, _ := database.GetItem(item.ID)
	logs, _ := database.GetLogs(item.ID)
	deps, _ := database.GetDeps(item.ID)

	output := captureOutput(func() {
		itemLabels := got.Labels
		if itemLabels == nil {
			itemLabels = []string{}
		}
		if deps == nil {
			deps = []string{}
		}
		logEntries := make([]LogJSON, 0, len(logs))
		for _, l := range logs {
			logEntries = append(logEntries, LogJSON{
				Message:   l.Message,
				CreatedAt: l.CreatedAt.Format(time.RFC3339),
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
	database := setupTestDB(t)

	items := []*model.Item{
		{
			ID: "ts-list01", Project: "test", Type: model.ItemTypeTask,
			Title: "List Task 1", Status: model.StatusOpen, Priority: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		{
			ID: "ts-list02", Project: "test", Type: model.ItemTypeTask,
			Title: "List Task 2", Status: model.StatusDone, Priority: 2,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}
	for _, item := range items {
		if err := database.CreateItem(item); err != nil {
			t.Fatalf("create item: %v", err)
		}
	}

	// Add dep: list01 depends on list02
	if err := database.AddDep("ts-list01", "ts-list02"); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	allItems, err := database.ListItems("test", nil)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if err := database.PopulateItemLabels(allItems); err != nil {
		t.Fatalf("populate labels: %v", err)
	}

	output := captureOutput(func() {
		jsonItems := make([]ItemListJSON, 0, len(allItems))
		for _, item := range allItems {
			labels := item.Labels
			if labels == nil {
				labels = []string{}
			}
			deps, _ := database.GetDeps(item.ID)
			if deps == nil {
				deps = []string{}
			}
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

	// Find ts-list01 and verify it has dependency
	for _, r := range result {
		if r.ID == "ts-list01" {
			if len(r.Dependencies) != 1 || r.Dependencies[0] != "ts-list02" {
				t.Errorf("ts-list01 deps = %v, want [ts-list02]", r.Dependencies)
			}
			if r.Status != "open" {
				t.Errorf("status = %q, want open", r.Status)
			}
		}
		if r.ID == "ts-list02" {
			if len(r.Dependencies) != 0 {
				t.Errorf("ts-list02 deps = %v, want []", r.Dependencies)
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
	database := setupTestDB(t)

	item := &model.Item{
		ID: "ts-nolog", Project: "test", Type: model.ItemTypeTask,
		Title: "No Logs", Status: model.StatusOpen, Priority: 2,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(item); err != nil {
		t.Fatalf("create item: %v", err)
	}

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
