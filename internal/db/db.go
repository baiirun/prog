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

const schema = `
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

// DB wraps a SQL database connection with task-specific operations.
type DB struct {
	*sql.DB
}

// DefaultPath returns the default database path (~/.prog/prog.db)
func DefaultPath() (string, error) {
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

	return &DB{db}, nil
}

// Init creates the schema and migrates existing data.
func (db *DB) Init() error {
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Migrate existing projects from items table
	if err := db.migrateProjects(); err != nil {
		return fmt.Errorf("failed to migrate projects: %w", err)
	}

	return nil
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
