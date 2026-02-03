---
title: "feat: TUI Split View - Side-by-Side List and Details"
type: feat
date: 2026-02-03
brainstorm: docs/brainstorms/2026-02-03-tui-split-view-brainstorm.md
---

# ✨ feat: TUI Split View - Side-by-Side List and Details

## Overview

Add a split-pane layout to the TUI that shows the task list on the left and task details on the right simultaneously. This eliminates the current enter/esc navigation pattern for normal-width terminals while preserving modal behavior as a fallback for narrow terminals.

## Problem Statement / Motivation

Currently, users must press `enter` to view task details and `esc` to return to the list. This back-and-forth interrupts workflow and makes it harder to scan through tasks while viewing their details. A split view provides immediate context without mode switching.

## Proposed Solution

Modify `listView()` in `internal/tui/tui.go` to render both list and detail panes side-by-side using `lipgloss.JoinHorizontal()`. The detail pane auto-updates when the cursor moves. Retain `ViewDetail` mode only for narrow terminal fallback.

**Key behaviors:**
- Fixed 50/50 split with vertical line separator (`│`)
- List pane always active (no focus switching)
- Detail loads on cursor movement; stale results ignored (simpler than cancellation)
- Terminals < 80 columns: show list only, `enter` opens full-screen modal
- Detail overflow truncates with `...` indicator

## Technical Considerations

### Architecture

**Files to modify:**
- `internal/tui/tui.go` - Main changes

**Functions to modify:**
- `listView()` (line 698) - Add split layout rendering
- `detailView()` (line 863) - Refactor to accept width parameter
- `handleListKey()` (line 478) - Modify enter behavior based on width
- `Update()` (line 266) - Handle resize transitions, ignore stale detail loads

**Constants to add:**
```go
const minSplitWidth = 80  // Minimum terminal width for split view
```

**Modified message type:**
```go
type detailMsg struct {
    logs []model.Log
    deps []string
    id   string  // Track which task this is for
    err  error
}
```

### Width Calculation

```
Total width: 80 columns
Left pane:   39 columns (floor((80-1)/2))
Separator:    1 column  (│)
Right pane:  40 columns (remainder)
Padding:      2 columns each side (contentPadding)
```

### Detail Loading Strategy

**Ignore-stale-results pattern** (simpler than cancellation):
1. On cursor move, fire `loadDetail()` with the task ID
2. When result arrives, check if it matches current selection
3. If IDs match, update detail view; if not, ignore the result

This avoids the complexity of Bubble Tea command cancellation while solving the actual problem (stale data display).

### Resize Handling

| Transition | Behavior |
|------------|----------|
| Wide → Narrow | Hide detail pane, preserve loaded data |
| Narrow → Wide | Show split, trigger `loadDetail()` for current selection |
| Narrow modal → Wide | Close modal, show split view |

### Performance

- No debouncing needed—ignore-stale pattern handles rapid navigation
- Truncate detail content at render time, not load time
- Keep existing log/dep data structure (no pagination needed for v1)

### Truncation Behavior

For v1, detail content that exceeds pane height is truncated with `...` at the bottom. Users with many logs (50+) will see recent logs only. This is acceptable for v1; scrollable detail pane can be added later if needed.

### Security

N/A - internal tooling, no new attack surface.

## Acceptance Criteria

### Functional
- [ ] Split view renders when terminal ≥ 80 columns
- [ ] List pane on left, detail pane on right with `│` separator
- [ ] 50/50 width split (±1 column for odd widths)
- [ ] Detail auto-updates on cursor movement (j/k/↑/↓)
- [ ] Stale detail loads are ignored (rapid navigation shows correct task)
- [ ] "No task selected" shown when list is empty
- [ ] Detail content truncates with `...` when exceeding pane height
- [ ] `enter` key does nothing in split view mode

### Narrow Terminal Fallback
- [ ] Terminals < 80 columns show list only (no detail pane)
- [ ] `enter` opens full-screen detail modal in narrow mode
- [ ] `esc`/`h`/`backspace` closes modal and returns to list

### Resize Behavior
- [ ] Resize from ≥80 to <80: detail pane hides seamlessly
- [ ] Resize from <80 to ≥80: split view appears, detail loads automatically
- [ ] Resize while in narrow modal: modal closes, split view shows

### Edge Cases
- [ ] Empty task list: both panes show appropriate empty state
- [ ] Single task: works correctly
- [ ] Very long task title: truncates in list pane
- [ ] Very long description: truncates in detail pane
- [ ] Terminal exactly 80 columns: split view works
- [ ] Terminal exactly 79 columns: narrow mode activates

## Success Metrics

- Users can view task details without pressing enter/esc
- No regression in list navigation speed
- Narrow terminal users have unchanged experience

## Dependencies & Risks

**Dependencies:**
- None - all functionality exists in current codebase

**Risks:**
- **Width calculation edge cases**: Test thoroughly at boundary widths (79, 80, 81)
- **Lipgloss JoinHorizontal alignment**: Ensure both panes have equal line counts

