package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/baiirun/dotworld-tasks/internal/db"
	"github.com/baiirun/dotworld-tasks/internal/model"
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
	flagProject   string
	flagStatus    string
	flagEpic      bool
	flagPriority  int
	flagDependsOn string
	flagForce     bool
	flagParent    string
	flagBlocks    string
)

func openDB() (*db.DB, error) {
	path, err := db.DefaultPath()
	if err != nil {
		return nil, err
	}
	database, err := db.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w (try running 'tasks init' first)", err)
	}
	return database, nil
}

var rootCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Lightweight task management for agents",
	Long: `A CLI for managing tasks, epics, and dependencies.
Designed for AI agents to track work across sessions.

Database: ~/.world/tasks/tasks.db

Quick start:
  tasks init
  tasks onboard
  tasks add "Build feature X" -p myproject
  tasks ready -p myproject
  tasks start <id>
  tasks done <id>`,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the tasks database",
	Long:  "Creates the database at ~/.world/tasks/tasks.db if it doesn't exist.",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.Init(); err != nil {
			return err
		}
		fmt.Println("Initialized tasks database")
		return nil
	},
}

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Create a new task or epic",
	Long: `Create a new task (or epic with -e flag).

Returns the generated ID (ts-XXXXXX for tasks, ep-XXXXXX for epics).

Examples:
  tasks add "Fix login bug" -p myproject
  tasks add "Auth system" -p myproject -e
  tasks add "Critical fix" --priority 1
  tasks add "Subtask" --parent ep-abc123
  tasks add "Dependency" --blocks ts-xyz789`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
	Long: `List all tasks, optionally filtered by project and/or status.

Examples:
  tasks list
  tasks list -p myproject
  tasks list --status open
  tasks list -p myproject --status blocked`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		var status *model.Status
		if flagStatus != "" {
			s := model.Status(flagStatus)
			if !s.IsValid() {
				return fmt.Errorf("invalid status: %s (valid: open, in_progress, blocked, done, canceled)", flagStatus)
			}
			status = &s
		}

		items, err := database.ListItems(flagProject, status)
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
  tasks ready
  tasks ready -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		items, err := database.ReadyItems(flagProject)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			fmt.Println("No ready tasks")
			return nil
		}
		printItemsTable(items)
		return nil
	},
}

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show task details",
	Long: `Show full details for a task including description, logs, and dependencies.

Example:
  tasks show ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
		defer database.Close()

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
		defer database.Close()

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
  tasks cancel ts-a1b2c3
  tasks cancel ts-a1b2c3 "Requirements changed, no longer needed"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
  tasks block ts-a1b2c3 "Need API spec from product team"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
  tasks delete ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
  tasks log ts-a1b2c3 "Implemented token refresh logic"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
  tasks graph
  tasks graph -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
		defer database.Close()

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
  - Ready tasks by priority

Examples:
  tasks status
  tasks status -p myproject`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		report, err := database.ProjectStatus(flagProject)
		if err != nil {
			return err
		}

		printStatusReport(report)
		return nil
	},
}

var appendCmd = &cobra.Command{
	Use:   "append <id> <text>",
	Short: "Append text to a task's description",
	Long: `Append text to a task's description.

Use this to add context, decisions, or handoff notes.

Example:
  tasks append ts-a1b2c3 "Decided to use JWT instead of sessions"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

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
	Short: "Edit a task's description in $TASKS_EDITOR",
	Long: `Open a task's description in your configured editor.

Uses $TASKS_EDITOR if set, otherwise defaults to nvim, then nano, then vi.
After saving and closing, the description is updated.

Example:
  tasks edit ts-a1b2c3
  TASKS_EDITOR=code tasks edit ts-a1b2c3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		id := args[0]

		// Get current description
		item, err := database.GetItem(id)
		if err != nil {
			return err
		}

		// Get editor (prefer $TASKS_EDITOR, then nvim, then nano)
		editor := os.Getenv("TASKS_EDITOR")
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
		tmpfile, err := os.CreateTemp("", "tasks-edit-*.md")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpfile.Name()
		defer os.Remove(tmpPath)

		// Write current description
		if _, err := tmpfile.WriteString(item.Description); err != nil {
			tmpfile.Close()
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpfile.Close()

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
For adding to existing content, use 'tasks append' instead.

Example:
  tasks desc ts-a1b2c3 "New description text here"`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		id := args[0]
		text := strings.Join(args[1:], " ")

		if err := database.SetDescription(id, text); err != nil {
			return err
		}
		fmt.Printf("Updated description for %s\n", id)
		return nil
	},
}

var depCmd = &cobra.Command{
	Use:   "dep <id> --on <other>",
	Short: "Add a dependency",
	Long: `Add a dependency between tasks.

The first task will be blocked until the dependency is done.

Example:
  tasks dep ts-a1b2c3 --on ts-d4e5f6
  # ts-a1b2c3 is now blocked until ts-d4e5f6 is done`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDependsOn == "" {
			return fmt.Errorf("--on flag is required")
		}

		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.AddDep(args[0], flagDependsOn); err != nil {
			return err
		}
		fmt.Printf("%s now depends on %s\n", args[0], flagDependsOn)
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
  tasks parent ts-a1b2c3 ep-d4e5f6
  # ts-a1b2c3 is now a child of ep-d4e5f6`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.SetParent(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("%s is now under %s\n", args[0], args[1])
		return nil
	},
}

