package main

import (
	"encoding/json"
	"fmt"
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
	flagLearnStaleReason string
	flagConceptsRecent   bool
	flagConceptsRelated  string
	flagConceptsSummary  string
	flagConceptsRename   string
	flagContextConcept   []string
	flagContextQuery     string
	flagContextStale     bool
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
	return database, nil
}

var rootCmd = &cobra.Command{
	Use:   "prog",
	Short: "Lightweight task management for agents",
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
  prog add "Dependency" --blocks ts-xyz789`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		itemType := model.ItemTypeTask
		if flagEpic {
			itemType = model.ItemTypeEpic
		}

		item := &model.Item{
			ID:        model.GenerateID(itemType),
			Project:   flagProject,
			Type:      itemType,
			Title:     strings.Join(args, " "),
			Status:    model.StatusOpen,
			Priority:  flagPriority,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
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

		fmt.Println(item.ID)
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
  prog list --no-blockers`,
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
				return fmt.Errorf("invalid status: %s (valid: open, in_progress, blocked, done, canceled)", flagStatus)
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
		}

		items, err := database.ListItemsFiltered(filter)
		if err != nil {
			return err
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
  - Status is "open" (not in_progress, blocked, or done)
  - All dependencies are "done"

Results are sorted by priority (1=high first).

Examples:
  prog ready
  prog ready -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		items, err := database.ReadyItems(flagProject)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			fmt.Println("No ready tasks")
			return nil
		}
		printReadyTable(items)
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Long: `Show full details for a task including description, logs, and dependencies.

Example:
  prog show ts-a1b2c3`,
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

		logs, err := database.GetLogs(args[0])
		if err != nil {
			return err
		}

		deps, err := database.GetDeps(args[0])
		if err != nil {
			return err
		}

		printItemDetail(item, logs, deps)
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
		return nil
	},
}

