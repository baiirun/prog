package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/baiirun/prog/internal/db"
	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tui"
	"github.com/spf13/cobra"
)

// version is set by goreleaser via ldflags; update before each release
var version = "0.12.4-dev"

// ClaudeSettings represents ~/.claude/settings.json structure
type ClaudeSettings struct {
	Hooks          map[string][]HookMatcher `json:"hooks,omitempty"`
	EnabledPlugins map[string]bool          `json:"enabledPlugins,omitempty"`
}

// HookMatcher represents a hook configuration with a matcher pattern
type HookMatcher struct {
	Matcher string `json:"matcher"`
	Hooks   []Hook `json:"hooks"`
}

// Hook represents a single hook command
type Hook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

var (
	flagProject          string
	flagStatus           string
	flagEpic             bool
	flagDraft            bool
	flagPriority         int
	flagForce            bool
	flagParent           string
	flagBlocks           string
	flagListParent       string
	flagListType         string
	flagBlocking         string
	flagBlockedBy        string
	flagHasBlockers      bool
	flagNoBlockers       bool
	flagEditTitle        string
	flagStatusAll        bool
	flagLearnConcept     []string
	flagLearnFile        []string
	flagLearnEditSummary string
	flagLearnEditDetail  string
	flagLearnStaleReason string
	flagConceptsRecent   bool
	flagConceptsRelated  string
	flagConceptsSummary  string
	flagConceptsRename   string
	flagConceptsStats    bool
	flagContextConcept   []string
	flagContextQuery     string
	flagContextStale     bool
	flagContextSummary   bool
	flagContextID        string
	flagContextJSON      bool
	flagLearnDetail      string
	flagLabelsColor      string
	flagAddLabels        []string
	flagFilterLabels     []string
	flagDoD              string
	flagDesc             string
	flagJSON             bool
)

func openDB() (*db.DB, error) {
	path, err := db.DefaultPath()
	if err != nil {
		return nil, err
	}
	database, err := db.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w (try running 'prog init' first)", err)
	}
	// Run any pending migrations
	if err := database.Migrate(); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	return database, nil
}

var rootCmd = &cobra.Command{
	Use:     "prog",
	Short:   "Lightweight task management for agents",
	Version: version,
	Long: `A CLI for managing tasks, epics, and dependencies.
Designed for AI agents to track work across sessions.

Database: ~/.prog/prog.db

Quick start:
  prog init
  prog onboard
  prog add "Build feature X" -p myproject
  prog ready -p myproject
  prog start <id>
  prog done <id>`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the prog database",
	Long:  "Creates the database at ~/.prog/prog.db if it doesn't exist.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := db.DefaultPath()
		if err != nil {
			return err
		}
		database, err := db.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.Init(); err != nil {
			return err
		}
		fmt.Printf("Initialized prog database at %s\n", path)
		fmt.Println("\nNext: run 'prog onboard' to set up Claude Code integration")
		return nil
	},
}

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Create a new task or epic",
	Long: `Create a new task (or epic with -e flag).

Returns the generated ID (ts-XXXXXX for tasks, ep-XXXXXX for epics).

Examples:
  prog add "Fix login bug" -p myproject
  prog add "Auth system" -p myproject -e
  prog add "Critical fix" --priority 1
  prog add "Subtask" --parent ep-abc123
  prog add "Dependency" --blocks ts-xyz789
  prog add "Bug fix" -p myproject -l bug -l urgent
  prog add "Feature" -p myproject -d "Detailed description here"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		itemType := model.ItemTypeTask
		if flagEpic {
			itemType = model.ItemTypeEpic
		}

		status := model.StatusOpen
		if flagDraft {
			status = model.StatusDraft
		}

		item := &model.Item{
			ID:        model.GenerateID(itemType),
			Project:   flagProject,
			Type:      itemType,
			Title:     strings.Join(args, " "),
			Status:    status,
			Priority:  flagPriority,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Set definition of done if specified
		if cmd.Flags().Changed("dod") && flagDoD != "" {
			item.DefinitionOfDone = &flagDoD
		}

		// Set description if specified
		if cmd.Flags().Changed("desc") && flagDesc != "" {
			item.Description = flagDesc
		}

		if err := database.CreateItem(item); err != nil {
			return err
		}

		// Set parent if specified
		if flagParent != "" {
			if err := database.SetParent(item.ID, flagParent); err != nil {
				return err
			}
		}

		// Add blocking relationship if specified
		if flagBlocks != "" {
			// This new item blocks the specified item
			// (the blocked item depends on this new one)
			if err := database.AddDep(flagBlocks, item.ID); err != nil {
				return err
			}
		}

		// Add labels if specified
		for _, labelName := range flagAddLabels {
			if err := database.AddLabelToItem(item.ID, item.Project, labelName); err != nil {
				return err
			}
		}

		fmt.Println(item.ID)

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long: `List all tasks, optionally filtered by various criteria.

Examples:
  prog list
  prog list -p myproject
  prog list --status open
  prog list -p myproject --status blocked
  prog list --parent ep-abc123
  prog list --type epic
  prog list --blocking ts-xyz789
  prog list --blocked-by ts-abc123
  prog list --has-blockers
  prog list --no-blockers
  prog list -l bug -l urgent
  prog list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		var status *model.Status
		if flagStatus != "" {
			s := model.Status(flagStatus)
			if !s.IsValid() {
				return fmt.Errorf("invalid status: %s (valid: draft, open, in_progress, blocked, reviewing, done, canceled)", flagStatus)
			}
			status = &s
		}

		filter := db.ListFilter{
			Project:     flagProject,
			Status:      status,
			Parent:      flagListParent,
			Type:        flagListType,
			Blocking:    flagBlocking,
			BlockedBy:   flagBlockedBy,
			HasBlockers: flagHasBlockers,
			NoBlockers:  flagNoBlockers,
			Labels:      flagFilterLabels,
		}

		items, err := database.ListItemsFiltered(filter)
		if err != nil {
			return err
		}

		// Populate labels for display
		if err := database.PopulateItemLabels(items); err != nil {
			return err
		}

		if flagJSON {
			if len(items) == 0 {
				fmt.Println("[]")
				return nil
			}
			output := make([]ItemListJSON, 0, len(items))
			for _, item := range items {
				labels := item.Labels
				if labels == nil {
					labels = []string{}
				}
				deps, err := database.GetDeps(item.ID)
				if err != nil {
					return err
				}
				if deps == nil {
					deps = []string{}
				}
				output = append(output, ItemListJSON{
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
			b, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}

		printItemsTable(items)
		return nil
	},
}

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Show tasks ready for work (unblocked)",
	Long: `Show tasks that are ready to work on.

A task is "ready" when:
  - Status is "open" (not draft, in_progress, blocked, reviewing, done, or canceled)
  - All dependencies are "done"

Results are sorted by priority (1=high first).

Examples:
  prog ready
  prog ready -p myproject
  prog ready -l bug
  prog ready --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		items, err := database.ReadyItemsFiltered(flagProject, flagFilterLabels)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			if flagJSON {
				fmt.Println("[]")
				return nil
			}
			fmt.Println("No ready tasks")
			return nil
		}

		// Populate labels for display
		if err := database.PopulateItemLabels(items); err != nil {
			return err
		}

		if flagJSON {
			output := make([]ItemReadyJSON, 0, len(items))
			for _, item := range items {
				output = append(output, ItemReadyJSON{
					ID:       item.ID,
					Title:    item.Title,
					Priority: item.Priority,
					Type:     string(item.Type),
					Parent:   item.ParentID,
				})
			}
			b, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}

		printReadyTable(items)
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Long: `Show full details for a task including description, logs, dependencies,
and suggested concepts for context retrieval.

Examples:
  prog show ts-a1b2c3
  prog show ts-a1b2c3 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		item, err := database.GetItem(args[0])
		if err != nil {
			return err
		}

		// Get labels for display
		labels, err := database.GetItemLabels(args[0])
		if err != nil {
			return err
		}
		for _, l := range labels {
			item.Labels = append(item.Labels, l.Name)
		}

		logs, err := database.GetLogs(args[0])
		if err != nil {
			return err
		}

		deps, err := database.GetDeps(args[0])
		if err != nil {
			return err
		}

		// Get related concepts for context suggestions
		concepts, err := database.GetRelatedConcepts(args[0])
		if err != nil {
			return err
		}

		if flagJSON {
			labels := item.Labels
			if labels == nil {
				labels = []string{}
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
			output := ItemShowJSON{
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
				Logs:             logEntries,
			}
			b, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal JSON: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}

		printItemDetail(item, logs, deps, concepts)
		return nil
	},
}

var startCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start working on a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateStatus(args[0], model.StatusInProgress); err != nil {
			return err
		}
		fmt.Printf("Started %s\n", args[0])
		return nil
	},
}

