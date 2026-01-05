# tasks

Lightweight task management for AI agents. SQLite-backed, CLI-driven.

## Install

```bash
go install github.com/byronguina/tasks-go/cmd/tasks@latest
```

## Usage

```bash
# Initialize database
tasks init

# Create tasks
tasks add "Build auth system" --project=gaia
tasks add -e "Auth MVP" --project=gaia  # epic

# See what's ready
tasks ready --project=gaia

# Work on a task
tasks start <id>
tasks log "Implemented token generation"
tasks done

# Mark blocked
tasks block "Waiting on API design"

# Overview
tasks status --project=gaia
```

## Design

See [DESIGN.md](DESIGN.md) for architecture and schema details.

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
