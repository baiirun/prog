package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// Retention: the newest keepRecentBackups, plus the newest backup of each
	// of the last keepDailyBackupsDays calendar days (today included), so a
	// burst of agent mutations cannot push out every backup from last week.
	keepRecentBackups    = 20
	keepDailyBackupsDays = 14
	// BackupDir is the subdirectory for backups within the prog directory
	BackupDir = "backups"
)

// BackupPath returns the path to the backups directory
func BackupPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".prog", BackupDir), nil
}

// Backup creates a backup of the database.
// Returns the path to the backup file.
func (db *DB) Backup() (string, error) {
	backupDir, err := BackupPath()
	if err != nil {
		return "", err
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupFile, err := reserveBackupName(backupDir, time.Now())
	if err != nil {
		return "", err
	}
	if err := db.BackupTo(backupFile); err != nil {
		return "", err
	}

	if err := pruneBackups(backupDir, time.Now()); err != nil {
		// Log but don't fail the backup
		fmt.Fprintf(os.Stderr, "warning: failed to prune old backups: %v\n", err)
	}

	return backupFile, nil
}

// reserveBackupName returns an unused backup path. VACUUM INTO refuses to
// overwrite, so names must be unique even for backups taken in the same
// instant by different processes: os.CreateTemp picks the name atomically,
// and the empty placeholder is removed so VACUUM INTO can create the file.
func reserveBackupName(dir string, now time.Time) (string, error) {
	f, err := os.CreateTemp(dir, "prog-"+now.Format("2006-01-02T15-04-05")+"-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to reserve backup name: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return "", fmt.Errorf("failed to reserve backup name: %w", err)
	}
	return name, nil
}

// BackupTo writes a consistent snapshot of the database to path using
// VACUUM INTO. The path is bound as a parameter, so quotes in it are safe.
func (db *DB) BackupTo(path string) error {
	if _, err := db.conn.Exec(`VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("failed to create backup at %s: %w", path, err)
	}
	return nil
}

// Snapshot implements tasks.Snapshotter.
func (db *DB) Snapshot() error {
	_, err := db.Backup()
	return err
}

// BackupQuiet creates a backup after a mutation. A failed backup must not
// fail the command that already succeeded, so it is reported on stderr.
func (db *DB) BackupQuiet() {
	if _, err := db.Backup(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: automatic backup failed: %v\n", err)
	}
}

// ListBackups returns a list of available backup files, newest first.
func ListBackups() ([]BackupInfo, error) {
	backupDir, err := BackupPath()
	if err != nil {
		return nil, err
	}
	return listBackupsIn(backupDir)
}

func listBackupsIn(backupDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "prog-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Path:    filepath.Join(backupDir, name),
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})
	return backups, nil
}

// BackupInfo contains information about a backup file
type BackupInfo struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// pruneBackups removes the backups the retention policy does not keep.
func pruneBackups(backupDir string, now time.Time) error {
	backups, err := listBackupsIn(backupDir)
	if err != nil {
		return err
	}
	for _, b := range backupsToPrune(backups, now) {
		if err := os.Remove(b.Path); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", b.Name, err)
		}
	}
	return nil
}

// backupsToPrune applies the retention policy to backups sorted newest first
// and returns the ones to delete.
func backupsToPrune(backups []BackupInfo, now time.Time) []BackupInfo {
	oldestDay := startOfDay(now).AddDate(0, 0, -(keepDailyBackupsDays - 1))
	keptDay := map[time.Time]bool{}
	var prune []BackupInfo
	for i, b := range backups {
		day := startOfDay(b.ModTime)
		switch {
		case i < keepRecentBackups:
			keptDay[day] = true
		case !day.Before(oldestDay) && !keptDay[day]:
			keptDay[day] = true
		default:
			prune = append(prune, b)
		}
	}
	return prune
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Restore copies a backup file to the main database location.
// The database connection should be closed before calling this.
func Restore(backupPath string) error {
	dbPath, err := DefaultPath()
	if err != nil {
		return err
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Read backup
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	// Write to main database
	if err := os.WriteFile(dbPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write database: %w", err)
	}

	return nil
}