var doneCmd = &cobra.Command{
	Use:   "done <id>",
	Short: "Mark a task as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateStatus(args[0], model.StatusDone); err != nil {
			return err
		}
		fmt.Printf("Completed %s\n", args[0])

		// Prompt reflection
		fmt.Println(`
Reflect: What would help the next agent? (See instructions for guidance)
  prog learn "summary" -c concept --detail "explanation"`)

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var reviewCmd = &cobra.Command{
	Use:   "review <id>",
	Short: "Mark a task as reviewing (awaiting merge)",
	Long: `Mark a task as reviewing — work is complete and awaiting merge.

Use this when you've created a PR/branch and the code is ready for review,
but shouldn't be marked "done" until it lands on main.

Only tasks that are in_progress can be marked as reviewing.

Example:
  prog review ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]

		// Verify the task is in_progress before transitioning
		item, err := database.GetItem(id)
		if err != nil {
			return err
		}
		if item.Status != model.StatusInProgress {
			return fmt.Errorf("can only review in_progress tasks (current status: %s)", item.Status)
		}

		if err := database.UpdateStatus(id, model.StatusReviewing); err != nil {
			return err
		}
		fmt.Printf("Marked %s as reviewing\n", id)

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel <id> [reason]",
	Short: "Cancel a task without completing it",
	Long: `Cancel a task that is no longer relevant.

Use this instead of delete when you want to preserve the task history
but close it without marking it as successfully completed.

Example:
  prog cancel ts-a1b2c3
  prog cancel ts-a1b2c3 "Requirements changed, no longer needed"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]

		if err := database.UpdateStatus(id, model.StatusCanceled); err != nil {
			return err
		}

		if len(args) > 1 {
			reason := strings.Join(args[1:], " ")
			if err := database.AddLog(id, "Canceled: "+reason); err != nil {
				return err
			}
			fmt.Printf("Canceled %s: %s\n", id, reason)
		} else {
			fmt.Printf("Canceled %s\n", id)
		}

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var draftCmd = &cobra.Command{
	Use:   "draft <id>",
	Short: "Set a task back to draft (not yet ready for work)",
	Long: `Set a task's status to draft.

Use this to move a task back to the planning/scoping phase.
Draft tasks don't appear in 'prog ready' output.

Example:
  prog draft ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateStatus(args[0], model.StatusDraft); err != nil {
			return err
		}
		fmt.Printf("Moved %s to draft\n", args[0])

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var openCmd = &cobra.Command{
	Use:   "open <id>",
	Short: "Reopen a task (set status back to open)",
	Long: `Set a task's status back to open.

Use this to reopen a task that was previously started, completed, or canceled.

Example:
  prog open ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateStatus(args[0], model.StatusOpen); err != nil {
			return err
		}
		fmt.Printf("Reopened %s\n", args[0])

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a task or epic",
	Long: `Permanently delete a task or epic and all associated data.

This removes the item, its logs, and any dependencies.
This action cannot be undone.

Example:
  prog delete ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.DeleteItem(args[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted %s\n", args[0])
		return nil
	},
}

var logCmd = &cobra.Command{
	Use:   "log <id> <message>",
	Short: "Add a log entry to a task",
	Long: `Add a timestamped log entry to a task's audit trail.

Use this to track progress while working.

Example:
  prog log ts-a1b2c3 "Implemented token refresh logic"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]
		message := strings.Join(args[1:], " ")

		if err := database.AddLog(id, message); err != nil {
			return err
		}
		fmt.Printf("Logged to %s\n", id)
		return nil
	},
}

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show dependency graph",
	Long: `Show all task dependencies as a graph.

Displays which tasks are blocked by other tasks.

Examples:
  prog graph
  prog graph -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		edges, err := database.GetAllDeps(flagProject)
		if err != nil {
			return err
		}

		if len(edges) == 0 {
			fmt.Println("No dependencies")
			return nil
		}

		printDepGraph(edges)
		return nil
	},
}

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects",
	Long:  `List all projects that have tasks.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		projects, err := database.ListProjects()
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			fmt.Println("No projects")
			return nil
		}

		for _, p := range projects {
			fmt.Println(p)
		}
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project status overview",
	Long: `Show a summary of project status for agent spin-up.

Includes:
  - Count by status (draft, open, in_progress, blocked, reviewing, done, canceled)
  - Recently completed tasks
  - Currently in-progress tasks
  - Tasks in review (awaiting merge)
  - Blocked tasks with reasons
  - Ready tasks by priority (limited to 10 by default)

Use --all to show all ready tasks.

Examples:
  prog status
  prog status -p myproject
  prog status --all
  prog status -l bug`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		report, err := database.ProjectStatusFiltered(flagProject, flagFilterLabels)
		if err != nil {
			return err
		}

		// Populate labels for all item slices in the report
		_ = database.PopulateItemLabels(report.DraftItems)
		_ = database.PopulateItemLabels(report.RecentDone)
		_ = database.PopulateItemLabels(report.InProgItems)
		_ = database.PopulateItemLabels(report.ReviewingItems)
		_ = database.PopulateItemLabels(report.BlockedItems)
		_ = database.PopulateItemLabels(report.ReadyItems)

		printStatusReport(report, flagStatusAll)
		return nil
	},
}

var appendCmd = &cobra.Command{
	Use:   "append <id> <text>",
	Short: "Append text to a task's description",
	Long: `Append text to a task's description.

Use this to add context, decisions, or handoff notes.

Example:
  prog append ts-a1b2c3 "Decided to use JWT instead of sessions"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]
		text := strings.Join(args[1:], " ")

		if err := database.AppendDescription(id, text); err != nil {
			return err
		}
		fmt.Printf("Appended to %s\n", id)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a task's title, description, or definition of done",
	Long: `Edit a task's title, description, or definition of done.

With --title, updates the title directly without opening an editor.
With --dod, sets or clears the definition of done (use "" to clear).
Without flags, opens the description in your configured editor.

Uses $PROG_EDITOR if set, otherwise defaults to nvim, then nano, then vi.

Examples:
  prog edit ts-a1b2c3                     # Edit description in editor
  prog edit ts-a1b2c3 --title "New title" # Update title directly
  prog edit ts-a1b2c3 --dod "Tests pass"  # Set definition of done
  prog edit ts-a1b2c3 --dod ""            # Clear definition of done
  PROG_EDITOR=code prog edit ts-a1b2c3    # Use VS Code as editor`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]

		// If --dod flag is set, update definition of done
		if cmd.Flags().Changed("dod") {
			var dod *string
			if flagDoD != "" {
				dod = &flagDoD
			}
			if err := database.SetDefinitionOfDone(id, dod); err != nil {
				return err
			}
			if dod == nil {
				fmt.Printf("Cleared definition of done for %s\n", id)
			} else {
				fmt.Printf("Updated definition of done for %s\n", id)
			}
			return nil
		}

		// If --title flag is set, update title directly
		if flagEditTitle != "" {
			if err := database.SetTitle(id, flagEditTitle); err != nil {
				return err
			}
			fmt.Printf("Updated title for %s\n", id)
			return nil
		}

		// Get current description
		item, err := database.GetItem(id)
		if err != nil {
			return err
		}

		// Get editor (prefer $PROG_EDITOR, then nvim, then nano)
		editor := os.Getenv("PROG_EDITOR")
		if editor == "" {
			if _, err := exec.LookPath("nvim"); err == nil {
				editor = "nvim"
			} else if _, err := exec.LookPath("nano"); err == nil {
				editor = "nano"
			} else {
				editor = "vi"
			}
		}

		// Create temp file
		tmpfile, err := os.CreateTemp("", "prog-edit-*.md")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpfile.Name()
		defer func() { _ = os.Remove(tmpPath) }()

		// Write current description
		if _, err := tmpfile.WriteString(item.Description); err != nil {
			_ = tmpfile.Close()
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		if err := tmpfile.Close(); err != nil {
			return fmt.Errorf("failed to close temp file: %w", err)
		}

		// Get original stat for comparison
		origStat, err := os.Stat(tmpPath)
		if err != nil {
			return fmt.Errorf("failed to stat temp file: %w", err)
		}

		// Open editor
		editorCmd := execCommand(editor, tmpPath)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr
		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Check if file was modified
		newStat, err := os.Stat(tmpPath)
		if err != nil {
			return fmt.Errorf("failed to stat temp file: %w", err)
		}

		if newStat.ModTime().Equal(origStat.ModTime()) {
			fmt.Println("No changes made")
			return nil
		}

		// Read new content
		newContent, err := os.ReadFile(tmpPath)
		if err != nil {
			return fmt.Errorf("failed to read temp file: %w", err)
		}

		// Update description
		if err := database.SetDescription(id, string(newContent)); err != nil {
			return err
		}
		fmt.Printf("Updated description for %s\n", id)
		return nil
	},
}

// execCommand wraps exec.Command for testing
var execCommand = func(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

var descCmd = &cobra.Command{
	Use:   "desc <id> <text>",
	Short: "Replace a task's description",
	Long: `Replace a task's entire description with new text.

Use this when you need to rewrite or fix the description content.
For adding to existing content, use 'prog append' instead.

Example:
  prog desc ts-a1b2c3 "New description text here"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]
		text := strings.Join(args[1:], " ")

		if err := database.SetDescription(id, text); err != nil {
			return err
		}
		fmt.Printf("Updated description for %s\n", id)
		return nil
	},
}

var parentCmd = &cobra.Command{
	Use:   "parent <id> <epic-id>",
	Short: "Set a task's parent epic",
	Long: `Set the parent epic for a task.

This establishes a hierarchical relationship where tasks belong to epics.
The parent must be an epic (created with -e flag).

Example:
  prog parent ts-a1b2c3 ep-d4e5f6
  # ts-a1b2c3 is now a child of ep-d4e5f6`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.SetParent(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("%s is now under %s\n", args[0], args[1])
		return nil
	},
}

