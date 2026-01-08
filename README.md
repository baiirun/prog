# prog

Lightweight task management for AI agents. SQLite-backed, CLI-driven.

## Install

### Homebrew (macOS/Linux)

```bash
brew install baiirun/tap/prog
```

### Go install

Requires Go 1.25+.

```bash
go install github.com/baiirun/prog/cmd/prog@latest
```

### Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/baiirun/prog.git
cd prog
go build -o prog ./cmd/prog
./prog --help
```

Or install to your `$GOBIN`:

```bash
git clone https://github.com/baiirun/prog.git
cd prog
go install ./cmd/prog
prog --help
```

## Quick Start

```bash
# Initialize database (creates ~/.prog/prog.db)
prog init

# Set up Claude Code hooks (recommended)
prog onboard

# Create a task
prog add "Implement user authentication" -p myproject --priority 1
# Output: ts-a1b2c3

# See what's ready to work on
prog ready -p myproject

# Start working
prog start ts-a1b2c3

# Log progress
prog log ts-a1b2c3 "Added JWT token generation"

# Mark complete
prog done ts-a1b2c3
```

## CLI Reference

### Core Commands

| Command | Description |
|---------|-------------|
| `prog init` | Initialize the database |
| `prog onboard` | Set up prog integration for AI agents |
| `prog add <title>` | Create a task (returns ID) |
| `prog list` | List all tasks |
| `prog show <id>` | Show task details, logs, and dependencies |
| `prog ready` | Show tasks ready for work (open + deps met) |
| `prog status` | Project overview for agent spin-up |
| `prog prime` | Output context for Claude Code hooks |
| `prog tui` | Launch interactive terminal UI (alias: `prog ui`) |

### Work Commands

| Command | Description |
|---------|-------------|
| `prog start <id>` | Set task to in_progress |
| `prog done <id>` | Mark task complete |
| `prog cancel <id> [reason]` | Cancel task (close without completing) |
| `prog block <id> <reason>` | Mark blocked with reason |
| `prog log <id> <message>` | Add timestamped log entry |
| `prog append <id> <text>` | Append to task description |
| `prog desc <id> <text>` | Replace task description |
| `prog edit <id>` | Edit description in $PROG_EDITOR (defaults to nvim, nano, vi) |

### Organization

| Command | Description |
|---------|-------------|
| `prog parent <id> <epic-id>` | Set task's parent epic |
| `prog blocks <id> <other>` | Add blocking relationship (other blocked until id done) |
| `prog graph` | Show dependency graph |
| `prog projects` | List all projects |
| `prog add -e <title>` | Create an epic instead of task |

### Flags

| Flag | Commands | Description |
|------|----------|-------------|
| `-p, --project` | all | Filter/set project scope |
| `-e, --epic` | add | Create epic instead of task |
| `--priority` | add | Priority: 1=high, 2=medium (default), 3=low |
| `--parent` | add, list | Set parent epic at creation / filter by parent |
| `--blocks` | add | Set task this will block at creation |
| `--status` | list | Filter by status |
| `--type` | list | Filter by item type (task, epic) |
| `--blocking` | list | Show items that block the given ID |
| `--blocked-by` | list | Show items blocked by the given ID |
| `--has-blockers` | list | Show only items with unresolved blockers |
| `--no-blockers` | list | Show only items with no blockers |
| `--all` | status | Show all ready tasks (default: limit to 10) |

## ID Format

IDs are auto-generated with type prefixes:
- `ts-XXXXXX` — tasks (e.g., `ts-a1b2c3`)
- `ep-XXXXXX` — epics (e.g., `ep-f0a20b`)

## Agent Workflow

### Spin-up (new agent joining)

```bash
# Get project overview
prog status -p myproject

# Output:
# Project: myproject
#
# Summary: 3 open, 1 in progress, 0 blocked, 2 done (2 ready)
#
# In progress:
#   [ts-a1b2c3] Implement auth middleware
#
# Ready for work:
#   [ts-d4e5f6] Add login endpoint (pri 1)
#   [ts-g7h8i9] Write auth tests (pri 2)
```

### Pick up work

```bash
# See what's unblocked
prog ready -p myproject

# Read full context
prog show ts-d4e5f6

# Claim it
prog start ts-d4e5f6
```

### While working

```bash
# Log progress (timestamped audit trail)
prog log ts-d4e5f6 "Implemented POST /login"
prog log ts-d4e5f6 "Added rate limiting"

# Append to description with decisions/context
prog append ts-d4e5f6 "Using bcrypt for password hashing"

# Replace description entirely
prog desc ts-d4e5f6 "Implement login endpoint with JWT auth and rate limiting"

# Or edit in your editor
prog edit ts-d4e5f6
```

### Finish or hand off

```bash
# Complete
prog done ts-d4e5f6

