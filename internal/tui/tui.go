// Package tui provides an interactive terminal UI for prog using Bubble Tea.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/baiirun/prog/internal/db"
	"github.com/baiirun/prog/internal/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view state.
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewDetail
)

// InputMode represents what kind of text input is active.
type InputMode int

const (
	InputNone    InputMode = iota
	InputBlock             // Entering block reason
	InputLog               // Entering log message
	InputCancel            // Entering cancel reason
	InputSearch            // Entering search text
	InputProject           // Entering project filter
	InputLabel             // Entering label filter
	InputAddDep            // Entering dependency ID to add
	InputCreate            // Entering new task title
)

// Status icons
const (
	iconOpen       = "○"
	iconInProgress = "◐"
	iconReviewing  = "◑"
	iconDone       = "●"
	iconBlocked    = "⊘"
	iconCanceled   = "✗"
)

// Layout constants
const (
	minSplitWidth = 80 // Minimum terminal width for split view
)

// FocusPane represents which pane is focused in split view.
type FocusPane int

const (
	FocusList FocusPane = iota
	FocusDetail
)

// Model is the main Bubble Tea model for the TUI.
type Model struct {
	db       *db.DB
	items    []model.Item // all items from db
	filtered []model.Item // items after filtering
	cursor   int
	viewMode ViewMode

	// Filter state
	filterProject  string
	filterStatuses map[model.Status]bool // which statuses to show
	filterSearch   string
	filterLabel    string // label filter (partial match, like search)

	// Input state
	inputMode  InputMode
	inputText  string
	inputLabel string

	// UI state
	width   int
	height  int
	err     error
	message string // temporary status message

	// Detail view state
	detailLogs []model.Log
	detailDeps []string

	// Split view state
	focusPane    FocusPane // Which pane is focused (list or detail)
	detailScroll int       // Scroll offset in detail pane
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("57"))

	statusColors = map[model.Status]lipgloss.Color{
		model.StatusOpen:       lipgloss.Color("252"),
		model.StatusInProgress: lipgloss.Color("214"),
		model.StatusReviewing:  lipgloss.Color("141"),
		model.StatusBlocked:    lipgloss.Color("196"),
		model.StatusDone:       lipgloss.Color("42"),
		model.StatusCanceled:   lipgloss.Color("245"),
	}

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	filterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))

	messageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	detailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("147"))

	// Content area padding
	contentPadding = 2
)

func statusIcon(s model.Status) string {
	switch s {
	case model.StatusOpen:
		return iconOpen
	case model.StatusInProgress:
		return iconInProgress
	case model.StatusReviewing:
		return iconReviewing
	case model.StatusDone:
		return iconDone
	case model.StatusBlocked:
		return iconBlocked
	case model.StatusCanceled:
		return iconCanceled
	default:
		return "?"
	}
}

// New creates a new TUI model with the given database connection.
func New(database *db.DB) Model {
	// Default: show open, in_progress, blocked, reviewing
	statuses := map[model.Status]bool{
		model.StatusOpen:       true,
		model.StatusInProgress: true,
		model.StatusBlocked:    true,
		model.StatusReviewing:  true,
		model.StatusDone:       false,
		model.StatusCanceled:   false,
	}
	return Model{
		db:             database,
		viewMode:       ViewList,
		filterStatuses: statuses,
	}
}

// Messages
type itemsMsg struct {
	items []model.Item
	err   error
}

type detailMsg struct {
	logs []model.Log
	deps []string
	id   string // Track which task this load was for (to ignore stale results)
	err  error
}

type actionMsg struct {
	message string
	err     error
}

// loadItems loads items from the database.
func (m Model) loadItems() tea.Cmd {
	return func() tea.Msg {
		items, err := m.db.ListItemsFiltered(db.ListFilter{})
		if err != nil {
			return itemsMsg{items: items, err: err}
		}
		// Populate labels for display
		if err := m.db.PopulateItemLabels(items); err != nil {
			return itemsMsg{items: items, err: err}
		}
		return itemsMsg{items: items, err: nil}
	}
}