var projectCmd = &cobra.Command{
	Use:   "project <id> <project>",
	Short: "Set a task's project",
	Long: `Set or change the project for a task.

The project will be created if it doesn't exist.

Example:
  prog project ts-a1b2c3 myproject
  # ts-a1b2c3 is now in myproject`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.SetProject(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("%s is now in project %s\n", args[0], args[1])
		return nil
	},
}

var blocksCmd = &cobra.Command{
	Use:   "blocks <id> <other-id>",
	Short: "Mark a task as blocking another",
	Long: `Mark a task as blocking another task.

The second task cannot be started until the first is done.

Example:
  prog blocks ts-a1b2c3 ts-d4e5f6
  # ts-d4e5f6 cannot start until ts-a1b2c3 is done`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		// blocks A B means B depends on A (A blocks B)
		if err := database.AddDep(args[1], args[0]); err != nil {
			return err
		}
		fmt.Printf("%s now blocks %s\n", args[0], args[1])
		return nil
	},
}

var labelCmd = &cobra.Command{
	Use:   "label <item-id> <label-name>",
	Short: "Add a label to a task",
	Long: `Add a label to a task or epic.

Creates the label if it doesn't exist (like concepts).

Example:
  prog label ts-a1b2c3 bug
  prog label ts-a1b2c3 urgent`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		// Get item to find its project
		item, err := database.GetItem(args[0])
		if err != nil {
			return err
		}

		if err := database.AddLabelToItem(args[0], item.Project, args[1]); err != nil {
			return err
		}
		fmt.Printf("Added label %q to %s\n", args[1], args[0])
		return nil
	},
}

var unlabelCmd = &cobra.Command{
	Use:   "unlabel <item-id> <label-name>",
	Short: "Remove a label from a task",
	Long: `Remove a label from a task or epic.

Example:
  prog unlabel ts-a1b2c3 bug`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		// Get item to find its project
		item, err := database.GetItem(args[0])
		if err != nil {
			return err
		}

		if err := database.RemoveLabelFromItem(args[0], item.Project, args[1]); err != nil {
			return err
		}
		fmt.Printf("Removed label %q from %s\n", args[1], args[0])
		return nil
	},
}

var learnCmd = &cobra.Command{
	Use:   "learn <summary>",
	Short: "Log a learning for future context retrieval",
	Long: `Log a learning discovered during work.

Learnings capture tacit knowledge—insights that aren't obvious from code alone.
They're stored with a two-phase structure for efficient context retrieval:

  Summary (required): One-liner (≤120 chars) for quick scanning
  Detail (optional): Full explanation with context, examples, and caveats

Learnings are tagged with concepts for organized retrieval.
Concepts are created automatically if they don't exist.

If a task is in progress for the project, the learning is linked to it.

CONTEXT MANAGEMENT FLOW:
┌─────────────────────────────────────────────────────────────┐
│ 1. During Work: Discover a pattern, gotcha, or insight      │
│    → prog learn "summary" -c concept --detail "full text"   │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Future Retrieval: Load context before starting work     │
│    → prog context -c concept        # by concept           │
│    → prog context -q "search term"  # full-text search     │
│    → prog context --summary         # scan all one-liners  │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Two-Phase Retrieval:                                    │
│    Phase 1: See summaries (lightweight, fast)              │
│    Phase 2: Load full detail only when relevant            │
└─────────────────────────────────────────────────────────────┘

LEARNING STRUCTURE:
  ┌─────────────────────────────────────┐
  │ Learning (lrn-XXXXXX)                │
  │ ├─ Summary: "One-liner for scanning" │
  │ ├─ Detail: "Full context..."         │
  │ ├─ Concepts: [auth, concurrency]     │
  │ ├─ Files: [auth.go, token.go]        │
  │ ├─ Status: active/stale/archived     │
  │ └─ Task ID: ts-abc123 (optional)     │
  └─────────────────────────────────────┘

Examples:
  prog learn "Token refresh has race condition" -p myproject -c auth -c concurrency
  prog learn "Config loaded from env first" -p myproject -c config -f config.go
  prog learn "Token refresh issue" -c auth -p myproject --detail "The mutex only protects..."
  echo "multi-line detail" | prog learn "summary" -c auth -p myproject --detail -`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Validate required flags
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}
		if len(flagLearnConcept) == 0 {
			return fmt.Errorf("at least one concept is required (-c)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		project := flagProject

		// Get current in-progress task for this project
		taskID, _ := database.GetCurrentTaskID(project)

		// Handle detail from stdin
		detail := flagLearnDetail
		if detail == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			detail = strings.TrimSpace(string(data))
		}

		now := time.Now()
		learning := &model.Learning{
			ID:        model.GenerateLearningID(),
			Project:   project,
			CreatedAt: now,
			UpdatedAt: now,
			TaskID:    taskID,
			Summary:   strings.Join(args, " "),
			Detail:    detail,
			Status:    model.LearningStatusActive,
			Concepts:  flagLearnConcept,
			Files:     flagLearnFile,
		}

		if err := database.CreateLearning(learning); err != nil {
			return err
		}

		// Build output
		output := learning.ID
		if taskID != nil {
			output += fmt.Sprintf(" (linked to %s)", *taskID)
		}
		fmt.Println(output)

		// Backup after successful mutation
		database.BackupQuiet()

		return nil
	},
}

var learnEditCmd = &cobra.Command{
	Use:   "edit <learning-id>",
	Short: "Edit a learning's summary or detail",
	Long: `Edit an existing learning's summary or detail.

Examples:
  prog learn edit lrn-abc123 --summary "Updated summary"
  prog learn edit lrn-abc123 --detail "Full context explanation"
  echo "multi-line" | prog learn edit lrn-abc123 --detail -`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagLearnEditSummary == "" && flagLearnEditDetail == "" {
			return fmt.Errorf("--summary or --detail is required")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if flagLearnEditSummary != "" {
			if err := database.UpdateLearningSummary(args[0], flagLearnEditSummary); err != nil {
				return err
			}
		}

		if flagLearnEditDetail != "" {
			detail := flagLearnEditDetail
			if detail == "-" {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %w", err)
				}
				detail = strings.TrimSpace(string(data))
			}
			if err := database.UpdateLearningDetail(args[0], detail); err != nil {
				return err
			}
		}

		fmt.Printf("Updated %s\n", args[0])
		return nil
	},
}

var learnStaleCmd = &cobra.Command{
	Use:   "stale <learning-id> [learning-id...]",
	Short: "Mark learnings as stale (outdated)",
	Long: `Mark one or more learnings as stale when they're outdated but still useful for reference.

Examples:
  prog learn stale lrn-abc123 --reason "Refactored in v2"
  prog learn stale lrn-a lrn-b lrn-c --reason "Compacted into lrn-xyz"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		for _, id := range args {
			if err := database.UpdateLearningStatus(id, model.LearningStatusStale); err != nil {
				return err
			}
		}

		// Output
		if len(args) == 1 {
			if flagLearnStaleReason != "" {
				fmt.Printf("Marked %s as stale: %s\n", args[0], flagLearnStaleReason)
			} else {
				fmt.Printf("Marked %s as stale\n", args[0])
			}
		} else {
			if flagLearnStaleReason != "" {
				fmt.Printf("Marked %d learnings as stale: %s\n", len(args), flagLearnStaleReason)
			} else {
				fmt.Printf("Marked %d learnings as stale\n", len(args))
			}
		}
		return nil
	},
}

var learnRmCmd = &cobra.Command{
	Use:   "rm <learning-id>",
	Short: "Delete a learning",
	Long: `Permanently delete a learning and its concept associations.

Example:
  prog learn rm lrn-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.DeleteLearning(args[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted %s\n", args[0])
		return nil
	},
}

var conceptsCmd = &cobra.Command{
	Use:   "concepts [name]",
	Short: "List or edit concepts for a project",
	Long: `List all concepts for a project, or edit a concept.

Concepts are knowledge categories that group related learnings.
They enable organized context retrieval by topic (e.g., "auth", "database", "api").

CONCEPT MODEL:
  ┌─────────────────────────────────────────────────────────┐
  │ Concepts (con-XXXXXX)                                    │
  │ ├─ Name: "auth"                                          │
  │ ├─ Project: "myproject"                                  │
  │ ├─ Summary: "Authentication patterns and gotchas"        │
  │ └─ Learning Count: 5                                     │
  └─────────────────────────────────────────────────────────┘
           │
           │ many-to-many
           │
           ↓
  ┌─────────────────────────────────────────────────────────┐
  │ Learnings                                                │
  │ ├─ lrn-abc: "Token refresh race condition" [auth]       │
  │ ├─ lrn-def: "JWT validation edge case" [auth]           │
  │ └─ lrn-ghi: "Session timeout handling" [auth]            │
  └─────────────────────────────────────────────────────────┘

RETRIEVAL PATTERNS:
  1. By Concept: prog context -c auth
     → Returns all learnings tagged with "auth"
  
  2. By Task: prog concepts --related ts-abc123
     → Suggests concepts based on task title/description
     → Then: prog context -c <suggested-concept>
  
  3. Full-Text Search: prog context -q "token refresh"
     → Searches across all learning summaries and details

Default sort is by learning count (most used first).

Examples:
  prog concepts -p myproject                        # list concepts
  prog concepts -p myproject --recent               # sort by last updated
  prog concepts -p myproject --stats                # show count and oldest age
  prog concepts --related ts-abc123                 # suggest concepts for a task
  prog concepts fts -p myproject --summary "..."    # set concept summary
  prog concepts fts -p myproject --rename "search"  # rename concept`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		// Edit mode: concept name provided with --summary or --rename
		if len(args) > 0 && (flagConceptsSummary != "" || flagConceptsRename != "") {
			if flagProject == "" {
				return fmt.Errorf("project is required (-p)")
			}
			if flagConceptsSummary != "" {
				if err := database.SetConceptSummary(args[0], flagProject, flagConceptsSummary); err != nil {
					return err
				}
				fmt.Printf("Updated %s\n", args[0])
			}
			if flagConceptsRename != "" {
				if err := database.RenameConcept(args[0], flagConceptsRename, flagProject); err != nil {
					return err
				}
				fmt.Printf("Renamed %s -> %s\n", args[0], flagConceptsRename)
			}
			return nil
		}

		// Stats mode
		if flagConceptsStats {
			if flagProject == "" {
				return fmt.Errorf("project is required (-p)")
			}
			stats, err := database.ListConceptsWithStats(flagProject)
			if err != nil {
				return err
			}
			if len(stats) == 0 {
				fmt.Println("No concepts")
				return nil
			}
			printConceptsStats(stats)
			return nil
		}

		// List mode
		var concepts []model.Concept

		if flagConceptsRelated != "" {
			// Get concepts related to a task
			concepts, err = database.GetRelatedConcepts(flagConceptsRelated)
			if err != nil {
				return err
			}
		} else {
			// List all concepts for project
			if flagProject == "" {
				return fmt.Errorf("project is required (-p) or use --related <task-id>")
			}
			concepts, err = database.ListConcepts(flagProject, flagConceptsRecent)
			if err != nil {
				return err
			}
		}

		if len(concepts) == 0 {
			fmt.Println("No concepts")
			return nil
		}

		printConceptsTable(concepts)
		return nil
	},
}

var labelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "List or manage labels for a project",
	Long: `List all labels for a project.

Labels are tags for categorizing tasks (bug, feature, refactor, etc).
Labels are project-scoped and identified by name.

Examples:
  prog labels -p myproject           # list all labels
  prog labels add bug -p myproject   # create a label
  prog labels rm bug -p myproject    # delete a label
  prog labels rename bug critical -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		labels, err := database.ListLabels(flagProject)
		if err != nil {
			return err
		}

		if len(labels) == 0 {
			fmt.Println("No labels")
			return nil
		}

		printLabelsTable(labels)
		return nil
	},
}

var labelsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new label",
	Long: `Create a new label in a project.

Labels are created on first use when attached to items, but you can
also create them explicitly with this command.

Examples:
  prog labels add bug -p myproject
  prog labels add urgent -p myproject --color "#ff0000"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		now := time.Now()
		label := &model.Label{
			ID:        model.GenerateLabelID(),
			Name:      args[0],
			Project:   flagProject,
			Color:     flagLabelsColor,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if err := database.CreateLabel(label); err != nil {
			return err
		}
		fmt.Printf("Created label: %s\n", args[0])
		return nil
	},
}

var labelsRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Delete a label",
	Long: `Delete a label and remove it from all items.

Example:
  prog labels rm bug -p myproject`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.DeleteLabel(flagProject, args[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted label: %s\n", args[0])
		return nil
	},
}

var labelsRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a label",
	Long: `Rename a label. All items with this label will be updated.

Example:
  prog labels rename bug critical -p myproject`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.RenameLabel(flagProject, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Renamed label: %s -> %s\n", args[0], args[1])
		return nil
	},
}

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Retrieve learnings for context",
	Long: `Retrieve learnings by concept, full-text search, or specific ID.

Use this to load relevant context before starting work on a task.

Examples:
  prog context -p myproject --summary                # all learnings, grouped by concept
  prog context -c auth -c concurrency -p myproject   # by concepts
  prog context -q "rate limit" -p myproject          # full-text search
  prog context -c auth --summary -p myproject        # one-liner per learning
  prog context --id lrn-abc123                       # specific learning by ID
  prog context -c auth --include-stale -p myproject  # include stale learnings
  prog context -c auth --json -p myproject           # JSON output for agents`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		// Mode 1: Specific learning by ID
		if flagContextID != "" {
			learning, err := database.GetLearning(flagContextID)
			if err != nil {
				return err
			}
			if flagContextJSON {
				return printLearningsJSON([]model.Learning{*learning})
			}
			printLearnings([]model.Learning{*learning})
			return nil
		}

		// All other modes require project
		if flagProject == "" {
			return fmt.Errorf("project is required (-p) or use --id")
		}

		// Mode 2: All learnings with --summary (no concepts/query required)
		if flagContextSummary && len(flagContextConcept) == 0 && flagContextQuery == "" {
			learnings, err := database.GetAllLearnings(flagProject, flagContextStale)
			if err != nil {
				return err
			}

			if len(learnings) == 0 {
				if flagContextJSON {
					fmt.Println("[]")
					return nil
				}
				fmt.Println("No learnings found")
				return nil
			}

			if flagContextJSON {
				return printLearningsJSON(learnings)
			}

			// Get concept summaries for grouped output
			concepts, _ := database.ListConcepts(flagProject, false)
			conceptMap := make(map[string]string)
			for _, c := range concepts {
				conceptMap[c.Name] = c.Summary
			}
			printAllLearningSummaries(learnings, conceptMap)
			return nil
		}

		// Modes 3 & 4 require concepts or query
		if len(flagContextConcept) == 0 && flagContextQuery == "" {
			return fmt.Errorf("specify concepts (-c), query (-q), or use --summary for all")
		}

		var learnings []model.Learning

		if len(flagContextConcept) > 0 {
			learnings, err = database.GetLearningsByConcepts(flagProject, flagContextConcept, flagContextStale)
		} else {
			learnings, err = database.SearchLearnings(flagProject, flagContextQuery, flagContextStale)
		}
		if err != nil {
			return err
		}

		if len(learnings) == 0 {
			if flagContextJSON {
				fmt.Println("[]")
				return nil
			}
			fmt.Println("No learnings found")
			return nil
		}

		// JSON mode
		if flagContextJSON {
			return printLearningsJSON(learnings)
		}

		// Mode 3: Summary mode (one-liners) for specific concepts
		if flagContextSummary {
			// Get concept summaries for header
			concepts, _ := database.ListConcepts(flagProject, false)
			conceptMap := make(map[string]string)
			for _, c := range concepts {
				conceptMap[c.Name] = c.Summary
			}
			printLearningSummaries(learnings, flagContextConcept, conceptMap)
			return nil
		}

		// Mode 4: Full output
		printLearnings(learnings)
		return nil
	},
}

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Set up prog integration for AI agents",
	Long: `Set up prog integration for AI agents.

Designed for Claude Code but provides guidance for other agents.

For Claude Code, this command:
1. Writes a prog workflow snippet to CLAUDE.md in the current directory
2. Installs a SessionStart hook in ~/.claude/settings.json to auto-run 'prog prime'

For other agents (Cursor, Opencode, Droid, Codex, Gemini, etc.):
- Copy the Task Tracking snippet to your agent's instruction file
- If hooks are available, add 'prog prime' to session start
- Otherwise, run 'prog prime' and paste output into agent context

Creates files if they don't exist. Skips if already configured (use --force to update).

Example:
  cd ~/code/myproject
  prog onboard
  prog onboard --force  # Update existing configuration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOnboard(flagForce)
	},
}

func findClaudeMD() string {
	// Check for existing file with exact case match
	// (os.Stat is case-insensitive on macOS, so we use ReadDir)
	entries, err := os.ReadDir(".")
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.EqualFold(name, "claude.md") {
				return name // Return actual casing
			}
		}
	}
	// Default to uppercase if none exists
	return "CLAUDE.md"
}

func runOnboard(force bool) error {
	return runOnboardWithSettings(force, "")
}

func runOnboardWithSettings(force bool, settingsPath string) error {
	claudePath := findClaudeMD()
	snippet := `## Task Tracking

This project uses **prog** for cross-session task management.
Run ` + "`prog prime`" + ` for workflow context, or configure hooks for auto-injection.

**Quick reference:**
` + "```" + `
prog ready              # Find unblocked work
prog add "Title" -p X   # Create task
prog start <id>         # Claim work
prog log <id> "msg"     # Log progress
prog done <id>          # Complete work
` + "```" + `