var blocksCmd = &cobra.Command{
	Use:   "blocks <id> <other-id>",
	Short: "Mark a task as blocking another",
	Long: `Mark a task as blocking another task.

The second task cannot be started until the first is done.

Example:
  tasks blocks ts-a1b2c3 ts-d4e5f6
  # ts-d4e5f6 cannot start until ts-a1b2c3 is done`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		// blocks A B means B depends on A (A blocks B)
		if err := database.AddDep(args[1], args[0]); err != nil {
			return err
		}
		fmt.Printf("%s now blocks %s\n", args[0], args[1])
		return nil
	},
}

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Add tasks integration to CLAUDE.md and install SessionStart hook",
	Long: `Set up tasks integration for Claude Code agents.

This command does two things:
1. Writes a tasks workflow snippet to CLAUDE.md in the current directory
2. Installs a SessionStart hook in ~/.claude/settings.json to auto-run 'tasks prime'

Creates files if they don't exist. Skips if already configured (use --force to update).

Example:
  cd ~/code/myproject
  tasks onboard
  tasks onboard --force  # Update existing configuration`,
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

This project uses **tasks** for cross-session task management.
Run ` + "`tasks prime`" + ` for workflow context, or configure hooks for auto-injection.

**Quick reference:**
` + "```" + `
tasks ready              # Find unblocked work
tasks add "Title" -p X   # Create task
tasks start <id>         # Claim work
tasks log <id> "msg"     # Log progress
tasks done <id>          # Complete work
` + "```" + `

For full workflow: ` + "`tasks prime`" + `
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
			fmt.Println("Created CLAUDE.md with tasks integration")
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
			fmt.Printf("Added tasks integration to %s\n", claudePath)
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

// installSessionStartHook installs "tasks prime" in ~/.claude/settings.json
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
	const hookCommand = "tasks prime"
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
agents maintain context about the tasks workflow.

Example hook configuration in Claude Code settings:
  "hooks": {
    "SessionStart": [{"command": "tasks prime"}],
    "PreCompact": [{"command": "tasks prime"}]
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			// Still output prime content even if DB fails
			printPrimeContent(nil)
			return nil
		}
		defer database.Close()

		report, _ := database.ProjectStatus("")
		printPrimeContent(report)
		return nil
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
	listCmd.Flags().StringVar(&flagStatus, "status", "", "Filter by status (open, in_progress, blocked, done)")

	// dep flags
	depCmd.Flags().StringVar(&flagDependsOn, "on", "", "ID of the item this depends on")

	// onboard flags
	onboardCmd.Flags().BoolVar(&flagForce, "force", false, "Replace existing Task Tracking section")

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
	rootCmd.AddCommand(depCmd)
	rootCmd.AddCommand(parentCmd)
	rootCmd.AddCommand(blocksCmd)
	rootCmd.AddCommand(primeCmd)
	rootCmd.AddCommand(onboardCmd)
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
	fmt.Println(strings.Repeat("-", 60))
	for _, item := range items {
		fmt.Printf("%-12s %-12s %-4d %s\n", item.ID, item.Status, item.Priority, item.Title)
	}
}

func printItemDetail(item *model.Item, logs []model.Log, deps []string) {
	fmt.Printf("ID:          %s\n", item.ID)
	fmt.Printf("Type:        %s\n", item.Type)
	fmt.Printf("Project:     %s\n", item.Project)
	fmt.Printf("Title:       %s\n", item.Title)
	fmt.Printf("Status:      %s\n", item.Status)
	fmt.Printf("Priority:    %d\n", item.Priority)
	fmt.Printf("Created:     %s\n", item.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated:     %s\n", item.UpdatedAt.Format(time.RFC3339))

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

func printStatusReport(report *db.StatusReport) {
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
		for _, item := range report.ReadyItems {
			fmt.Printf("  [%s] %s (pri %d)\n", item.ID, item.Title, item.Priority)
		}
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
	fmt.Println(`# Tasks CLI Context

This project uses 'tasks' for cross-session task management.
Run 'tasks status' to see current state.

## SESSION CLOSE PROTOCOL

Before ending ANY session, you MUST complete ALL of these steps:

1. Log progress on active tasks:
   tasks log <id> "What you accomplished"

2. Verify artifacts are updated:
   - If you changed behavior: is help text / CLI output updated?
   - If you added features: is documentation current?
   - If you fixed bugs: do error messages reflect the fix?
   - Do new tests need to be written? Do existing tests need updating?
   Run the relevant commands to confirm outputs match the code.

3. Update task status:
   - tasks done <id>     # if complete
   - tasks block <id> "reason"  # if blocked

4. Add handoff context for next agent:
   tasks append <id> "Next steps: ..."

5. Update parent epic (if task is part of one):
   tasks append <epic-id> "Completed X, next: Y"

NEVER end a session without updating task state.
Work is NOT complete until tasks reflect reality.

## Core Rules

- Use 'tasks' for strategic work tracking (persists across sessions)
- Use TodoWrite for tactical within-session checklists
- Always claim work before starting: tasks start <id>
- Log progress frequently, not just at the end

## Essential Commands

# Finding work
tasks status              # Overview of all projects
tasks ready               # Tasks ready to work on (deps satisfied)
tasks show <id>           # Full details including logs
tasks graph               # Show dependency tree
tasks projects            # List all projects

Resuming a session? If a task is already in_progress, use 'tasks show <id>'
to read its logs and understand current state before continuing.

# Working
tasks start <id>          # Claim a task
tasks log <id> "message"  # Log progress
tasks done <id>           # Mark complete
tasks cancel <id> "why"   # Cancel (close without completing)
tasks block <id> "why"    # Mark blocked

# Creating & organizing
tasks add "title" -p project    # New task
tasks add "title" -e            # New epic
tasks add "title" --parent <epic-id>   # New task under epic
tasks add "title" --blocks <id>        # New task that blocks id
tasks parent <id> <epic-id>     # Set task's parent epic
tasks blocks <id> <other>       # id blocks other (other can't start until id done)

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
		fmt.Println("\n(No database connection - run 'tasks init' if needed)")
	}
}