// loadDetail loads logs and deps for current item.
func (m Model) loadDetail() tea.Cmd {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	id := m.filtered[m.cursor].ID
	return func() tea.Msg {
		logs, err := m.db.GetLogs(id)
		if err != nil {
			return detailMsg{id: id, err: err}
		}
		deps, err := m.db.GetDeps(id)
		if err != nil {
			return detailMsg{id: id, err: err}
		}
		return detailMsg{logs: logs, deps: deps, id: id}
	}
}

// applyFilters filters items based on current filter state.
func (m *Model) applyFilters() {
	m.filtered = nil
	for _, item := range m.items {
		// Status filter
		if !m.filterStatuses[item.Status] {
			continue
		}
		// Project filter (partial match)
		if m.filterProject != "" && !strings.Contains(strings.ToLower(item.Project), strings.ToLower(m.filterProject)) {
			continue
		}
		// Search filter
		if m.filterSearch != "" {
			search := strings.ToLower(m.filterSearch)
			if !strings.Contains(strings.ToLower(item.Title), search) &&
				!strings.Contains(strings.ToLower(item.ID), search) &&
				!strings.Contains(strings.ToLower(item.Description), search) {
				continue
			}
		}
		// Label filter (partial match, like search)
		if m.filterLabel != "" {
			found := false
			filter := strings.ToLower(m.filterLabel)
			for _, itemLabel := range item.Labels {
				if strings.Contains(strings.ToLower(itemLabel), filter) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		m.filtered = append(m.filtered, item)
	}
	// Adjust cursor
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return m.loadItems()
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Clear message on any key
		m.message = ""
		m.err = nil
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		oldWidth := m.width
		m.width = msg.Width
		m.height = msg.Height

		// Handle resize transitions
		// Narrow → Wide: load detail for current selection
		if oldWidth < minSplitWidth && m.width >= minSplitWidth && len(m.filtered) > 0 {
			return m, m.loadDetail()
		}
		// Narrow modal → Wide: close modal, show split view
		if m.viewMode == ViewDetail && m.width >= minSplitWidth {
			m.viewMode = ViewList
		}
		return m, nil

	case itemsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.items = msg.items
		m.applyFilters()
		// Auto-load detail in split view
		if m.width >= minSplitWidth && len(m.filtered) > 0 {
			return m, m.loadDetail()
		}
		return m, nil

	case detailMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		// Ignore stale results from previous cursor position
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) &&
			m.filtered[m.cursor].ID == msg.id {
			m.detailLogs = msg.logs
			m.detailDeps = msg.deps
		}
		return m, nil

	case actionMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.message = msg.message
		}
		return m, m.loadItems()
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle input mode first
	if m.inputMode != InputNone {
		return m.handleInputKey(msg)
	}

	switch m.viewMode {
	case ViewList:
		return m.handleListKey(msg)
	case ViewDetail:
		return m.handleDetailKey(msg)
	}
	return m, nil
}