For full workflow: ` + "`prog prime`" + `
`

	var claudeMDUpdated bool

	// Check if file exists
	content, err := os.ReadFile(claudePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new file
			if err := os.WriteFile(claudePath, []byte(snippet), 0644); err != nil {
				return fmt.Errorf("failed to create CLAUDE.md: %w", err)
			}
			fmt.Println("Created CLAUDE.md with prog integration")
			claudeMDUpdated = true
		} else {
			return fmt.Errorf("failed to read CLAUDE.md: %w", err)
		}
	} else {
		// Check if already onboarded
		if strings.Contains(string(content), "## Task Tracking") {
			if !force {
				fmt.Println("CLAUDE.md already has Task Tracking section")
			} else {
				// Replace existing section
				newContent := replaceTaskTrackingSection(string(content), snippet)
				if err := os.WriteFile(claudePath, []byte(newContent), 0644); err != nil {
					return fmt.Errorf("failed to update %s: %w", claudePath, err)
				}
				fmt.Printf("Updated Task Tracking section in %s\n", claudePath)
				claudeMDUpdated = true
			}
		} else {
			// Append to existing file
			newContent := string(content)
			if !strings.HasSuffix(newContent, "\n") {
				newContent += "\n"
			}
			newContent += "\n" + snippet

			if err := os.WriteFile(claudePath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("failed to update %s: %w", claudePath, err)
			}
			fmt.Printf("Added prog integration to %s\n", claudePath)
			claudeMDUpdated = true
		}
	}

	// Install SessionStart hook
	hookAdded, err := installSessionStartHook(settingsPath)
	if err != nil {
		return fmt.Errorf("failed to install hook: %w", err)
	}

	if hookAdded {
		fmt.Println("Installed SessionStart hook in ~/.claude/settings.json")
	} else {
		fmt.Println("SessionStart hook already installed")
	}

	if !claudeMDUpdated && !hookAdded && !force {
		fmt.Println("Use --force to update existing configuration")
	}

	// Print summary and guidance for other agents
	fmt.Println()
	fmt.Println("Note: This assumes Claude Code. For other agents:")
	fmt.Println("  1. Update your agent's instruction file (AGENTS.md, .cursorrules, etc.)")
	fmt.Println("     with the Task Tracking section above")
	fmt.Println("  2. If your tool supports hooks, add 'prog prime' to session start")
	fmt.Println("  3. If no hooks, run 'prog prime' and paste output into agent context")

	return nil
}

// replaceTaskTrackingSection finds the "## Task Tracking" section and replaces it
// with the new snippet. The section ends at the next heading (# or ##) or EOF.
func replaceTaskTrackingSection(content, snippet string) string {
	// Find where "## Task Tracking" starts
	startIdx := strings.Index(content, "## Task Tracking")
	if startIdx == -1 {
		return content
	}

	// Find where the section ends (next heading or EOF)
	rest := content[startIdx+len("## Task Tracking"):]
	endIdx := -1

	// Look for next heading (line starting with #)
	lines := strings.Split(rest, "\n")
	charCount := 0
	for i, line := range lines {
		if i > 0 && len(line) > 0 && line[0] == '#' {
			endIdx = startIdx + len("## Task Tracking") + charCount
			break
		}
		charCount += len(line) + 1 // +1 for newline
	}

	// Build new content
	before := content[:startIdx]
	var after string
	if endIdx == -1 {
		// Section goes to EOF
		after = ""
	} else {
		after = content[endIdx:]
	}

	// Ensure proper spacing
	result := strings.TrimRight(before, "\n")
	if result != "" {
		result += "\n\n"
	}
	result += strings.TrimRight(snippet, "\n")
	if after != "" {
		result += "\n\n" + strings.TrimLeft(after, "\n")
	} else {
		result += "\n"
	}

	return result
}

// installSessionStartHook installs "prog prime" in ~/.claude/settings.json
// Returns true if hook was added, false if already present
func installSessionStartHook(settingsPath string) (bool, error) {
	// Default to ~/.claude/settings.json if not specified
	if settingsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("failed to get home directory: %w", err)
		}
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}

	// Read existing settings or create new
	var settings ClaudeSettings
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to read settings: %w", err)
		}
		// File doesn't exist, start fresh
		settings = ClaudeSettings{
			Hooks: make(map[string][]HookMatcher),
		}
	} else {
		if err := json.Unmarshal(content, &settings); err != nil {
			return false, fmt.Errorf("failed to parse settings: %w", err)
		}
		if settings.Hooks == nil {
			settings.Hooks = make(map[string][]HookMatcher)
		}
	}

	// Check if hook already exists
	const hookCommand = "prog prime"
	for _, matcher := range settings.Hooks["SessionStart"] {
		for _, hook := range matcher.Hooks {
			if hook.Command == hookCommand {
				return false, nil // Already installed
			}
		}
	}

	// Add the hook
	newHook := Hook{
		Type:    "command",
		Command: hookCommand,
	}

	// Find or create a matcher with empty string (matches all)
	found := false
	for i, matcher := range settings.Hooks["SessionStart"] {
		if matcher.Matcher == "" {
			settings.Hooks["SessionStart"][i].Hooks = append(matcher.Hooks, newHook)
			found = true
			break
		}
	}

	if !found {
		// Create new matcher
		settings.Hooks["SessionStart"] = append(settings.Hooks["SessionStart"], HookMatcher{
			Matcher: "",
			Hooks:   []Hook{newHook},
		})
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create settings directory: %w", err)
	}

	// Write settings back
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, append(output, '\n'), 0644); err != nil {
		return false, fmt.Errorf("failed to write settings: %w", err)
	}

	return true, nil
}

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Output context for Claude Code hooks",
	Long: `Output essential workflow context for AI agents.

Designed to run on SessionStart and PreCompact hooks to ensure
agents maintain context about the prog workflow.

Example hook configuration in Claude Code settings:
  "hooks": {
    "SessionStart": [{"command": "prog prime"}],
    "PreCompact": [{"command": "prog prime"}]
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			// Still output prime content even if DB fails
			printPrimeContent(nil)
			return nil
		}
		defer func() { _ = database.Close() }()

		report, _ := database.ProjectStatus("")

		printPrimeContent(report)
		return nil
	},
}

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Output compaction workflow guidance for agents",
	Long: `Output guidance for grooming learnings and concepts.

Use this when learnings have accumulated and need review.
Covers identifying redundancy, staleness, and quality issues.

The workflow uses two phases:
1. Discovery: Scan summaries to identify grooming candidates
2. Selection: Load detail for candidates, groom, repeat

Example:
  prog compact              # Output compaction guidance
  prog compact -p myproject # Include project-specific stats`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			printCompactContent(nil)
			return nil
		}
		defer func() { _ = database.Close() }()

		var stats []db.ConceptStats
		if flagProject != "" {
			stats, _ = database.ListConceptsWithStats(flagProject)
		} else {
			// Get stats across all projects
			projects, _ := database.ListProjects()
			for _, p := range projects {
				pStats, _ := database.ListConceptsWithStats(p)
				stats = append(stats, pStats...)
			}
		}

		printCompactContent(stats)
		return nil
	},
}

var flagBackupQuiet bool

var backupCmd = &cobra.Command{
	Use:   "backup [path]",
	Short: "Create a backup of the database",
	Long: `Create a backup of the prog database.

Backups are stored in ~/.prog/backups/ by default with timestamped names.
The last 10 backups are kept; older ones are automatically pruned.

Optionally specify a custom path for the backup file.

Examples:
  prog backup                    # Create backup in ~/.prog/backups/
  prog backup ~/my-backup.db     # Create backup at custom path
  prog backup --quiet            # Silent backup (for hooks)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		var backupPath string
		if len(args) > 0 {
			// Custom path - use VACUUM INTO directly
			backupPath = args[0]
			_, err = database.Exec(fmt.Sprintf("VACUUM INTO '%s'", backupPath))
			if err != nil {
				return fmt.Errorf("failed to create backup: %w", err)
			}
		} else {
			backupPath, err = database.Backup()
			if err != nil {
				return err
			}
		}

		if !flagBackupQuiet {
			fmt.Printf("Backup created: %s\n", backupPath)
		}
		return nil
	},
}

var backupsCmd = &cobra.Command{
	Use:   "backups",
	Short: "List available backups",
	Long: `List all available database backups.

Shows backups in ~/.prog/backups/, newest first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		backups, err := db.ListBackups()
		if err != nil {
			return err
		}

		if len(backups) == 0 {
			fmt.Println("No backups found")
			return nil
		}

		fmt.Printf("%-30s  %10s  %s\n", "BACKUP", "SIZE", "CREATED")
		for _, b := range backups {
			size := formatSize(b.Size)
			age := formatTimeAgo(b.ModTime)
			fmt.Printf("%-30s  %10s  %s\n", b.Name, size, age)
		}
		return nil
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore <path>",
	Short: "Restore database from a backup",
	Long: `Restore the prog database from a backup file.

This replaces the current database with the backup.
A backup of the current database is created first.

Examples:
  prog restore ~/.prog/backups/prog-2024-01-09T12-00-00.db
  prog restore ~/my-backup.db`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backupPath := args[0]

		// First, create a backup of current state
		database, err := openDB()
		if err != nil {
			// If we can't open the DB, that's fine - just restore
			fmt.Println("Note: Could not backup current database (may not exist)")
		} else {
			preRestorePath, err := database.Backup()
			_ = database.Close()
			if err != nil {
				fmt.Printf("Warning: Could not backup current database: %v\n", err)
			} else {
				fmt.Printf("Current database backed up to: %s\n", preRestorePath)
			}
		}

		// Restore from backup
		if err := db.Restore(backupPath); err != nil {
			return err
		}

		fmt.Printf("Restored from: %s\n", backupPath)
		return nil
	},
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

