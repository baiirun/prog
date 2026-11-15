package main

import (
	"testing"

	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

// Protects the flag-to-filter mapping (e.g. --has-blockers must not select
// items without blockers). Rejected input is covered end to end in
// TestList_RejectsInvalidFlags.
func TestListFlagsParse(t *testing.T) {
	tests := []struct {
		name  string
		flags listFlags
		check func(tasks.ListFilter) bool
	}{
		{"empty", listFlags{}, func(f tasks.ListFilter) bool {
			return f.Status == nil && f.Type == "" && f.Blockers == tasks.AnyBlockers
		}},
		{"status", listFlags{status: "reviewing"}, func(f tasks.ListFilter) bool {
			return f.Status != nil && *f.Status == model.StatusReviewing
		}},
		{"type", listFlags{itemType: "epic"}, func(f tasks.ListFilter) bool { return f.Type == model.ItemTypeEpic }},
		{"has blockers", listFlags{hasBlockers: true}, func(f tasks.ListFilter) bool {
			return f.Blockers == tasks.WithUnresolvedBlockers
		}},
		{"no blockers", listFlags{noBlockers: true}, func(f tasks.ListFilter) bool {
			return f.Blockers == tasks.WithoutUnresolvedBlockers
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := tt.flags.parse()
			if err != nil {
				t.Fatalf("parse(%+v): %v", tt.flags, err)
			}
			if !tt.check(f) {
				t.Errorf("parse(%+v) = %+v", tt.flags, f)
			}
		})
	}
}