func (m Model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = InputNone
		m.inputText = ""
		return m, nil

	case "enter":
		return m.submitInput()

	case "backspace":
		if len(m.inputText) > 0 {
			m.inputText = m.inputText[:len(m.inputText)-1]
			// Live filter for search, project, and label
			switch m.inputMode {
			case InputSearch:
				m.filterSearch = m.inputText
				m.applyFilters()
			case InputProject:
				m.filterProject = m.inputText
				m.applyFilters()
			case InputLabel:
				m.filterLabel = m.inputText
				m.applyFilters()
			}
		}

	default:
		// Add character if printable
		if len(msg.String()) == 1 {
			m.inputText += msg.String()
			// Live filter for search, project, and label
			switch m.inputMode {
			case InputSearch:
				m.filterSearch = m.inputText
				m.applyFilters()
			case InputProject:
				m.filterProject = m.inputText
				m.applyFilters()
			case InputLabel:
				m.filterLabel = m.inputText
				m.applyFilters()
			}
		}
	}
	return m, nil
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := m.inputText
	mode := m.inputMode
	m.inputMode = InputNone
	m.inputText = ""

	// Handle inputs that don't require an existing item
	switch mode {
	case InputSearch:
		m.filterSearch = text
		m.applyFilters()
		return m, nil

	case InputProject:
		m.filterProject = text
		m.applyFilters()
		return m, nil

	case InputLabel:
		m.filterLabel = text
		m.applyFilters()
		return m, nil

	case InputCreate:
		if text == "" {
			return m, nil
		}
		// Use the selected item's project if available
		var project string
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			project = m.filtered[m.cursor].Project
		}
		return m, func() tea.Msg {
			now := time.Now()
			newItem := &model.Item{
				ID:        model.GenerateID(model.ItemTypeTask),
				Project:   project,
				Type:      model.ItemTypeTask,
				Title:     text,
				Status:    model.StatusOpen,
				Priority:  2,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := m.db.CreateItem(newItem); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{message: fmt.Sprintf("Created %s", newItem.ID)}
		}
	}

	// Remaining inputs require an existing item
	if len(m.filtered) == 0 {
		return m, nil
	}
	item := m.filtered[m.cursor]

	switch mode {
	case InputBlock:
		if text == "" {
			return m, nil
		}
		return m, func() tea.Msg {
			if err := m.db.UpdateStatus(item.ID, model.StatusBlocked); err != nil {
				return actionMsg{err: err}
			}
			if err := m.db.AddLog(item.ID, "Blocked: "+text); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{message: fmt.Sprintf("Blocked %s", item.ID)}
		}

	case InputLog:
		if text == "" {
			return m, nil
		}
		return m, func() tea.Msg {
			if err := m.db.AddLog(item.ID, text); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{message: fmt.Sprintf("Logged to %s", item.ID)}
		}

	case InputCancel:
		return m, func() tea.Msg {
			if err := m.db.UpdateStatus(item.ID, model.StatusCanceled); err != nil {
				return actionMsg{err: err}
			}
			if text != "" {
				if err := m.db.AddLog(item.ID, "Canceled: "+text); err != nil {
					return actionMsg{err: err}
				}
			}
			return actionMsg{message: fmt.Sprintf("Canceled %s", item.ID)}
		}

	case InputAddDep:
		if text == "" {
			return m, nil
		}
		return m, func() tea.Msg {
			// text blocks current item
			if err := m.db.AddDep(item.ID, text); err != nil {
				return actionMsg{err: err}
			}
			return actionMsg{message: fmt.Sprintf("%s now blocks %s", text, item.ID)}
		}
	}

	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// In split view with detail focused, handle detail-specific navigation
	if m.width >= minSplitWidth && m.focusPane == FocusDetail {
		return m.handleDetailPaneKey(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		// Toggle focus between list and detail panes (only in split view)
		if m.width >= minSplitWidth {
			if m.focusPane == FocusList {
				m.focusPane = FocusDetail
			} else {
				m.focusPane = FocusList
			}
		}
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.detailScroll = 0 // Reset detail scroll on cursor change
			// Auto-load detail in split view
			if m.width >= minSplitWidth {
				return m, m.loadDetail()
			}
		}

	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.detailScroll = 0 // Reset detail scroll on cursor change
			// Auto-load detail in split view
			if m.width >= minSplitWidth {
				return m, m.loadDetail()
			}
		}

	case "g", "home":
		if m.cursor != 0 {
			m.cursor = 0
			m.detailScroll = 0
			// Auto-load detail in split view
			if m.width >= minSplitWidth {
				return m, m.loadDetail()
			}
		}

	case "G", "end":
		newCursor := max(0, len(m.filtered)-1)
		if m.cursor != newCursor {
			m.cursor = newCursor
			m.detailScroll = 0
			// Auto-load detail in split view
			if m.width >= minSplitWidth {
				return m, m.loadDetail()
			}
		}

	case "enter", "l":
		// In narrow mode, open full-screen detail view
		// In split view, focus the detail pane
		if m.width < minSplitWidth && len(m.filtered) > 0 {
			m.viewMode = ViewDetail
			return m, m.loadDetail()
		} else if m.width >= minSplitWidth {
			m.focusPane = FocusDetail
		}

	// Actions
	case "s":
		return m.doStart()
	case "d":
		return m.doDone()
	case "b":
		return m.startInput(InputBlock, "Block reason: ")
	case "L":
		return m.startInput(InputLog, "Log message: ")
	case "c":
		return m.startInput(InputCancel, "Cancel reason (optional): ")
	case "D":
		return m.doDelete()

	// Filtering
	case "/":
		return m.startInput(InputSearch, "Search: ")
	case "p":
		return m.startInput(InputProject, "Project: ")
	case "t":
		return m.startInput(InputLabel, "Label: ")
	case "1":
		m.filterStatuses[model.StatusOpen] = !m.filterStatuses[model.StatusOpen]
		m.applyFilters()
	case "2":
		m.filterStatuses[model.StatusInProgress] = !m.filterStatuses[model.StatusInProgress]
		m.applyFilters()
	case "3":
		m.filterStatuses[model.StatusBlocked] = !m.filterStatuses[model.StatusBlocked]
		m.applyFilters()
	case "4":
		m.filterStatuses[model.StatusReviewing] = !m.filterStatuses[model.StatusReviewing]
		m.applyFilters()
	case "5":
		m.filterStatuses[model.StatusDone] = !m.filterStatuses[model.StatusDone]
		m.applyFilters()
	case "6":
		m.filterStatuses[model.StatusCanceled] = !m.filterStatuses[model.StatusCanceled]
		m.applyFilters()
	case "0":
		// Show all
		for s := range m.filterStatuses {
			m.filterStatuses[s] = true
		}
		m.applyFilters()

	case "esc":
		// If filters are set, clear them; otherwise quit
		if m.filterSearch != "" || m.filterProject != "" || m.filterLabel != "" {
			m.filterSearch = ""
			m.filterProject = ""
			m.filterLabel = ""
			m.applyFilters()
		} else {
			return m, tea.Quit
		}

	case "r":
		return m, m.loadItems()

	// Dependencies
	case "a":
		return m.startInput(InputAddDep, "Add blocker ID: ")

	// Create
	case "n":
		label := "New task: "
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			if proj := m.filtered[m.cursor].Project; proj != "" {
				label = fmt.Sprintf("New task [%s]: ", proj)
			}
		}
		return m.startInput(InputCreate, label)
	}

	return m, nil
}