var blockCmd = &cobra.Command{
	Use:   "block <id> <reason>",
	Short: "Mark a task as blocked",
	Long: `Mark a task as blocked and log the reason.

Use this when you can't proceed and need to hand off to another agent.

Example:
  prog block ts-a1b2c3 "Need API spec from product team"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]
		reason := strings.Join(args[1:], " ")

		if err := database.UpdateStatus(id, model.StatusBlocked); err != nil {
			return err
		}
		if err := database.AddLog(id, "Blocked: "+reason); err != nil {
			return err
		}
		fmt.Printf("Blocked %s: %s\n", id, reason)
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
  - Count by status (open, in_progress, blocked, done)
  - Recently completed tasks
  - Currently in-progress tasks
  - Blocked tasks with reasons
  - Ready tasks by priority (limited to 10 by default)

Use --all to show all ready tasks.

Examples:
  prog status
  prog status -p myproject
  prog status --all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		report, err := database.ProjectStatus(flagProject)
		if err != nil {
			return err
		}

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
	Short: "Edit a task's title or description",
	Long: `Edit a task's title or description.

With --title, updates the title directly without opening an editor.
Without flags, opens the description in your configured editor.

Uses $PROG_EDITOR if set, otherwise defaults to nvim, then nano, then vi.

Examples:
  prog edit ts-a1b2c3                     # Edit description in editor
  prog edit ts-a1b2c3 --title "New title" # Update title directly
  PROG_EDITOR=code prog edit ts-a1b2c3    # Use VS Code as editor`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		id := args[0]

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

var learnCmd = &cobra.Command{
	Use:   "learn <summary>",
	Short: "Log a learning for future context retrieval",
	Long: `Log a learning discovered during work.

Learnings are tagged with concepts for organized retrieval.
Concepts are created automatically if they don't exist.

If a task is in progress for the project, the learning is linked to it.

Examples:
  prog learn "Token refresh has race condition" -p myproject -c auth -c concurrency
  prog learn "Config loaded from env first" -p myproject -c config -f config.go`,
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

		now := time.Now()
		learning := &model.Learning{
			ID:        model.GenerateLearningID(),
			Project:   project,
			CreatedAt: now,
			UpdatedAt: now,
			TaskID:    taskID,
			Summary:   strings.Join(args, " "),
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
		return nil
	},
}

var learnEditCmd = &cobra.Command{
	Use:   "edit <learning-id>",
	Short: "Edit a learning's summary",
	Long: `Edit an existing learning's summary.

Example:
  prog learn edit lrn-abc123 --summary "Updated summary"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagLearnEditSummary == "" {
			return fmt.Errorf("--summary is required")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateLearningSummary(args[0], flagLearnEditSummary); err != nil {
			return err
		}
		fmt.Printf("Updated %s\n", args[0])
		return nil
	},
}

var learnStaleCmd = &cobra.Command{
	Use:   "stale <learning-id>",
	Short: "Mark a learning as stale (outdated)",
	Long: `Mark a learning as stale when it's outdated but still useful for reference.

Example:
  prog learn stale lrn-abc123 --reason "Refactored in v2"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		if err := database.UpdateLearningStatus(args[0], model.LearningStatusStale); err != nil {
			return err
		}

		// Log reason if provided
		if flagLearnStaleReason != "" {
			// We could add a detail field update here, but for now just print
			fmt.Printf("Marked %s as stale: %s\n", args[0], flagLearnStaleReason)
		} else {
			fmt.Printf("Marked %s as stale\n", args[0])
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
Default sort is by learning count (most used first).

Examples:
  prog concepts -p myproject                        # list concepts
  prog concepts -p myproject --recent               # sort by last updated
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

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Retrieve learnings for context",
	Long: `Retrieve learnings by concept or full-text search.

Use this to load relevant context before starting work on a task.

Examples:
  prog context -c auth -c concurrency -p myproject   # by concepts
  prog context -q "rate limit" -p myproject          # full-text search
  prog context -c auth --include-stale -p myproject  # include stale learnings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagProject == "" {
			return fmt.Errorf("project is required (-p)")
		}
		if len(flagContextConcept) == 0 && flagContextQuery == "" {
			return fmt.Errorf("specify concepts (-c) or query (-q)")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

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
			fmt.Println("No learnings found")
			return nil
		}

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
  s   Start task (open/blocked -> in_progress)
  d   Mark done (in_progress -> done)
  b   Block task (prompts for reason)
  L   Log progress (prompts for message)
  c   Cancel task (prompts for optional reason)
  n   Create new task (inherits project from selected item)
  D   Delete task
  a   Add dependency (prompts for blocker ID)
  r   Refresh task list

Filtering:
  /       Search by title/ID/description
  p       Filter by project
  1-5     Toggle status: 1=open 2=in_progress 3=blocked 4=done 5=canceled
  0       Show all statuses
  esc     Clear filters, or quit if none set

Press q to quit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer func() { _ = database.Close() }()

		return tui.Run(database)
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&flagProject, "project", "p", "", "Project scope")

	// add flags
	addCmd.Flags().BoolVarP(&flagEpic, "epic", "e", false, "Create an epic instead of a task")
	addCmd.Flags().IntVar(&flagPriority, "priority", 2, "Priority (1=high, 2=medium, 3=low)")
	addCmd.Flags().StringVar(&flagParent, "parent", "", "Parent epic ID")
	addCmd.Flags().StringVar(&flagBlocks, "blocks", "", "ID of task this will block")

	// list flags
	listCmd.Flags().StringVar(&flagStatus, "status", "", "Filter by status (open, in_progress, blocked, done, canceled)")
	listCmd.Flags().StringVar(&flagListParent, "parent", "", "Filter by parent epic ID")
	listCmd.Flags().StringVar(&flagListType, "type", "", "Filter by item type (task, epic)")
	listCmd.Flags().StringVar(&flagBlocking, "blocking", "", "Show items that block the given ID")
	listCmd.Flags().StringVar(&flagBlockedBy, "blocked-by", "", "Show items blocked by the given ID")
	listCmd.Flags().BoolVar(&flagHasBlockers, "has-blockers", false, "Show only items with unresolved blockers")
	listCmd.Flags().BoolVar(&flagNoBlockers, "no-blockers", false, "Show only items with no blockers")

	// onboard flags
	onboardCmd.Flags().BoolVar(&flagForce, "force", false, "Replace existing Task Tracking section")

	// edit flags
	editCmd.Flags().StringVar(&flagEditTitle, "title", "", "New title for the task")

	// status flags
	statusCmd.Flags().BoolVar(&flagStatusAll, "all", false, "Show all ready tasks (default: limit to 10)")

	// learn flags
	learnCmd.Flags().StringArrayVarP(&flagLearnConcept, "concept", "c", nil, "Concept to tag this learning with (can be repeated)")
	learnCmd.Flags().StringArrayVarP(&flagLearnFile, "file", "f", nil, "Related file (can be repeated)")

	// learn subcommands
	learnCmd.AddCommand(learnEditCmd)
	learnCmd.AddCommand(learnStaleCmd)
	learnCmd.AddCommand(learnRmCmd)

	// learn edit flags
	learnEditCmd.Flags().StringVar(&flagLearnEditSummary, "summary", "", "New summary for the learning")

	// learn stale flags
	learnStaleCmd.Flags().StringVar(&flagLearnStaleReason, "reason", "", "Reason for marking as stale")

	// concepts flags
	conceptsCmd.Flags().BoolVar(&flagConceptsRecent, "recent", false, "Sort by last updated instead of learning count")
	conceptsCmd.Flags().StringVar(&flagConceptsRelated, "related", "", "Suggest concepts related to a task")
	conceptsCmd.Flags().StringVar(&flagConceptsSummary, "summary", "", "Set concept summary (requires concept name as argument)")
	conceptsCmd.Flags().StringVar(&flagConceptsRename, "rename", "", "Rename concept (requires concept name as argument)")

	// context flags
	contextCmd.Flags().StringArrayVarP(&flagContextConcept, "concept", "c", nil, "Concept to retrieve learnings for (can be repeated)")
	contextCmd.Flags().StringVarP(&flagContextQuery, "query", "q", "", "Full-text search query")
	contextCmd.Flags().BoolVar(&flagContextStale, "include-stale", false, "Include stale learnings in results")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(readyCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(doneCmd)
	rootCmd.AddCommand(cancelCmd)
	rootCmd.AddCommand(blockCmd)
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
	rootCmd.AddCommand(learnCmd)
	rootCmd.AddCommand(conceptsCmd)
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(primeCmd)
	rootCmd.AddCommand(onboardCmd)
	rootCmd.AddCommand(tuiCmd)
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
		fmt.Printf("%-12s %-12s %-4d %s\n", item.ID, item.Status, item.Priority, item.Title)
	}
}

func printReadyTable(items []model.Item) {
	if len(items) == 0 {
		fmt.Println("No items")
		return
	}

	fmt.Printf("%-12s %-4s %s\n", "ID", "PRI", "TITLE")
	for _, item := range items {
		fmt.Printf("%-12s %-4d %s\n", item.ID, item.Priority, item.Title)
	}
}

func printItemDetail(item *model.Item, logs []model.Log, deps []string) {
	fmt.Printf("ID:          %s\n", item.ID)
	fmt.Printf("Type:        %s\n", item.Type)
	fmt.Printf("Project:     %s\n", item.Project)
	fmt.Printf("Title:       %s\n", item.Title)
	fmt.Printf("Status:      %s\n", item.Status)
	fmt.Printf("Priority:    %d\n", item.Priority)
	if item.ParentID != nil {
		fmt.Printf("Parent:      %s\n", *item.ParentID)
	}

	if item.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", item.Description)
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
}

func printStatusReport(report *db.StatusReport, showAll bool) {
	project := report.Project
	if project == "" {
		project = "(all)"
	}
	fmt.Printf("Project: %s\n\n", project)

	fmt.Printf("Summary: %d open, %d in progress, %d blocked, %d done, %d canceled (%d ready)\n\n",
		report.Open, report.InProgress, report.Blocked, report.Done, report.Canceled, report.Ready)

	if len(report.RecentDone) > 0 {
		fmt.Println("Recently completed:")
		for _, item := range report.RecentDone {
			fmt.Printf("  [%s] %s\n", item.ID, item.Title)
		}
		fmt.Println()
	}

	if len(report.InProgItems) > 0 {
		fmt.Println("In progress:")
		for _, item := range report.InProgItems {
			fmt.Printf("  [%s] %s\n", item.ID, item.Title)
		}
		fmt.Println()
	}

	if len(report.BlockedItems) > 0 {
		fmt.Println("Blocked:")
		for _, item := range report.BlockedItems {
			fmt.Printf("  [%s] %s\n", item.ID, item.Title)
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
			fmt.Printf("  [%s] %s (pri %d)\n", item.ID, item.Title, item.Priority)
		}
		if remaining > 0 {
			fmt.Printf("  (+%d more, use --all to see all)\n", remaining)
		}
	}
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
   - prog done <id>     # if complete
   - prog block <id> "reason"  # if blocked

4. Add handoff context for next agent:
   prog append <id> "Next steps: ..."

5. Update parent epic (if task is part of one):
   prog append <epic-id> "Completed X, next: Y"

NEVER end a session without updating task state.
Work is NOT complete until prog reflects reality.

## Core Rules

- Use 'prog' for strategic work tracking (persists across sessions)
- Use TodoWrite for tactical within-session checklists
- Always claim work before starting: prog start <id>
- Log progress frequently, not just at the end

## Essential Commands

# Finding work
prog status              # Overview
prog ready               # Tasks ready to work on
prog show <id>           # Full details

# Working
prog start <id>          # Claim a task
prog log <id> "message"  # Log progress
prog done <id>           # Mark complete
prog block <id> "why"    # Mark blocked

# Creating
prog add "title" -p project    # New task
prog add "title" -e            # New epic

# Editing
prog append <id> "text"        # Add to description

# Filtering
prog list -p myproject         # Filter by project
prog list --status open        # Filter by status
prog ready -p myproject        # Ready in project

## Current State`)

	if report != nil {
		fmt.Printf("\n%d open, %d in progress, %d blocked, %d done, %d canceled\n",
			report.Open, report.InProgress, report.Blocked, report.Done, report.Canceled)

		if len(report.InProgItems) > 0 {
			fmt.Println("\nIn progress:")
			for _, item := range report.InProgItems {
				fmt.Printf("  [%s] %s\n", item.ID, item.Title)
			}
		}

		if len(report.BlockedItems) > 0 {
			fmt.Println("\nBlocked:")
			for _, item := range report.BlockedItems {
				fmt.Printf("  [%s] %s\n", item.ID, item.Title)
			}
		}
	} else {
		fmt.Println("\n(No database connection - run 'prog init' if needed)")
	}
}