var tuiCmd = &cobra.Command{
	Use:     "tui",
	Aliases: []string{"ui"},
	Short:   "Launch interactive terminal UI",
	Long: `Launch an interactive terminal UI for managing tasks.

Navigation:
  j/k or arrows    Move cursor up/down
  g/G or home/end  Jump to first/last item
  enter or l       View task details
  esc or h         Go back to list

Actions:
  s   Start task (open -> in_progress)
  d   Mark done (in_progress -> done)
  o   Reopen task (any status -> open)
  L   Log progress (prompts for message)
  c   Cancel task (prompts for optional reason)
  n   Create new task (inherits project from selected item)
  D   Delete task
  a   Add dependency (prompts for blocker ID)
  r   Refresh task list

Filtering:
  /       Search by title/ID/description
  p       Filter by project
  1-7     Toggle status: 1=draft 2=open 3=in_progress 4=blocked 5=reviewing 6=done 7=canceled
  0       Show all statuses
  esc     Clear filters, or quit if none set

Press q to quit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		return tui.Run(database, flagProject)
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&flagProject, "project", "p", "", "Project scope")

	// add flags
	addCmd.Flags().BoolVarP(&flagEpic, "epic", "e", false, "Create an epic instead of a task")
	addCmd.Flags().BoolVar(&flagDraft, "draft", false, "Create in draft status (not yet ready for work)")
	addCmd.Flags().IntVar(&flagPriority, "priority", 2, "Priority (1=high, 2=medium, 3=low)")
	addCmd.Flags().StringVar(&flagParent, "parent", "", "Parent epic ID")
	addCmd.Flags().StringVar(&flagBlocks, "blocks", "", "ID of task this will block")
	addCmd.Flags().StringArrayVarP(&flagAddLabels, "label", "l", nil, "Label to attach (can be repeated)")
	addCmd.Flags().StringVar(&flagDoD, "dod", "", "Definition of done (completion criteria)")
	addCmd.Flags().StringVarP(&flagDesc, "desc", "d", "", "Description body")

	// list flags
	listCmd.Flags().StringVar(&flagStatus, "status", "", "Filter by status (draft, open, in_progress, blocked, reviewing, done, canceled)")
	listCmd.Flags().StringVar(&flagListParent, "parent", "", "Filter by parent epic ID")
	listCmd.Flags().StringVar(&flagListType, "type", "", "Filter by item type (task, epic)")
	listCmd.Flags().StringVar(&flagBlocking, "blocking", "", "Show items that block the given ID")
	listCmd.Flags().StringVar(&flagBlockedBy, "blocked-by", "", "Show items blocked by the given ID")
	listCmd.Flags().BoolVar(&flagHasBlockers, "has-blockers", false, "Show only items with unresolved blockers")
	listCmd.Flags().BoolVar(&flagNoBlockers, "no-blockers", false, "Show only items with no blockers")
	listCmd.Flags().StringArrayVarP(&flagFilterLabels, "label", "l", nil, "Filter by label (can be repeated, AND logic)")
	listCmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")

	// onboard flags
	onboardCmd.Flags().BoolVar(&flagForce, "force", false, "Replace existing Task Tracking section")

	// edit flags
	editCmd.Flags().StringVar(&flagEditTitle, "title", "", "New title for the task")
	editCmd.Flags().StringVar(&flagDoD, "dod", "", "Definition of done (use \"\" to clear)")

	// show flags
	showCmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")

	// ready flags
	readyCmd.Flags().StringArrayVarP(&flagFilterLabels, "label", "l", nil, "Filter by label (can be repeated, AND logic)")
	readyCmd.Flags().BoolVar(&flagJSON, "json", false, "Output as JSON")

	// status flags
	statusCmd.Flags().BoolVar(&flagStatusAll, "all", false, "Show all ready tasks (default: limit to 10)")
	statusCmd.Flags().StringArrayVarP(&flagFilterLabels, "label", "l", nil, "Filter by label (can be repeated, AND logic)")

	// learn flags
	learnCmd.Flags().StringArrayVarP(&flagLearnConcept, "concept", "c", nil, "Concept to tag this learning with (can be repeated)")
	learnCmd.Flags().StringArrayVarP(&flagLearnFile, "file", "f", nil, "Related file (can be repeated)")
	learnCmd.Flags().StringVar(&flagLearnDetail, "detail", "", "Full context/explanation (use '-' for stdin)")

	// learn subcommands
	learnCmd.AddCommand(learnEditCmd)
	learnCmd.AddCommand(learnStaleCmd)
	learnCmd.AddCommand(learnRmCmd)

	// learn edit flags
	learnEditCmd.Flags().StringVar(&flagLearnEditSummary, "summary", "", "New summary for the learning")
	learnEditCmd.Flags().StringVar(&flagLearnEditDetail, "detail", "", "New detail for the learning (use '-' for stdin)")

	// learn stale flags
	learnStaleCmd.Flags().StringVar(&flagLearnStaleReason, "reason", "", "Reason for marking as stale")

	// concepts flags
	conceptsCmd.Flags().BoolVar(&flagConceptsRecent, "recent", false, "Sort by last updated instead of learning count")
	conceptsCmd.Flags().StringVar(&flagConceptsRelated, "related", "", "Suggest concepts related to a task")
	conceptsCmd.Flags().StringVar(&flagConceptsSummary, "summary", "", "Set concept summary (requires concept name as argument)")
	conceptsCmd.Flags().StringVar(&flagConceptsRename, "rename", "", "Rename concept (requires concept name as argument)")
	conceptsCmd.Flags().BoolVar(&flagConceptsStats, "stats", false, "Show statistics (count and oldest learning age)")

	// labels flags
	labelsAddCmd.Flags().StringVar(&flagLabelsColor, "color", "", "Label color (hex, e.g. #ff0000)")

	// labels subcommands
	labelsCmd.AddCommand(labelsAddCmd)
	labelsCmd.AddCommand(labelsRmCmd)
	labelsCmd.AddCommand(labelsRenameCmd)

	// context flags
	contextCmd.Flags().StringArrayVarP(&flagContextConcept, "concept", "c", nil, "Concept to retrieve learnings for (can be repeated)")
	contextCmd.Flags().StringVarP(&flagContextQuery, "query", "q", "", "Full-text search query")
	contextCmd.Flags().BoolVar(&flagContextStale, "include-stale", false, "Include stale learnings in results")
	contextCmd.Flags().BoolVar(&flagContextSummary, "summary", false, "Show one-liner per learning (no detail)")
	contextCmd.Flags().StringVar(&flagContextID, "id", "", "Load specific learning by ID")
	contextCmd.Flags().BoolVar(&flagContextJSON, "json", false, "Output as JSON for machine processing")

	// backup flags
	backupCmd.Flags().BoolVarP(&flagBackupQuiet, "quiet", "q", false, "Silent backup (no output)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(readyCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(doneCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(cancelCmd)
	rootCmd.AddCommand(draftCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(projectsCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(appendCmd)
	rootCmd.AddCommand(descCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(parentCmd)
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(blocksCmd)
	rootCmd.AddCommand(labelCmd)
	rootCmd.AddCommand(unlabelCmd)
	rootCmd.AddCommand(learnCmd)
	rootCmd.AddCommand(conceptsCmd)
	rootCmd.AddCommand(labelsCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(primeCmd)
	rootCmd.AddCommand(compactCmd)
	rootCmd.AddCommand(onboardCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(backupsCmd)
	rootCmd.AddCommand(restoreCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Output formatting

func printItemsTable(items []model.Item) {
	if len(items) == 0 {
		fmt.Println("No items")
		return
	}

	fmt.Printf("%-12s %-12s %-4s %s\n", "ID", "STATUS", "PRI", "TITLE")
	for _, item := range items {
		title := item.Title
		if len(item.Labels) > 0 {
			title = formatLabels(item.Labels) + " " + title
		}
		fmt.Printf("%-12s %-12s %-4d %s\n", item.ID, item.Status, item.Priority, title)
	}
}

func printReadyTable(items []model.Item) {
	if len(items) == 0 {
		fmt.Println("No items")
		return
	}

	fmt.Printf("%-12s %-4s %s\n", "ID", "PRI", "TITLE")
	for _, item := range items {
		title := item.Title
		if len(item.Labels) > 0 {
			title = formatLabels(item.Labels) + " " + title
		}
		// Add [DoD] indicator if definition of done is set
		if item.DefinitionOfDone != nil && *item.DefinitionOfDone != "" {
			title = "[DoD] " + title
		}
		fmt.Printf("%-12s %-4d %s\n", item.ID, item.Priority, title)
	}
}

// formatLabels returns labels in [label1] [label2] format.
func formatLabels(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	var parts []string
	for _, l := range labels {
		parts = append(parts, "["+l+"]")
	}
	return strings.Join(parts, " ")
}

func printItemDetail(item *model.Item, logs []model.Log, deps []string, concepts []model.Concept) {
	fmt.Printf("ID:          %s\n", item.ID)
	fmt.Printf("Type:        %s\n", item.Type)
	fmt.Printf("Project:     %s\n", item.Project)
	fmt.Printf("Title:       %s\n", item.Title)
	fmt.Printf("Status:      %s\n", item.Status)
	fmt.Printf("Priority:    %d\n", item.Priority)
	if item.ParentID != nil {
		fmt.Printf("Parent:      %s\n", *item.ParentID)
	}
	if len(item.Labels) > 0 {
		fmt.Printf("Labels:      %s\n", strings.Join(item.Labels, ", "))
	}

	if item.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", item.Description)
	}

	if item.DefinitionOfDone != nil && *item.DefinitionOfDone != "" {
		fmt.Printf("\nDefinition of Done:\n%s\n", *item.DefinitionOfDone)
	}

	if len(deps) > 0 {
		fmt.Printf("\nDependencies:\n")
		for _, dep := range deps {
			fmt.Printf("  - %s\n", dep)
		}
	}

	if len(logs) > 0 {
		fmt.Printf("\nLogs:\n")
		for _, log := range logs {
			fmt.Printf("  [%s] %s\n", log.CreatedAt.Format("2006-01-02 15:04"), log.Message)
		}
	}

	if len(concepts) > 0 {
		fmt.Printf("\nSuggested context:\n")
		var conceptFlags []string
		for _, c := range concepts {
			summary := c.Summary
			if summary == "" {
				summary = "(no summary)"
			}
			fmt.Printf("  %s (%d) - %s\n", c.Name, c.LearningCount, summary)
			conceptFlags = append(conceptFlags, "-c "+c.Name)
		}
		fmt.Printf("\nLoad with: prog context %s -p %s --summary\n", strings.Join(conceptFlags, " "), item.Project)
	}
}

func printStatusReport(report *db.StatusReport, showAll bool) {
	project := report.Project
	if project == "" {
		project = "(all)"
	}
	fmt.Printf("Project: %s\n\n", project)

	fmt.Printf("Summary: %d draft, %d open, %d in progress, %d blocked, %d reviewing, %d done, %d canceled (%d ready)\n\n",
		report.Draft, report.Open, report.InProgress, report.Blocked, report.Reviewing, report.Done, report.Canceled, report.Ready)

	// Show project in output when viewing all projects
	showProject := report.Project == ""

	if len(report.DraftItems) > 0 {
		fmt.Println("Draft:")
		for _, item := range report.DraftItems {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, false))
		}
		fmt.Println()
	}

	if len(report.RecentDone) > 0 {
		fmt.Println("Recently completed:")
		for _, item := range report.RecentDone {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, false))
		}
		fmt.Println()
	}

	if len(report.InProgItems) > 0 {
		fmt.Println("In progress:")
		for _, item := range report.InProgItems {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, false))
		}
		fmt.Println()
	}

	if len(report.ReviewingItems) > 0 {
		fmt.Println("Reviewing:")
		for _, item := range report.ReviewingItems {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, false))
		}
		fmt.Println()
	}

	if len(report.BlockedItems) > 0 {
		fmt.Println("Blocked:")
		for _, item := range report.BlockedItems {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, false))
		}
		fmt.Println()
	}

	if len(report.ReadyItems) > 0 {
		fmt.Println("Ready for work:")
		readyLimit := 10
		displayItems := report.ReadyItems
		remaining := 0
		if !showAll && len(report.ReadyItems) > readyLimit {
			displayItems = report.ReadyItems[:readyLimit]
			remaining = len(report.ReadyItems) - readyLimit
		}
		for _, item := range displayItems {
			fmt.Printf("  %s\n", formatStatusItem(item, showProject, true))
		}
		if remaining > 0 {
			fmt.Printf("  (+%d more, use --all to see all)\n", remaining)
		}
	}
}

func formatStatusItem(item model.Item, showProject, showPriority bool) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s]", item.ID))
	if showProject && item.Project != "" {
		parts = append(parts, fmt.Sprintf("(%s)", item.Project))
	}
	if len(item.Labels) > 0 {
		parts = append(parts, formatLabels(item.Labels))
	}
	parts = append(parts, item.Title)
	if showPriority {
		parts = append(parts, fmt.Sprintf("(pri %d)", item.Priority))
	}
	return strings.Join(parts, " ")
}

func printConceptsTable(concepts []model.Concept) {
	fmt.Printf("%-20s %10s  %-12s  %s\n", "NAME", "LEARNINGS", "LAST UPDATED", "SUMMARY")
	for _, c := range concepts {
		ago := formatTimeAgo(c.LastUpdated)
		summary := c.Summary
		if len(summary) > 40 {
			summary = summary[:37] + "..."
		}
		fmt.Printf("%-20s %10d  %-12s  %s\n", c.Name, c.LearningCount, ago, summary)
	}
}

func printConceptsStats(stats []db.ConceptStats) {
	fmt.Printf("%-20s %6s  %s\n", "CONCEPT", "COUNT", "OLDEST")
	for _, s := range stats {
		oldest := "-"
		if s.OldestAge != nil {
			oldest = formatDurationShort(*s.OldestAge)
		}
		fmt.Printf("%-20s %6d  %s\n", s.Name, s.LearningCount, oldest)
	}
}

func formatDurationShort(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	hours := int(d.Hours())
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return "<1h"
}

func printLabelsTable(labels []model.Label) {
	fmt.Printf("%-20s  %-12s  %s\n", "NAME", "CREATED", "COLOR")
	for _, l := range labels {
		ago := formatTimeAgo(l.CreatedAt)
		color := l.Color
		if color == "" {
			color = "-"
		}
		fmt.Printf("%-20s  %-12s  %s\n", l.Name, ago, color)
	}
}

func printLearnings(learnings []model.Learning) {
	for i, l := range learnings {
		if i > 0 {
			fmt.Println()
		}

		// Header with ID, status, and age
		status := ""
		if l.Status == model.LearningStatusStale {
			status = " [stale]"
		}
		fmt.Printf("## %s%s (%s)\n", l.ID, status, formatTimeAgo(l.CreatedAt))

		// Summary
		fmt.Println(l.Summary)

		// Detail if present
		if l.Detail != "" {
			fmt.Printf("\n%s\n", l.Detail)
		}

		// Metadata
		if len(l.Concepts) > 0 {
			fmt.Printf("\nConcepts: %s\n", strings.Join(l.Concepts, ", "))
		}
		if len(l.Files) > 0 {
			fmt.Printf("Files: %s\n", strings.Join(l.Files, ", "))
		}
		if l.TaskID != nil {
			fmt.Printf("Task: %s\n", *l.TaskID)
		}
	}
}

// ItemReadyJSON is the JSON serialization format for ready items.
type ItemReadyJSON struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Priority int     `json:"priority"`
	Type     string  `json:"type"`
	Parent   *string `json:"parent"`
}

// ItemShowJSON is the JSON serialization format for show (full detail).
type ItemShowJSON struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Type             string    `json:"type"`
	Status           string    `json:"status"`
	Priority         int       `json:"priority"`
	Project          string    `json:"project"`
	Parent           *string   `json:"parent"`
	Description      string    `json:"description"`
	DefinitionOfDone *string   `json:"definition_of_done"`
	Labels           []string  `json:"labels"`
	Dependencies     []string  `json:"dependencies"`
	Logs             []LogJSON `json:"logs"`
}

// ItemListJSON is the JSON serialization format for list (show schema minus logs).
type ItemListJSON struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Type             string   `json:"type"`
	Status           string   `json:"status"`
	Priority         int      `json:"priority"`
	Project          string   `json:"project"`
	Parent           *string  `json:"parent"`
	Description      string   `json:"description"`
	DefinitionOfDone *string  `json:"definition_of_done"`
	Labels           []string `json:"labels"`
	Dependencies     []string `json:"dependencies"`
}

// LogJSON is the JSON serialization format for log entries.
type LogJSON struct {
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

// LearningJSON is the JSON serialization format for learnings.
type LearningJSON struct {
	ID        string   `json:"id"`
	Summary   string   `json:"summary"`
	Detail    string   `json:"detail,omitempty"`
	Concepts  []string `json:"concepts"`
	Files     []string `json:"files,omitempty"`
	CreatedAt string   `json:"created_at"`
	Status    string   `json:"status"`
}

func printLearningsJSON(learnings []model.Learning) error {
	output := make([]LearningJSON, 0, len(learnings))
	for _, l := range learnings {
		lj := LearningJSON{
			ID:        l.ID,
			Summary:   l.Summary,
			Detail:    l.Detail,
			Concepts:  l.Concepts,
			Files:     l.Files,
			CreatedAt: l.CreatedAt.Format(time.RFC3339),
			Status:    string(l.Status),
		}
		if lj.Concepts == nil {
			lj.Concepts = []string{}
		}
		output = append(output, lj)
	}
	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(b))
	return nil
}

func printLearningSummaries(learnings []model.Learning, requestedConcepts []string, conceptSummaries map[string]string) {
	// Print concept headers with summaries
	for _, conceptName := range requestedConcepts {
		summary := conceptSummaries[conceptName]
		if summary == "" {
			summary = "(no summary)"
		}
		fmt.Printf("%s: %s\n", conceptName, summary)
	}
	if len(requestedConcepts) > 0 {
		fmt.Println()
	}

	// Print one-liner per learning
	for _, l := range learnings {
		status := ""
		if l.Status == model.LearningStatusStale {
			status = " [stale]"
		}
		fmt.Printf("  %s: %s%s\n", l.ID, l.Summary, status)
	}
}

func printAllLearningSummaries(learnings []model.Learning, conceptSummaries map[string]string) {
	// Group learnings by concept
	type conceptGroup struct {
		summary   string
		learnings []model.Learning
	}
	groups := make(map[string]*conceptGroup)
	conceptOrder := []string{}

	for _, l := range learnings {
		for _, conceptName := range l.Concepts {
			if groups[conceptName] == nil {
				groups[conceptName] = &conceptGroup{
					summary: conceptSummaries[conceptName],
				}
				conceptOrder = append(conceptOrder, conceptName)
			}
			groups[conceptName].learnings = append(groups[conceptName].learnings, l)
		}
	}

	// Print grouped by concept
	for i, conceptName := range conceptOrder {
		if i > 0 {
			fmt.Println()
		}
		group := groups[conceptName]
		summary := group.summary
		if summary == "" {
			summary = "(no summary)"
		}
		fmt.Printf("%s: %s\n", conceptName, summary)

		for _, l := range group.learnings {
			status := ""
			if l.Status == model.LearningStatusStale {
				status = " [stale]"
			}
			fmt.Printf("  %s: %s%s\n", l.ID, l.Summary, status)
		}
	}
}

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func printDepGraph(edges []db.DepEdge) {
	// Group by item
	type depInfo struct {
		title  string
		status string
		deps   []db.DepEdge
	}
	items := make(map[string]*depInfo)
	order := []string{}

	for _, e := range edges {
		if _, ok := items[e.ItemID]; !ok {
			items[e.ItemID] = &depInfo{title: e.ItemTitle, status: e.ItemStatus}
			order = append(order, e.ItemID)
		}
		items[e.ItemID].deps = append(items[e.ItemID].deps, e)
	}

	for _, id := range order {
		info := items[id]
		fmt.Printf("%s [%s] %s\n", id, info.status, info.title)
		for i, dep := range info.deps {
			prefix := "├──"
			if i == len(info.deps)-1 {
				prefix = "└──"
			}
			fmt.Printf("  %s %s [%s] %s\n", prefix, dep.DependsOnID, dep.DependsOnStatus, dep.DependsOnTitle)
		}
	}
}

func printPrimeContent(report *db.StatusReport) {
	fmt.Println(`# Prog CLI Context

This project uses 'prog' for cross-session task management.
Run 'prog status' to see current state.

## Starting Work

When picking up a task:
1. prog show <task>                 # See task + suggested concepts
2. prog context -c X -c Y           # Load relevant concepts
   prog context -c X --summary      # Or scan first if many learnings

Load context that's relevant to your task. Don't skip it, don't load everything.

## SESSION CLOSE PROTOCOL

Before ending ANY session, you MUST complete ALL of these steps:

1. Log progress on active tasks:
   prog log <id> "What you accomplished"

2. Verify artifacts are updated:
   - If you changed behavior: is help text / CLI output updated?
   - If you added features: is documentation current?
   - If you fixed bugs: do error messages reflect the fix?
   - Do new tests need to be written? Do existing tests need updating?
   Run the relevant commands to confirm outputs match the code.

3. Update task status:
   - prog done <id>     # if complete (will prompt for reflection)
   - prog open <id>     # reopen if needed

4. Add handoff context for next agent:
   prog append <id> "Next steps: ..."

5. Update parent epic (if task is part of one):
   prog append <epic-id> "Completed X, next: Y"

## Logging Learnings

When prog done prompts for reflection, ask: What would help the next agent?
  - What pattern or technique proved effective?
  - What gotcha would trap someone unfamiliar?
  - What's not obvious from reading the code?

Validate insights with the user before logging.

To log (ALWAYS include both summary AND detail):
  prog concepts                              # Check existing concepts first
  prog learn "summary" -c concept --detail "full explanation"

Why both? Two-phase retrieval:
  - Summary: one-liner for scanning/discovery
  - Detail: full context when selected
  Without detail, future agents get only the one-liner.

Good learnings are specific and actionable:
  ✓ prog learn "Schema migrations need built binary" -c database \
      --detail "go run doesn't embed assets; must use go build first"

Not learnings (use prog log instead):
  ✗ "Fixed the auth bug"
  ✗ "This file handles authentication"

NEVER end a session without updating task state.
Work is NOT complete until prog reflects reality.

## Core Rules

- Use 'prog' for strategic work tracking (persists across sessions)
- Use TodoWrite for tactical within-session checklists
- Always claim work before starting: prog start <id>
- Log progress frequently, not just at the end

## Before Completing Work

If a task has a Definition of Done:
1. Run 'prog show <id>' to see the DoD criteria
2. Verify each criterion is met before marking complete
3. Run any commands mentioned in the DoD
4. Only then call 'prog done <id>'

Tasks with DoD show [DoD] in 'prog ready' output.

## Essential Commands

# Finding work
prog status              # Overview
prog ready               # Tasks ready to work on
prog show <id>           # Full details + suggested concepts

# Working
prog start <id>          # Claim a task
prog log <id> "message"  # Log progress
prog done <id>           # Mark complete
prog review <id>         # Mark as reviewing (awaiting merge)
prog open <id>           # Reopen a task

# Creating
prog add "title" -p project           # New task
prog add "title" -d "description"     # With description
prog add "title" -l bug               # With label
prog add "title" -e                   # New epic

# Editing
prog append <id> "text"        # Add to description
prog label <id> <name>         # Add label to task

# Context retrieval
prog context -c concept        # Load learnings for a concept
prog context -c X --summary    # Scan one-liners first
prog concepts                  # List available concepts
prog learn "summary" -c X --detail "explanation"  # Log with both parts

# Filtering
prog list -p myproject         # Filter by project
prog list --status open        # Filter by status
prog list -l bug -l urgent     # Filter by labels (AND)
prog ready -p myproject        # Ready in project`)

	fmt.Println("\n## Current State")

	if report != nil {
		if len(report.InProgItems) > 0 {
			fmt.Println("\nIn progress:")
			for _, item := range report.InProgItems {
				fmt.Printf("  [%s] %s\n", item.ID, item.Title)
			}
		}

		if len(report.ReviewingItems) > 0 {
			fmt.Println("\nReviewing:")
			for _, item := range report.ReviewingItems {
				fmt.Printf("  [%s] %s\n", item.ID, item.Title)
			}
		}

		if len(report.BlockedItems) > 0 {
			fmt.Println("\nBlocked:")
			for _, item := range report.BlockedItems {
				fmt.Printf("  [%s] %s\n", item.ID, item.Title)
			}
		}

		fmt.Println("\nRun 'prog ready [-p project]' to find unblocked work.")
	} else {
		fmt.Println("\n(No database connection - run 'prog init' if needed)")
	}
}