// handleDetailPaneKey handles keys when detail pane is focused in split view.
func (m Model) handleDetailPaneKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab", "esc", "h":
		// Return focus to list
		m.focusPane = FocusList
		return m, nil

	case "up", "k":
		// Scroll detail up
		if m.detailScroll > 0 {
			m.detailScroll--
		}

	case "down", "j":
		// Scroll detail down (will be bounded by content in detailView)
		m.detailScroll++

	case "g", "home":
		m.detailScroll = 0

	case "G", "end":
		// Scroll to bottom - set to large value, will be bounded in render
		m.detailScroll = 9999

	// Actions still work when detail is focused
	case "s":
		return m.doStart()
	case "d":
		return m.doDone()
	case "b":
		return m.startInput(InputBlock, "Block reason: ")
	case "L":
		return m.startInput(InputLog, "Log message: ")
	case "c":
		return m.startInput(InputCancel, "Cancel reason (optional): ")
	case "a":
		return m.startInput(InputAddDep, "Add blocker ID: ")
	}

	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc", "h", "backspace":
		m.viewMode = ViewList

	// Actions work in detail view too
	case "s":
		return m.doStart()
	case "d":
		return m.doDone()
	case "b":
		return m.startInput(InputBlock, "Block reason: ")
	case "L":
		return m.startInput(InputLog, "Log message: ")
	case "c":
		return m.startInput(InputCancel, "Cancel reason (optional): ")
	case "a":
		return m.startInput(InputAddDep, "Add blocker ID: ")

	case "r":
		return m, m.loadDetail()
	}

	return m, nil
}

