# Tasks System Design

A lightweight task management system for agents. SQLite-backed, CLI-driven.

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
- Complex workflows (molecules, convoys, etc.)

---

## Architecture

```
Agent
  ↓ (CLI calls)
tasks CLI
  ↓ (reads/writes)
SQLite database
```

Single database at `.world/tasks/tasks.db`. No file watcher, no sync layer, no daemon.

---

## Schema

```sql
CREATE TABLE items (
  id TEXT PRIMARY KEY,
  project TEXT NOT NULL,
  type TEXT NOT NULL,              -- task, epic
  title TEXT NOT NULL,
  description TEXT,                -- definition + context + handoff notes
  status TEXT NOT NULL DEFAULT 'open',  -- open, in_progress, blocked, done
  priority INTEGER DEFAULT 2,      -- 1=high, 2=medium, 3=low
  parent_id TEXT REFERENCES items(id),
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE deps (
  item_id TEXT REFERENCES items(id),
  depends_on TEXT REFERENCES items(id),
  PRIMARY KEY (item_id, depends_on)
);

CREATE TABLE logs (
  id INTEGER PRIMARY KEY,
  item_id TEXT REFERENCES items(id),
  message TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Notes

- `description` is a living document: task definition + current state + handoff notes for next agent
- `logs` is optional audit trail (timestamped history)
- `deps` tracks blocking dependencies; children of epics are parallel by default
- `project` scopes work to areas: gaia, geogenesis, eldspire, zaum, etc.

---

## CLI

```
tasks init                        # create tasks.db
tasks add <title>                 # create task, returns id
tasks add -e <title>              # create epic
tasks list [--status=X] [--project=X]
tasks ready [--project=X]         # unblocked tasks by priority
tasks show <id>                   # full item detail
tasks start <id>                  # claim, set in_progress
tasks log <message>               # append to current task's log
tasks block <reason>              # set blocked + log reason
tasks done [id]                   # complete task
tasks append <id> <text>          # append to description
tasks dep <id> --on <other>       # add dependency
tasks context [project]           # get/set default project
tasks status [--project=X]        # overview for agent spin-up
```

---

## Agent Workflow

### Spin-up

```bash
tasks status --project=gaia
```

Returns:
- Last completed work
- In-progress tasks (possibly abandoned)
- Blocked tasks with reasons
- Ready tasks by priority

### Picking up work

```bash
tasks ready --project=gaia        # see what's unblocked
tasks show <id>                   # read full context
tasks start <id>                  # claim it
```

### Working

```bash
tasks log "Implemented X"         # append to log
tasks append <id> "Decision: using Y because Z"  # update description
```

### Finishing or handing off

```bash
tasks done <id>                   # complete
# or
tasks block "Stuck on X, next agent should try Y"
```

---

## Ready Work Calculation

A task is "ready" when:
1. Status is `open` (not in_progress, blocked, or done)
2. All dependencies are `done`
3. Parent (if any) is not `blocked`

```sql
SELECT * FROM items
WHERE status = 'open'
  AND id NOT IN (
    SELECT item_id FROM deps
    WHERE depends_on IN (SELECT id FROM items WHERE status != 'done')
  )
ORDER BY priority, created_at;
```

---

## Open Questions

- **Stale in_progress detection**: How to know if an agent died mid-task? Could add `updated_at` check, or explicit heartbeat, or just let humans/agents manually unblock.
- **ID generation**: Human-readable slugs from title? Or short hashes like beads (`bd-a1b2`)?
- **Project validation**: Enforce known projects or allow any string?

---

## Future Additions (Not MVP)

- Companion markdown files for rich human-editable content
- Export/import for backup
- TUI for browsing work
- Integration with Zaum for cross-linking
- Convoys (cross-project work tracking, like Gas Town)
