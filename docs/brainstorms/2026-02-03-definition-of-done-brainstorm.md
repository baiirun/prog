---
date: 2026-02-03
topic: definition-of-done
---

# Definition of Done for Agent-Driven Tasks

## What We're Building

A `definition_of_done` TEXT column on the items table that gives agents a deterministic heuristic for when work is complete. The field stores **pure natural language**—agents interpret and validate everything themselves.

**Example DoD:**
```
- Tests pass for affected code
- Run go test ./... and verify no failures
- No new linting errors
- Feature works as described in the ticket
- User can click button and accordion opens
- Documentation updated if public API changed
```

No special syntax. Agents read this, interpret the criteria, run any commands mentioned, verify outcomes, and only then call `prog done`.

## Why This Approach

We considered three approaches:

1. **Structured YAML with executable checks** - Adds parsing complexity, agents don't need it
2. **Separate relational tables** - Over-engineered for task metadata
3. **Embed in description** - No clear separation, harder to display/query

**Chosen: Dedicated column with natural language**

The enforcement mechanism is **prompting, not code**. Agents are instructed to:
1. Read DoD via `prog show <id>`
2. Validate their work against each criterion
3. Run any commands mentioned in the DoD
4. Only call `prog done` when all criteria pass

This keeps `prog done` simple (no auto-checks) while ensuring agents have clear completion criteria.

## Key Decisions

### Data Model
- **Dedicated column**: Add `definition_of_done TEXT` to items table
- **No inheritance**: Each task defines its own DoD (YAGNI)
- **Nullable**: DoD is optional; tasks without DoD work as before

### Format
- **Pure natural language**: No structured syntax, prefixes, or YAML
- **Agent-interpreted**: Agents reason about text, run commands as needed
- **No auto-execution**: `prog done` does not run checks; agent self-verifies

### CLI Commands

**Setting DoD at creation:**
```
prog add "Implement feature X" --dod "Tests pass; docs updated"
```

**Updating DoD later:**
```
prog edit <id> --dod "New criteria here"
```

Extends existing `edit` command rather than adding a new `prog dod` command.

### Agent Workflow Integration

**`prog show <id>` output** includes DoD section:
```
ts-abc123: Implement feature X
Status: in_progress
Priority: 2

Description:
  Add the new widget to the dashboard...

Definition of Done:
  - Tests pass for affected code
  - No regressions in existing functionality
  - User can see widget on dashboard
```

**`prog prime` output** updated to mention DoD:
```
## Before Completing Work

1. Review Definition of Done: prog show <id>
2. Verify each criterion is met
3. Run any commands mentioned in DoD
4. Only then: prog done <id>
```

## Decisions NOT Made (YAGNI)

- **No `prog verify` command**: Agents run checks themselves
- **No DoD templates**: Each task is explicit
- **No epic inheritance**: Keep it simple
- **No structured check parsing**: Natural language is sufficient

## Open Questions

None remaining—ready for planning.

## Next Steps

→ `/workflows:plan` for implementation details