func (m Model) startInput(mode InputMode, label string) (Model, tea.Cmd) {
	m.inputMode = mode
	m.inputLabel = label
	m.inputText = ""
	return m, nil
}

func (m Model) doStart() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	item := m.filtered[m.cursor]
	if item.Status != model.StatusOpen && item.Status != model.StatusBlocked {
		m.message = "Can only start open or blocked tasks"
		return m, nil
	}
	return m, func() tea.Msg {
		if err := m.db.UpdateStatus(item.ID, model.StatusInProgress); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: fmt.Sprintf("Started %s", item.ID)}
	}
}

func (m Model) doDone() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	item := m.filtered[m.cursor]
	if item.Status != model.StatusInProgress {
		m.message = "Can only complete in_progress tasks"
		return m, nil
	}
	return m, func() tea.Msg {
		if err := m.db.UpdateStatus(item.ID, model.StatusDone); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: fmt.Sprintf("Completed %s", item.ID)}
	}
}

func (m Model) doDelete() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	item := m.filtered[m.cursor]
	return m, func() tea.Msg {
		if err := m.db.DeleteItem(item.ID); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: fmt.Sprintf("Deleted %s", item.ID)}
	}
}

// View implements tea.Model.
func (m Model) View() string {
	var b strings.Builder

	switch m.viewMode {
	case ViewList:
		b.WriteString(m.listView())
	case ViewDetail:
		b.WriteString(m.detailView(0)) // 0 = full width
	}

	// Input line
	if m.inputMode != InputNone {
		b.WriteString("\n")
		b.WriteString(inputStyle.Render(m.inputLabel + m.inputText + "█"))
	}

	// Status message
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("Error: " + m.err.Error()))
	} else if m.message != "" {
		b.WriteString("\n")
		b.WriteString(messageStyle.Render(m.message))
	}

	// Apply padding to entire content
	padStyle := lipgloss.NewStyle().
		PaddingLeft(contentPadding).
		PaddingRight(contentPadding).
		PaddingTop(1)

	return padStyle.Render(b.String())
}

func (m Model) listView() string {
	// Check if we should show split view
	if m.width >= minSplitWidth {
		return m.splitView()
	}
	// Narrow terminal: show list only
	return m.renderListPane(m.width - (contentPadding * 2))
}

// splitView renders the split layout with list on left and details on right.
func (m Model) splitView() string {
	// Border colors
	focusedColor := lipgloss.Color("39")    // Blue for focused
	unfocusedColor := lipgloss.Color("241") // Dim gray for unfocused

	// Calculate pane dimensions
	// Each pane has: 1 border left + content + 1 border right
	// Plus 1 char gap between panes
	gap := 1
	borderChars := 4 // 2 per pane (left + right borders)
	availableWidth := m.width - borderChars - gap - (contentPadding * 2)
	leftContentWidth := availableWidth / 2
	rightContentWidth := availableWidth - leftContentWidth

	// Height: fill viewport
	// Account for: outer padding top (1), border top (1), border bottom (1), padding bottom (1)
	contentHeight := m.height - 4
	if contentHeight < 10 {
		contentHeight = 10
	}

	// Render content for each pane, passing the exact height available
	leftContent := m.renderListPaneWithHeight(leftContentWidth, contentHeight)
	rightContent := m.detailViewWithHeight(rightContentWidth, contentHeight)

	// Split into lines and normalize heights
	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	// Ensure exact height by padding/truncating
	leftLines = normalizeLines(leftLines, contentHeight, leftContentWidth)
	rightLines = normalizeLines(rightLines, contentHeight, rightContentWidth)

	// Determine border colors based on focus
	leftColor := unfocusedColor
	rightColor := unfocusedColor
	if m.focusPane == FocusList {
		leftColor = focusedColor
	} else {
		rightColor = focusedColor
	}

	// Build bordered panes manually
	leftBox := buildBorderedBox(leftLines, leftContentWidth, leftColor)
	rightBox := buildBorderedBox(rightLines, rightContentWidth, rightColor)

	// Join with gap
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, strings.Repeat(" ", gap), rightBox)
}

