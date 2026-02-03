---
title: "feat: Add Definition of Done field for agent-driven task completion"
type: feat
date: 2026-02-03
---

# feat: Add Definition of Done field for agent-driven task completion

## Overview

Add a `definition_of_done` TEXT column to the items table that provides agents with a deterministic heuristic for when work is complete. The field stores pure natural language criteria that agents interpret and verify before calling `prog done`.

## Problem Statement / Motivation

Currently, agents running autonomously have no structured way to know when a task is truly complete. They rely on implicit understanding of the task description, which can lead to:

- Premature completion (missing edge cases)
- Over-engineering (doing more than needed)
- Inconsistent completion standards across tasks

A Definition of Done (DoD) gives explicit, verifiable criteria that agents can check before marking work done.

## Proposed Solution

Add a nullable `definition_of_done` TEXT column to tasks. Agents read this via `prog show`, verify each criterion, and only then call `prog done`. The enforcement is through prompting, not code—`prog done` does not auto-verify.

**Example usage:**
```bash
# Create with DoD
prog add "Implement user auth" --dod "Tests pass; No security warnings; Docs updated"

# Update DoD
prog edit ts-abc123 --dod "Tests pass; Login works; Logout works"

# View (prog show includes DoD section)
prog show ts-abc123

# Clear DoD
prog edit ts-abc123 --dod ""
```

## Technical Considerations

### Schema Migration

Increment `SchemaVersion` from 2 to 3 and add migration:

```sql
ALTER TABLE items ADD COLUMN definition_of_done TEXT;
```

### Files to Modify

| File | Changes |
|------|---------|
| `internal/db/db.go` | Add migration, increment version |
| `internal/model/item.go` | Add `DefinitionOfDone *string` field |
| `internal/db/items.go` | Update `CreateItem`, `GetItem` |
| `internal/db/queries.go` | Update `queryItems` SELECT |
| `cmd/prog/main.go` | Add `--dod` flag to add/edit, update show/ready output, update prime |

### Output Formatting

**prog show:**
```
ID:          ts-abc123
Title:       Implement user auth
Status:      in_progress
...

Definition of Done:
  - Tests pass for affected code
  - No security warnings from linter
  - Documentation updated
```

**prog ready** (indicator only):
```
Ready tasks:
  [ts-abc123] Implement user auth       [DoD]
  [ts-def456] Fix login bug
```

**prog prime** (add guidance):
```
## Before Completing Work

1. Review Definition of Done: prog show <id>
2. Verify each criterion is met
3. Run any commands mentioned in DoD
4. Only then: prog done <id>
```

## Acceptance Criteria

- [ ] Schema migration adds `definition_of_done` column
- [ ] `prog add --dod "criteria"` sets DoD at creation
- [ ] `prog edit <id> --dod "criteria"` updates existing DoD
- [ ] `prog edit <id> --dod ""` clears DoD (sets to NULL)
- [ ] `prog show <id>` displays DoD section when present
- [ ] `prog ready` shows `[DoD]` indicator for tasks with DoD
- [ ] `prog prime` includes DoD verification guidance
- [ ] Existing tasks without DoD continue to work unchanged
- [ ] Tests cover add, edit, show, ready with DoD

## Success Metrics

- Agents consistently check DoD before calling `prog done`
- Reduction in premature task completion
- Clear audit trail of completion criteria per task

## Dependencies & Risks

**Dependencies:**
- None—uses existing patterns

**Risks:**
- **Low:** Multi-line DoD input via CLI may be awkward. Mitigation: Users can use `prog edit <id>` (opens editor) for complex DoD.
- **Low:** Long DoD may wrap awkwardly in terminal. Mitigation: Indent continuation lines.

## Implementation Tasks

1. **Schema & Model** (~15 min)
   - Add migration to `db.go`
   - Add field to `Item` struct in `model/item.go`

2. **DB Operations** (~20 min)
   - Update `CreateItem` in `items.go`
   - Update `GetItem` in `items.go`
   - Update `queryItems` in `queries.go`
   - Add `SetDefinitionOfDone` function

3. **CLI Commands** (~30 min)
   - Add `--dod` flag declaration
   - Register flag for `addCmd` and `editCmd`
   - Handle flag in add command's `RunE`
   - Handle flag in edit command's `RunE`

4. **Output Updates** (~20 min)
   - Update `printItemDetail` for show output
   - Update `prog ready` output with `[DoD]` indicator
   - Update `printPrimeContent` with DoD guidance

5. **Tests** (~30 min)
   - Test add with `--dod`
   - Test edit with `--dod` (set and clear)
   - Test show output includes DoD
   - Test ready output includes indicator
   - Test migration on existing DB

## References & Research

### Internal References
- Schema pattern: `internal/db/db.go:120-143`
- Model pattern: `internal/model/item.go:53-65`
- Flag pattern: `cmd/prog/main.go:2061-2065`
- Show output: `cmd/prog/main.go:2221-2266`
- Prime output: `cmd/prog/main.go:2569-2699`

### Brainstorm
- `docs/brainstorms/2026-02-03-definition-of-done-brainstorm.md`
