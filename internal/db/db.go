// Package db provides SQLite database operations for the prog task system.
//
// The database is stored at ~/.prog/prog.db by default.
// Use Open() to connect and Init() to create the schema.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SchemaVersion is the current schema version.
// Increment this when adding new migrations.
const SchemaVersion = 3

// baseSchema is the original schema (version 1).
// New tables should be added via migrations, not here.
const baseSchema = `
CREATE TABLE IF NOT EXISTS items (
	id TEXT PRIMARY KEY,
	project TEXT NOT NULL,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT,
	status TEXT NOT NULL DEFAULT 'open',
	priority INTEGER DEFAULT 2,
	parent_id TEXT REFERENCES items(id),
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deps (
	item_id TEXT REFERENCES items(id),
	depends_on TEXT REFERENCES items(id),
	PRIMARY KEY (item_id, depends_on)
);

CREATE TABLE IF NOT EXISTS logs (
	id INTEGER PRIMARY KEY,
	item_id TEXT REFERENCES items(id),
	message TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS projects (
	name TEXT PRIMARY KEY,
	description TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS concepts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	project TEXT NOT NULL,
	summary TEXT,
	last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (name, project)
);

CREATE TABLE IF NOT EXISTS learnings (
	id TEXT PRIMARY KEY,
	project TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	task_id TEXT REFERENCES items(id),
	summary TEXT NOT NULL,
	detail TEXT,
	files TEXT,
	status TEXT DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS learning_concepts (
	learning_id TEXT REFERENCES learnings(id),
	concept_id TEXT REFERENCES concepts(id),
	PRIMARY KEY (learning_id, concept_id)
);

CREATE VIRTUAL TABLE IF NOT EXISTS learnings_fts USING fts5(
	summary,
	detail,
	content='learnings',
	content_rowid='rowid'
);

CREATE TRIGGER IF NOT EXISTS learnings_ai AFTER INSERT ON learnings BEGIN
	INSERT INTO learnings_fts(rowid, summary, detail)
	VALUES (NEW.rowid, NEW.summary, NEW.detail);
END;

CREATE TRIGGER IF NOT EXISTS learnings_ad AFTER DELETE ON learnings BEGIN
	INSERT INTO learnings_fts(learnings_fts, rowid, summary, detail)
	VALUES ('delete', OLD.rowid, OLD.summary, OLD.detail);
END;

CREATE TRIGGER IF NOT EXISTS learnings_au AFTER UPDATE ON learnings BEGIN
	INSERT INTO learnings_fts(learnings_fts, rowid, summary, detail)
	VALUES ('delete', OLD.rowid, OLD.summary, OLD.detail);
	INSERT INTO learnings_fts(rowid, summary, detail)
	VALUES (NEW.rowid, NEW.summary, NEW.detail);
END;

CREATE INDEX IF NOT EXISTS idx_items_project ON items(project);
CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);
CREATE INDEX IF NOT EXISTS idx_items_parent ON items(parent_id);
CREATE INDEX IF NOT EXISTS idx_logs_item ON logs(item_id);
CREATE INDEX IF NOT EXISTS idx_learnings_project ON learnings(project);
CREATE INDEX IF NOT EXISTS idx_learnings_task ON learnings(task_id);
CREATE INDEX IF NOT EXISTS idx_learnings_status ON learnings(status);
CREATE INDEX IF NOT EXISTS idx_learning_concepts_concept ON learning_concepts(concept_id);
`

// migrations defines incremental schema changes.
// Each migration upgrades from version N-1 to N.
// Index 0 is migration to version 2, index 1 is migration to version 3, etc.
var migrations = []string{
	// Version 2: Add labels system
	`
CREATE TABLE IF NOT EXISTS labels (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	project TEXT NOT NULL,
	color TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (name, project)
);

CREATE TABLE IF NOT EXISTS item_labels (
	item_id TEXT REFERENCES items(id),
	label_id TEXT REFERENCES labels(id),
	PRIMARY KEY (item_id, label_id)
);

CREATE INDEX IF NOT EXISTS idx_labels_project ON labels(project);
CREATE INDEX IF NOT EXISTS idx_item_labels_item ON item_labels(item_id);
CREATE INDEX IF NOT EXISTS idx_item_labels_label ON item_labels(label_id);
`,
	// Version 3: Add definition_of_done to items
	`
ALTER TABLE items ADD COLUMN definition_of_done TEXT;
`,
}

// DB wraps a SQL database connection with task-specific operations.
type DB struct {
	*sql.DB
}

// DefaultPath returns the default database path (~/.prog/prog.db)
// Can be overridden with PROG_DB environment variable.
func DefaultPath() (string, error) {
	if envPath := os.Getenv("PROG_DB"); envPath != "" {
		return envPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".prog", "prog.db"), nil
}

// Open opens or creates the database at the given path
func Open(path string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrency (allows concurrent readers during writes)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Wait up to 500ms on lock contention before returning SQLITE_BUSY
	if _, err := db.Exec("PRAGMA busy_timeout=500"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	return &DB{db}, nil
}

// Init creates the schema for a fresh database.
// For existing databases, use Migrate() instead.
func (db *DB) Init() error {
	_, err := db.Exec(baseSchema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Run all migrations to bring to current version
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Migrate existing projects from items table
	if err := db.migrateProjects(); err != nil {
		return fmt.Errorf("failed to migrate projects: %w", err)
	}

	return nil
}

// Migrate runs any pending schema migrations.
// Safe to call on every startup - only runs migrations newer than current version.
func (db *DB) Migrate() error {
	currentVersion, err := db.getSchemaVersion()
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	// If version is 0 but tables exist, this is a legacy database (v1)
	if currentVersion == 0 {
		exists, err := db.tableExists("items")
		if err != nil {
			return fmt.Errorf("failed to check tables: %w", err)
		}
		if exists {
			currentVersion = 1
			if err := db.setSchemaVersion(1); err != nil {
				return fmt.Errorf("failed to set legacy version: %w", err)
			}
		}
	}

	// Run pending migrations
	for i, migration := range migrations {
		targetVersion := i + 2 // migrations[0] upgrades to v2
		if currentVersion >= targetVersion {
			continue
		}

		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration to v%d failed: %w", targetVersion, err)
		}

		if err := db.setSchemaVersion(targetVersion); err != nil {
			return fmt.Errorf("failed to update version to %d: %w", targetVersion, err)
		}
		currentVersion = targetVersion
	}

	return nil
}

// getSchemaVersion returns the current schema version using PRAGMA user_version.
func (db *DB) getSchemaVersion() (int, error) {
	var version int
	err := db.QueryRow("PRAGMA user_version").Scan(&version)
	return version, err
}

// setSchemaVersion sets the schema version using PRAGMA user_version.
func (db *DB) setSchemaVersion(version int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version))
	return err
}

// tableExists checks if a table exists in the database.
func (db *DB) tableExists(name string) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
		name,
	).Scan(&count)
	return count > 0, err
}

// migrateProjects populates the projects table from existing items.
func (db *DB) migrateProjects() error {
	_, err := db.Exec(`
		INSERT OR IGNORE INTO projects (name, created_at, updated_at)
		SELECT DISTINCT project, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		FROM items
		WHERE project != ''
	`)
	return err
}
