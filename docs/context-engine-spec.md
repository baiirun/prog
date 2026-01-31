# Context Engine v2 (Index + Markdown Learnings)

## Summary
Move canonical learnings into repo-local Markdown pages and use `prog` as a fast indexing layer. Agents see lightweight stubs (one-liner + short summary + path) and pull full pages only on demand. This enables progressive disclosure, portability, and high agent usability.

## Goals
- Keep canonical knowledge in Markdown files inside the working repo.
- Keep agent context light via stubs, with explicit on-demand expansion.
- Make discovery and updates fast for agents ("always available").
- Preserve provenance and quality via indexing metadata.

## Non-goals
- `prog` is not the source of truth for learning content.
- Do not require a centralized knowledge base outside the repo.
- No implicit full-content loading without explicit signal or high confidence.

## Key Concepts
- **Learning Page**: Markdown page containing a learning (or a set of related learnings).
- **Index Stub**: Minimal metadata stored by `prog` (one-liner + summary + path + tags).
- **Progressive Disclosure**: Default to stubs; load full content only when needed.

## Learning Page Standard
Location: repo-local, preferably `docs/learnings/` or near relevant code.

Required template:
- Title
- Problem (what failed or was slow)
- Pattern / Rule (actionable guidance)
- Example (concrete usage)
- Caveats / When to ignore
- References (issues, PRs, tasks)

Example frontmatter (optional but recommended):
```
---
tags: [db, sqlite, migrations]
updated: 2026-01-21
---
```

## Index Stub Schema (prog)
- `path` (repo-relative)
- `title`
- `one_liner` (<= 120 chars)
- `summary` (1–3 sentences)
- `tags` (optional)
- `updated_at`
- `provenance` (task id / issue / PR / commit)

## Retrieval Behavior
1. **Phase 1 (Stub Only)**: return `one_liner + summary + path` for top matches.
2. **Phase 2 (Full Content)**: load page only if:
   - agent explicitly requests it, or
   - match confidence exceeds a threshold (configurable), or
   - the agent is updating a related area.

## Prompting & UX (Agent Behavior)

### Agent Contract (must-follow rules)
- Always check the index before asking for global context.
- When a relevant stub appears, open the Markdown page before making changes.
- If new learning emerges, update an existing page or create a new one.
- Re-index immediately after page changes.
- Do not duplicate learnings in `prog`; keep canonical text in Markdown.

### System Prompt Additions (suggested)
```
You have access to a context index. Always search it first.
If a stub looks relevant, open the referenced Markdown page and read it fully.
When you produce a new learning, write it into a repo-local Markdown page,
then re-index so future agents can find it. The index stores only stubs.
```

### UX Commands (examples)
- `prog context search <query>` -> returns stubs
- `prog context open <id>` -> opens Markdown path
- `prog context upsert <path>` -> re-indexes a page
- `prog context reindex` -> re-indexes changed pages

## Availability & Usability
- Index queries must be fast (<200ms local target).
- Stubs must be readable in a single screen.
- Prefer deterministic paths and stable IDs for stubs.

## Quality Gates
- Reject indexing if template sections are missing.
- Require at least one Example and one Caveat.

## Curation & Relevance Feedback
The index should optimize for how well stubs match agent expectations, not raw usage frequency.

Signals are captured only when the agent explicitly evaluates a stub:
- `expected_relevant`: agent believed the stub was relevant before opening.
- `actually_relevant`: after reading the page, did it apply to the task?
- `mismatch_reason` (optional): why it failed (scope mismatch, stale info, misleading tags, wrong subsystem).
- `task_context` (optional): task type or scope tags to make feedback contextual.

Use these signals to:
- Penalize stubs that frequently look relevant but are not.
- Improve ranking and tag quality over time.

## Metrics
- Stub-to-open ratio (indicates progressive disclosure efficiency)
- Token usage savings
- Time-to-first-relevant-context
- Reuse rate per learning page

## Migration Plan (high level)
1. Create `docs/learnings/` and seed with top existing learnings.
2. Build indexer that scans repo pages and stores stubs.
3. Update agent prompts and CLI commands.
4. Enforce template and quality gates in the indexer.
