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
	// MaxBackups is the maximum number of backups to keep
	MaxBackups = 10
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

	// Generate timestamped filename
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	backupFile := filepath.Join(backupDir, fmt.Sprintf("prog-%s.db", timestamp))

	// Use SQLite's backup via VACUUM INTO for a consistent snapshot
	_, err = db.Exec(fmt.Sprintf("VACUUM INTO '%s'", backupFile))
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Prune old backups
	if err := pruneBackups(backupDir, MaxBackups); err != nil {
		// Log but don't fail the backup
		fmt.Fprintf(os.Stderr, "warning: failed to prune old backups: %v\n", err)
	}

	return backupFile, nil
}

// BackupQuiet creates a backup without printing any output.
// Errors are silently ignored.
func (db *DB) BackupQuiet() {
	_, _ = db.Backup()
}

// ListBackups returns a list of available backup files, newest first.
func ListBackups() ([]BackupInfo, error) {
	backupDir, err := BackupPath()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "prog-") || !strings.HasSuffix(name, ".db") {
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

	// Sort newest first
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

// pruneBackups removes old backups, keeping only the newest 'keep' backups.
func pruneBackups(backupDir string, keep int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	// Filter to only backup files
	type backupFile struct {
		name    string
		modTime time.Time
	}
	var backups []backupFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "prog-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupFile{name: name, modTime: info.ModTime()})
	}

	// Sort oldest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.Before(backups[j].modTime)
	})

	// Remove oldest backups if we have more than 'keep'
	toRemove := len(backups) - keep
	if toRemove <= 0 {
		return nil
	}

	for i := 0; i < toRemove; i++ {
		path := filepath.Join(backupDir, backups[i].name)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", backups[i].name, err)
		}
	}

	return nil
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