// normalizeLines ensures the slice has exactly `height` lines, each padded to `width`.
func normalizeLines(lines []string, height, width int) []string {
	result := make([]string, height)
	for i := 0; i < height; i++ {
		if i < len(lines) {
			result[i] = padToWidth(lines[i], width)
		} else {
			result[i] = strings.Repeat(" ", width)
		}
	}
	return result
}

// buildBorderedBox creates a box with rounded borders around content lines.
func buildBorderedBox(lines []string, contentWidth int, borderColor lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(borderColor)

	// Box drawing chars (rounded)
	topLeft := style.Render("╭")
	topRight := style.Render("╮")
	bottomLeft := style.Render("╰")
	bottomRight := style.Render("╯")
	horizontal := style.Render("─")
	vertical := style.Render("│")

	var b strings.Builder

	// Top border
	b.WriteString(topLeft)
	b.WriteString(strings.Repeat(horizontal, contentWidth))
	b.WriteString(topRight)
	b.WriteString("\n")

	// Content lines with side borders
	for _, line := range lines {
		b.WriteString(vertical)
		b.WriteString(line)
		b.WriteString(vertical)
		b.WriteString("\n")
	}

	// Bottom border
	b.WriteString(bottomLeft)
	b.WriteString(strings.Repeat(horizontal, contentWidth))
	b.WriteString(bottomRight)

	return b.String()
}

// padToWidth pads a string to the specified width with spaces.
// Accounts for ANSI escape codes when calculating visible width.
func padToWidth(s string, width int) string {
	visibleLen := lipgloss.Width(s)
	if visibleLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visibleLen)
}

// renderListPane renders the list content for a given width (uses default height calc).
func (m Model) renderListPane(width int) string {
	height := m.height - 8
	if height < 10 {
		height = 15
	}
	return m.renderListPaneWithHeight(width, height)
}