## Implementation Suggestions

### Step 1: Add constant and modify `detailMsg`

```go
const minSplitWidth = 80

type detailMsg struct {
    logs []model.Log
    deps []string
    id   string  // Add: track which task this load was for
    err  error
}
```

### Step 2: Refactor `detailView()` to accept width

```go
// Before
func (m Model) detailView() string {

// After  
func (m Model) detailView(width int) string {
    // Use width for truncation and wrapping
    // Truncate content that exceeds available height with "..."
}
```

### Step 3: Extract list rendering and add split layout

```go
// Extract list rendering to reusable function
func (m Model) renderListPane(width int) string {
    // Existing list rendering logic, parameterized by width
}

func (m Model) listView() string {
    if m.width >= minSplitWidth {
        // Split view
        leftWidth := (m.width - 1) / 2  // -1 for separator
        rightWidth := m.width - leftWidth - 1
        
        left := m.renderListPane(leftWidth)
        
        // Render separator using Lipgloss (not strings.Repeat)
        separator := lipgloss.NewStyle().
            Width(1).
            Render(strings.Repeat("│\n", m.height-2))
        
        right := m.detailView(rightWidth)
        
        return lipgloss.JoinHorizontal(lipgloss.Top, left, separator, right)
    }
    // Narrow: full-width list
    return m.renderListPane(m.width)
}
```

### Step 4: Modify `handleListKey()` for enter behavior

```go
case "enter", "l":
    if m.width < minSplitWidth && len(m.filtered) > 0 {
        // Narrow mode: open modal
        m.viewMode = ViewDetail
        return m, m.loadDetail()
    }
    // Wide mode: no-op (detail already visible)
    return m, nil
```

### Step 5: Update `loadDetail()` to include task ID

```go
func (m Model) loadDetail() tea.Cmd {
    if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
        return nil
    }
    id := m.filtered[m.cursor].ID
    return func() tea.Msg {
        logs, err := m.db.GetLogs(id)
        if err != nil {
            return detailMsg{err: err, id: id}
        }
        deps, err := m.db.GetDeps(id)
        return detailMsg{logs: logs, deps: deps, id: id, err: err}
    }
}
```

### Step 6: Handle stale results in `Update()`

```go
case detailMsg:
    // Ignore stale results from previous cursor position
    if len(m.filtered) > 0 && m.cursor < len(m.filtered) &&
       m.filtered[m.cursor].ID == msg.id {
        m.detailLogs = msg.logs
        m.detailDeps = msg.deps
    }
    return m, nil
```

### Step 7: Handle resize transitions in `Update()`

```go
case tea.WindowSizeMsg:
    oldWidth := m.width
    m.width = msg.Width
    m.height = msg.Height
    
    // Transition narrow → wide: load detail for current selection
    if oldWidth < minSplitWidth && m.width >= minSplitWidth && len(m.filtered) > 0 {
        return m, m.loadDetail()
    }
    // Transition narrow modal → wide: close modal, show split
    if m.viewMode == ViewDetail && m.width >= minSplitWidth {
        m.viewMode = ViewList
    }
    return m, nil
```

### Step 8: Trigger detail load on cursor movement

In `handleListKey()`, after any navigation that changes cursor:

```go
case "j", "down":
    if m.cursor < len(m.filtered)-1 {
        m.cursor++
        if m.width >= minSplitWidth {
            return m, m.loadDetail()  // Auto-load in split view
        }
    }
    return m, nil
```

## Testing Checklist

### Manual Tests
- [ ] Launch with 0 tasks
- [ ] Launch with 1 task  
- [ ] Launch with 100+ tasks
- [ ] Navigate rapidly (hold down j key)
- [ ] Resize from 120 → 60 columns
- [ ] Resize from 60 → 120 columns
- [ ] Filter to 0 results, then clear filter
- [ ] Task with very long title (100+ chars)
- [ ] Task with 50+ log entries
- [ ] Terminal exactly 80 columns wide
- [ ] Terminal exactly 79 columns wide

### Unit Tests
```go
func TestSplitWidthCalculation(t *testing.T) {
    tests := []struct {
        width     int
        wantLeft  int
        wantRight int
    }{
        {80, 39, 40},
        {81, 40, 40},
        {100, 49, 50},
        {79, 0, 0},  // Below threshold, no split
    }
    for _, tt := range tests {
        // Test width calculation logic
    }
}
```

## References & Research

### Internal References
- `internal/tui/tui.go:698` - Current `listView()` implementation
- `internal/tui/tui.go:863` - Current `detailView()` implementation  
- `internal/tui/tui.go:193` - Current `loadDetail()` implementation
- `internal/tui/tui.go:78-124` - Lipgloss style definitions

### External References
- [Lipgloss JoinHorizontal docs](https://github.com/charmbracelet/lipgloss#joining-paragraphs)
- [Bubble Tea architecture](https://github.com/charmbracelet/bubbletea)

### Related Work
- Brainstorm: `docs/brainstorms/2026-02-03-tui-split-view-brainstorm.md`
