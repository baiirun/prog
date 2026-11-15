package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestPrintPrimeContent_NilReport(t *testing.T) {
	output := captureOutput(func() {
		printPrimeContent(nil)
	})

	// Should contain core sections
	if !strings.Contains(output, "# Prog CLI Context") {
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
	report := &tasks.StatusReport{
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

	// Should prompt for ready command
	if !strings.Contains(output, "Run 'prog ready [-p project]'") {
		t.Error("missing ready command prompt")
	}
}

func TestPrintPrimeContent_EmptyReport(t *testing.T) {
	report := &tasks.StatusReport{
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

	// Should NOT contain "In progress:" section when empty
	if strings.Contains(output, "In progress:\n  [") {
		t.Error("should not show in-progress section when empty")
	}

	// Should prompt for ready command
	if !strings.Contains(output, "Run 'prog ready [-p project]'") {
		t.Error("should prompt to run prog ready")
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
		"prog status",
		"prog ready",
		"prog show",
		"prog start",
		"prog log",
		"prog done",
		"prog open",
		"prog add",
		"prog append",
	}

	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("missing command: %s", cmd)
		}
	}
}

func TestPrintPrimeContent_NoLearnings(t *testing.T) {
	output := captureOutput(func() {
		printPrimeContent(nil)
	})

	if !strings.Contains(output, "Starting Work") {
		t.Error("missing Starting Work section")
	}

	// Learnings were removed; prime must not point agents at commands that no
	// longer exist.
	for _, gone := range []string{"prog learn", "prog context", "prog concepts", "reflection"} {
		if strings.Contains(output, gone) {
			t.Errorf("prime still mentions %q", gone)
		}
	}
}