# Or cancel if no longer needed
prog cancel ts-d4e5f6 "Requirements changed"

# Or mark blocked for next agent
prog block ts-d4e5f6 "Need API spec for OAuth flow"

# If task is part of an epic, update the epic too
prog append ep-a1b2c3 "Completed auth endpoint, next: write tests"
```

### Dependencies

Use dependencies to enforce task ordering. A task with unmet dependencies won't appear in `prog ready`.

```bash
# Create a task that blocks another (at creation time)
prog add "Build API" -p myproject --blocks ts-frontend
# New task blocks ts-frontend, so ts-frontend can't start until the new task is done

# Or add blocking relationship to existing tasks
prog blocks ts-backend ts-frontend

# View all dependencies
prog graph

# Output:
# ts-frontend [open] Build frontend components
#   └── ts-backend [in_progress] Implement API endpoints
```

The `ready` command automatically filters out tasks with unmet dependencies, so agents only see work they can actually start.

### Epics

Group related tasks under an epic for organization:

```bash
# Create an epic
prog add "Authentication system" -p myproject -e
# Output: ep-a1b2c3

# Create task under epic (at creation time)
prog add "Implement login" -p myproject --parent ep-a1b2c3

# Or assign existing tasks to the epic
prog parent ts-d4e5f6 ep-a1b2c3
prog parent ts-g7h8i9 ep-a1b2c3

# View task with parent
prog show ts-d4e5f6
# Output includes: Parent: ep-a1b2c3
```

## Claude Code Integration

The `prog onboard` command configures Claude Code hooks to inject workflow context at session start and before context compaction. This ensures agents maintain context about the prog workflow across sessions.

**Using a different agent?** (Cursor, Opencode, Droid, Codex, Gemini, etc.)

1. Copy the Task Tracking snippet from `CLAUDE.md` to your agent's instruction file (`.cursorrules`, `AGENTS.md`, etc.)
2. If your tool supports hooks, add `prog prime` to session start
3. If no hooks, run `prog prime` and paste output into agent context

### Hook Configuration

Running `prog onboard` adds this to your Claude Code settings (`.claude/settings.json`):

```json
{
  "hooks": {
    "SessionStart": [
      { "command": "prog prime" }
    ],
    "PreCompact": [
      { "command": "prog prime" }
    ]
  }
}
```

### What `prog prime` outputs

- **SESSION CLOSE PROTOCOL**: Mandatory checklist for logging progress and updating status before ending sessions
- **Core Rules**: When to use `prog` (strategic, cross-session) vs TodoWrite (tactical, within-session)
- **Essential Commands**: Quick reference grouped by workflow phase
- **Current State**: Live summary of open, in-progress, and blocked tasks

This ensures agents never forget the workflow, even after context compaction.

## Interactive TUI

Launch with `prog tui` (or `prog ui`):

```
prog  12/47 items  status:oib

◐ ts-234d9f  Set up Bubble Tea scaffold       [tasks]
○ ts-9566cd  Task list view with indicators   [tasks]
○ ts-f39592  Vim keybind navigation           [tasks]
```

### Navigation

| Key | Action |
|-----|--------|
| `j/k` or arrows | Move up/down |
| `g/G` or Home/End | Jump to first/last |
| `enter` or `l` | View task details |
| `esc` or `h` | Go back to list |
| `q` | Quit |

### Actions

| Key | Action |
|-----|--------|
| `s` | Start task |
| `d` | Mark done |
| `b` | Block (prompts for reason) |
| `L` | Log progress (prompts for message) |
| `c` | Cancel task |
| `D` | Delete task |
| `a` | Add dependency |
| `r` | Refresh |

### Filtering

| Key | Action |
|-----|--------|
| `/` | Search by title/ID/description |
| `p` | Filter by project (partial match) |
| `1-5` | Toggle status: 1=open 2=in_progress 3=blocked 4=done 5=canceled |
| `0` | Show all statuses |
| `esc` | Clear filters |

## Data Model

- **Items**: Tasks or epics with title, description, status, priority
- **Status**: `open` → `in_progress` → `done` (or `blocked`, `canceled`)
- **Dependencies**: Task A can depend on Task B (A is blocked until B is done)
- **Logs**: Timestamped audit trail per item
- **Projects**: String tag to scope work (e.g., "gaia", "myapp")

Database location: `~/.prog/prog.db`

## Goals

1. Track tasks within larger work (epics)
2. Progress reports for current work and what's left
3. Split work for parallel agents
4. Track dependencies for ordering
5. Prioritize work
6. Store context so agents can resume where others left off

## Non-Goals

- Git sync (single-player, local only)
- Multiplayer / collaboration
- Complex workflows

## License

MIT