// renderListPaneWithHeight renders the list content for given width and height.
func (m Model) renderListPaneWithHeight(width, height int) string {
	var b strings.Builder

	// Header (takes ~2 lines)
	title := "prog"
	b.WriteString(titleStyle.Render(title))
	b.WriteString(fmt.Sprintf("  %d/%d items", len(m.filtered), len(m.items)))

	// Active filters (truncate if needed for narrow pane)
	filters := m.activeFiltersString()
	if filters != "" {
		if len(filters) > width-20 && width > 30 {
			filters = filters[:width-23] + "..."
		}
		b.WriteString("  ")
		b.WriteString(filterStyle.Render(filters))
	}
	b.WriteString("\n\n")

	// Footer takes 3 lines (blank + 2 help lines)
	// So items get: height - 2 (header) - 3 (footer) = height - 5
	itemsHeight := height - 5
	if itemsHeight < 3 {
		itemsHeight = 3
	}

	// Items
	if len(m.filtered) == 0 {
		b.WriteString("No items match filters\n")
	} else {
		// Calculate visible window - keep cursor in view
		start := 0
		if m.cursor >= itemsHeight {
			start = m.cursor - itemsHeight + 1
		}
		end := min(start+itemsHeight, len(m.filtered))

		rowWidth := width
		if rowWidth < 40 {
			rowWidth = 40
		}

		for i := start; i < end; i++ {
			item := m.filtered[i]
			selected := i == m.cursor

			if selected {
				// For selected row: plain text, then apply highlight to full width
				line := m.formatItemLinePlain(item, rowWidth)
				b.WriteString(selectedRowStyle.Width(rowWidth).Render(line))
			} else {
				// For non-selected: use styled version, also constrained to width
				line := m.formatItemLineStyled(item, rowWidth)
				b.WriteString(lipgloss.NewStyle().Width(rowWidth).Render(line))
			}
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	if m.width >= minSplitWidth {
		// Split view footer - show Tab hint and current focus
		if m.focusPane == FocusList {
			b.WriteString(helpStyle.Render("j/k:nav  tab:focus detail  s:start d:done L:log n:new"))
		} else {
			b.WriteString(helpStyle.Render("j/k:scroll  tab:focus list  s:start d:done L:log"))
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("/:search p:project 1-6:status  q:quit"))
	} else {
		// Full width footer
		b.WriteString(helpStyle.Render("j/k:nav  enter:detail  s:start d:done b:block L:log c:cancel n:new"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("/:search p:project t:label 1-6:status 0:all  a:add-dep  r:refresh q:quit"))
	}

	return b.String()
}

// formatItemLinePlain returns a plain text line without any ANSI styling.
// Used for selected rows where we apply a single highlight style.
func (m Model) formatItemLinePlain(item model.Item, width int) string {
	icon := statusIcon(item.Status)

	// Format: icon id title [label1] [label2] [project]
	project := ""
	projectWidth := 0
	if item.Project != "" {
		project = "[" + item.Project + "]"
		projectWidth = len(project) + 1
	}

	// Build labels string
	labels := ""
	labelsWidth := 0
	for _, lbl := range item.Labels {
		labels += " [" + lbl + "]"
		labelsWidth += len(lbl) + 3 // brackets + space
	}

	// Calculate available space for title
	// icon(1) + space(1) + id(9) + space(2) + labels + project + space = ~14 + labels + project
	titleWidth := width - 14 - labelsWidth - projectWidth
	if titleWidth < 20 {
		titleWidth = 40
	}

	title := item.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-3] + "..."
	}

	return fmt.Sprintf("%s %s  %-*s%s %s", icon, item.ID, titleWidth, title, labels, project)
}

// formatItemLineStyled returns a styled line with colors for non-selected rows.
func (m Model) formatItemLineStyled(item model.Item, width int) string {
	icon := statusIcon(item.Status)
	color := statusColors[item.Status]
	iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)

	id := dimStyle.Render(item.ID)

	// Format: icon id title [label1] [label2] [project]
	project := ""
	projectWidth := 0
	if item.Project != "" {
		project = dimStyle.Render("[" + item.Project + "]")
		projectWidth = len(item.Project) + 3 // brackets + space
	}

	// Build labels string
	labels := ""
	labelsWidth := 0
	for _, lbl := range item.Labels {
		labels += " " + labelStyle.Render("["+lbl+"]")
		labelsWidth += len(lbl) + 3 // brackets + space
	}

	// Calculate available space for title
	// icon(1) + space(1) + id(9) + space(2) + labels + project + space = ~14 + labels + project
	titleWidth := width - 14 - labelsWidth - projectWidth
	if titleWidth < 20 {
		titleWidth = 40
	}

	title := item.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-3] + "..."
	}

	return fmt.Sprintf("%s %s  %-*s%s %s", iconStyled, id, titleWidth, title, labels, project)
}

func (m Model) activeFiltersString() string {
	var parts []string

	// Status filter
	var statuses []string
	for s, active := range m.filterStatuses {
		if active {
			statuses = append(statuses, string(s)[:1]) // First char: o/i/b/d/c
		}
	}
	if len(statuses) < 6 {
		parts = append(parts, "status:"+strings.Join(statuses, ""))
	}

	if m.filterProject != "" {
		parts = append(parts, "project:"+m.filterProject)
	}

	if m.filterSearch != "" {
		parts = append(parts, "search:\""+m.filterSearch+"\"")
	}

	if m.filterLabel != "" {
		parts = append(parts, "label:\""+m.filterLabel+"\"")
	}

	return strings.Join(parts, " ")
}

// detailView renders the detail pane. If width is 0, uses full terminal width.
// If width > 0, constrains rendering to that width (for split view) and applies scroll.
func (m Model) detailView(width int) string {
	return m.detailViewWithHeight(width, 0)
}

