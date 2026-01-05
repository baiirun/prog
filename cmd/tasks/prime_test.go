package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baiirun/dotworld-tasks/internal/db"
	"github.com/baiirun/dotworld-tasks/internal/model"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintPrimeContent_NilReport(t *testing.T) {
	output := captureOutput(func() {
		printPrimeContent(nil)
	})

	// Should contain core sections
	if !strings.Contains(output, "# Tasks CLI Context") {
		t.Error("missing header")
	}
	if !strings.Contains(output, "SESSION CLOSE PROTOCOL") {
		t.Error("missing session close protocol")
	}
	if !strings.Contains(output, "Core Rules") {
		t.Error("missing core rules")
	}
	if !strings.Contains(output, "Essential Commands") {
		t.Error("missing essential commands")
	}
	if !strings.Contains(output, "No database connection") {
		t.Error("should indicate no database connection")
	}
}

func TestPrintPrimeContent_WithReport(t *testing.T) {
	report := &db.StatusReport{
		Project:    "test",
		Open:       5,
		InProgress: 2,
		Blocked:    1,
		Done:       3,
		Canceled:   1,
		Ready:      4,
		InProgItems: []model.Item{
			{ID: "ts-111111", Title: "Working on this"},
		},
		BlockedItems: []model.Item{
			{ID: "ts-222222", Title: "Stuck here"},
		},
	}

	output := captureOutput(func() {
		printPrimeContent(report)
	})

	// Should contain status counts
	if !strings.Contains(output, "5 open") {
		t.Error("missing open count")
	}
	if !strings.Contains(output, "2 in progress") {
		t.Error("missing in progress count")
	}
	if !strings.Contains(output, "1 blocked") {
		t.Error("missing blocked count")
	}

	// Should contain in-progress items
	if !strings.Contains(output, "ts-111111") {
		t.Error("missing in-progress item ID")
	}
	if !strings.Contains(output, "Working on this") {
		t.Error("missing in-progress item title")
	}

	// Should contain blocked items
	if !strings.Contains(output, "ts-222222") {
		t.Error("missing blocked item ID")
	}
	if !strings.Contains(output, "Stuck here") {
		t.Error("missing blocked item title")
	}
}

func TestPrintPrimeContent_EmptyReport(t *testing.T) {
	report := &db.StatusReport{
		Project:      "",
		Open:         0,
		InProgress:   0,
		Blocked:      0,
		Done:         0,
		Canceled:     0,
		Ready:        0,
		InProgItems:  []model.Item{},
		BlockedItems: []model.Item{},
	}

	output := captureOutput(func() {
		printPrimeContent(report)
	})

	// Should contain zeroes
	if !strings.Contains(output, "0 open") {
		t.Error("should show 0 open")
	}

	// Should NOT contain "In progress:" section when empty
	if strings.Contains(output, "In progress:\n  [") {
		t.Error("should not show in-progress section when empty")
	}
}

func TestPrintPrimeContent_MandatoryLanguage(t *testing.T) {
	output := captureOutput(func() {
		printPrimeContent(nil)
	})

	// Should contain strong MUST/NEVER language
	if !strings.Contains(output, "MUST") {
		t.Error("should contain MUST for mandatory actions")
	}
	if !strings.Contains(output, "NEVER") {
		t.Error("should contain NEVER for prohibited actions")
	}
}

func TestPrintPrimeContent_EssentialCommands(t *testing.T) {
	output := captureOutput(func() {
		printPrimeContent(nil)
	})

	// Should contain key commands
	commands := []string{
		"tasks status",
		"tasks ready",
		"tasks show",
		"tasks start",
		"tasks log",
		"tasks done",
		"tasks cancel",
		"tasks block",
		"tasks add",
	}

	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("missing command: %s", cmd)
		}
	}
}

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	if err := database.Init(); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	t.Cleanup(func() { database.Close() })
	return database
}

func TestPrimeCommand_Integration(t *testing.T) {
	database := setupTestDB(t)

	// Create some test data
	item := &model.Item{
		ID:        "ts-test01",
		Project:   "test",
		Type:      model.ItemTypeTask,
		Title:     "Integration test task",
		Status:    model.StatusInProgress,
		Priority:  1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.CreateItem(item); err != nil {
		t.Fatalf("failed to create item: %v", err)
	}

	report, err := database.ProjectStatus("")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}

	output := captureOutput(func() {
		printPrimeContent(report)
	})

	// Should contain the test task
	if !strings.Contains(output, "Integration test task") {
		t.Error("should contain integration test task in output")
	}
	if !strings.Contains(output, "1 in progress") {
		t.Error("should show 1 in progress")
	}
}
