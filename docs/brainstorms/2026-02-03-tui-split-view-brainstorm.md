---
date: 2026-02-03
topic: tui-split-view
---

# TUI Split View: Side-by-Side List and Details

## What We're Building

A split-pane TUI layout that shows the task list on the left and task details on the right simultaneously. This eliminates the current enter/esc navigation pattern where users must switch between full-screen list and detail views.

The list pane remains the active pane—arrow keys navigate tasks and the details pane auto-updates to show the currently selected task. A vertical line separates the two panels.

## Why This Approach

**Approaches considered:**

1. **Embed details in ViewList (chosen)** — Modify the existing `listView()` to render both panels side-by-side. Remove `ViewDetail` mode entirely. Simplest implementation, reduces code.

2. **New ViewSplit mode** — Add a third view mode composing existing views. More modular but adds code paths.

3. **Layout container abstraction** — Create reusable split helper. Over-engineering for single use case (YAGNI).

**Why Approach 1:** Aligns with the goal of simplifying navigation. Removing a view mode is better than adding one. The current `detailView()` rendering logic can be reused with minor refactoring to accept a width parameter.

## Key Decisions

### Layout & Proportions
- **Panel split**: Fixed 50/50. Simple and predictable.
- **Panel separator**: Vertical line border using box-drawing characters (`│`).
- **Minimum width**: 80 columns for split view.

### Navigation & Focus
- **Active pane**: List always active. No pane switching needed.
- **Keyboard shortcuts**: No new shortcuts. Existing shortcuts work as-is.
- **Enter key in narrow mode**: Opens full-screen detail view (preserves modal behavior as fallback).

### Narrow Terminal Behavior (< 80 columns)
- Show list only, hide details pane entirely.
- Press `enter` to view details full-screen (modal fallback preserved for this case).
- This means `ViewDetail` mode is kept but only used for narrow terminals.

### Details Pane Content
- **Content order**: Keep current layout (ID, type, project, status, priority, parent, labels, deps, description, logs).
- **Overflow handling**: Truncate with `...` indicator when content exceeds pane height. No scrolling—keeps it simple.
- **Empty state**: Show "No task selected" hint text when list is empty.

### View Mode Strategy
- Replace modal as the default for normal-width terminals.
- Retain `ViewDetail` mode only for narrow terminal fallback.

## Technical Notes

Current implementation details (from repo research):
- Framework: Bubble Tea + Lipgloss
- Main file: `internal/tui/tui.go` (937 lines)
- Current modes: `ViewList`, `ViewDetail`
- Key styles: `selectedRowStyle`, `detailLabelStyle`, `helpStyle`
- Use `lipgloss.JoinHorizontal()` for the split layout

## Open Questions

None—all decisions made.

## Next Steps

→ `/workflows:plan` for implementation details
