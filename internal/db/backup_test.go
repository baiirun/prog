package db

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// Protects the retention policy: a burst of agent mutations must not push
// out the only backups from earlier days.
func TestBackupsToPrune_KeepsRecentAndOnePerDay(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local)
	var backups []BackupInfo // newest first, as ListBackups returns
	add := func(name string, at time.Time) {
		backups = append(backups, BackupInfo{Name: name, ModTime: at})
	}
	for i := range 30 { // a burst today, one minute apart
		add(fmt.Sprintf("burst-%02d", i), now.Add(-time.Duration(i)*time.Minute))
	}
	for day := 1; day <= 20; day++ { // two backups a day for the previous 20 days
		d := now.AddDate(0, 0, -day)
		add(fmt.Sprintf("day-%02d-late", day), d.Add(2*time.Hour))
		add(fmt.Sprintf("day-%02d-early", day), d.Add(-2*time.Hour))
	}

	pruned := map[string]bool{}
	for _, b := range backupsToPrune(backups, now) {
		pruned[b.Name] = true
	}
	var kept []string
	for _, b := range backups {
		if !pruned[b.Name] {
			kept = append(kept, b.Name)
		}
	}

	want := []string{}
	for i := range keepRecentBackups {
		want = append(want, fmt.Sprintf("burst-%02d", i))
	}
	for day := 1; day < keepDailyBackupsDays; day++ {
		want = append(want, fmt.Sprintf("day-%02d-late", day))
	}
	if !slices.Equal(kept, want) {
		t.Errorf("kept %d backups:\n %v\nwant %d:\n %v", len(kept), kept, len(want), want)
	}
}

// Protects backups for back-to-back mutations: VACUUM INTO refuses to
// overwrite, so two backups in quick succession (an agent's `add` then
// `done`) need distinct files rather than relying on clock precision.
func TestBackup_BackToBack(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db := setupTestDB(t)
	seen := map[string]bool{}
	for range 5 {
		path, err := db.Backup()
		if err != nil {
			t.Fatalf("back-to-back backup: %v", err)
		}
		if seen[path] {
			t.Fatalf("two backups wrote %s", path)
		}
		seen[path] = true
	}
}