func printCompactContent(stats []db.ConceptStats) {
	fmt.Println(`# Compact Learnings

Groom learnings and concepts using two phases: **discovery** then **selection**.

## Why Compact?

Over time, learnings accumulate. Without grooming:
- Redundant entries waste context tokens
- Stale learnings mislead agents
- Unclear summaries reduce retrieval value
- Fragmented insights are harder to find

Compaction keeps the knowledge base high-signal and navigable.

## Phase 1: Discovery

Scan all learning summaries grouped by concept:

` + "```" + `bash
prog context -p <project> --summary   # All learnings, grouped by concept
` + "```" + `

Flag candidates:
- **Redundant**: Similar summaries (potential duplicates)
- **Stale**: References old code/patterns, or old learnings (7+ days) that may be outdated
- **Low quality**: Vague summaries, missing detail, not actionable
- **Fragmented**: Related small learnings that should be one

Present candidates to user. Confirm which to address.

## Phase 2: Selection & Grooming

Load full detail only for flagged candidates:

` + "```" + `bash
prog context --id lrn-abc123          # Specific learning
` + "```" + `

For each candidate, determine action:
- **Archive**: Redundant or superseded → ` + "`prog learn stale <id> --reason \"...\"`" + `
- **Update**: Valid but unclear → ` + "`prog learn edit <id> --summary \"...\"`" + `
- **Consolidate**: Merge related → archive originals, create new combined learning
- **Keep**: No changes needed

Present changes to user. Execute after approval.

## Repeat

After each batch:
- Re-run discovery if significant changes
- Continue until no candidates remain or user is satisfied

## Quality Guidelines

**Good summaries**: Specific, actionable, self-contained
**Archive when**: Superseded, redundant, or references removed code
**Consolidate when**: Multiple learnings express the same insight
**Update when**: Insight valid but poorly expressed`)

	// Show current stats if available
	if len(stats) > 0 {
		fmt.Println("\n## Current Stats")
		fmt.Printf("\n%-20s %6s  %s\n", "CONCEPT", "COUNT", "OLDEST")
		for _, s := range stats {
			oldest := "-"
			if s.OldestAge != nil {
				oldest = formatDurationShort(*s.OldestAge)
			}
			fmt.Printf("%-20s %6d  %s\n", s.Name, s.LearningCount, oldest)
		}

		// Highlight compaction candidates
		var candidates []string
		for _, s := range stats {
			if s.LearningCount >= 5 {
				candidates = append(candidates, s.Name)
			}
		}
		if len(candidates) > 0 {
			fmt.Printf("\nCompaction candidates (5+ learnings): %s\n", strings.Join(candidates, ", "))
		}
	}
}