// detailViewWithHeight renders the detail pane with explicit height constraint.
func (m Model) detailViewWithHeight(width, height int) string {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return "No task selected"
	}

	item := m.filtered[m.cursor]
	var lines []string

	// Helper to truncate text if we're in constrained width mode
	truncate := func(s string, maxLen int) string {
		if width == 0 || len(s) <= maxLen {
			return s
		}
		if maxLen <= 3 {
			return "..."
		}
		return s[:maxLen-3] + "..."
	}

	// Calculate effective width for content
	effectiveWidth := width
	if effectiveWidth == 0 {
		effectiveWidth = m.width - (contentPadding * 2)
	}
	if effectiveWidth < 40 {
		effectiveWidth = 40
	}

	// Title with status icon
	icon := statusIcon(item.Status)
	color := statusColors[item.Status]
	iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)
	title := item.Title
	if width > 0 {
		title = truncate(title, effectiveWidth-4) // Leave room for icon and spacing
	}
	lines = append(lines, iconStyled+" "+titleStyle.Render(title))
	lines = append(lines, "")

	lines = append(lines, detailLabelStyle.Render("ID:       ")+item.ID)
	lines = append(lines, detailLabelStyle.Render("Type:     ")+string(item.Type))
	lines = append(lines, detailLabelStyle.Render("Project:  ")+truncate(item.Project, effectiveWidth-10))

	statusStyled := lipgloss.NewStyle().Foreground(color).Render(string(item.Status))
	lines = append(lines, detailLabelStyle.Render("Status:   ")+statusStyled)
	lines = append(lines, detailLabelStyle.Render("Priority: ")+fmt.Sprintf("%d", item.Priority))

	if item.ParentID != nil {
		lines = append(lines, detailLabelStyle.Render("Parent:   ")+*item.ParentID)
	}

	// Labels
	if len(item.Labels) > 0 {
		labelsStr := ""
		for i, lbl := range item.Labels {
			if i > 0 {
				labelsStr += " "
			}
			labelsStr += labelStyle.Render("[" + lbl + "]")
		}
		lines = append(lines, detailLabelStyle.Render("Labels:   ")+truncate(labelsStr, effectiveWidth-10))
	}

	// Dependencies
	if len(m.detailDeps) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabelStyle.Render("Blocked by:"))
		for _, dep := range m.detailDeps {
			lines = append(lines, "  "+dimStyle.Render("→")+" "+dep)
		}
	}

	// Description
	if item.Description != "" {
		lines = append(lines, "")
		lines = append(lines, detailLabelStyle.Render("Description:"))
		// Split description into lines
		desc := item.Description
		descLines := strings.Split(desc, "\n")
		for _, dl := range descLines {
			if width > 0 && len(dl) > effectiveWidth {
				dl = truncate(dl, effectiveWidth)
			}
			lines = append(lines, dl)
		}
	}

	// Logs
	if len(m.detailLogs) > 0 {
		lines = append(lines, "")
		lines = append(lines, detailLabelStyle.Render("Logs:"))
		for _, log := range m.detailLogs {
			ts := dimStyle.Render(log.CreatedAt.Format("2006-01-02 15:04"))
			msg := log.Message
			if width > 0 {
				msg = truncate(msg, effectiveWidth-20) // Leave room for timestamp
			}
			lines = append(lines, "  "+ts+" "+msg)
		}
	}

	// For full-screen detail view (width == 0), just return all content
	if width == 0 {
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("esc:back  s:start d:done b:block L:log c:cancel a:add-dep  q:quit"))
		return strings.Join(lines, "\n")
	}

	// For split view, apply scroll offset and height constraint
	totalLines := len(lines)
	visibleHeight := height
	if visibleHeight <= 0 {
		visibleHeight = totalLines // No height constraint
	}

	// Bound scroll to valid range (can't scroll past content)
	maxScroll := max(0, totalLines-visibleHeight)
	scroll := m.detailScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Apply scroll offset - show window of lines
	start := scroll
	end := min(start+visibleHeight, totalLines)
	if start < len(lines) {
		lines = lines[start:end]
	}

	return strings.Join(lines, "\n")
}

// Run starts the TUI.
func Run(database *db.DB) error {
	m := New(database)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
