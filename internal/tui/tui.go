// Package tui provides an interactive terminal UI for prog using Bubble Tea.
package tui

import (
	"fmt"
	"strings"

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
	InputAddDep            // Entering dependency ID to add
)

// Status icons
const (
	iconOpen       = "○"
	iconInProgress = "◐"
	iconDone       = "●"
	iconBlocked    = "⊘"
	iconCanceled   = "✗"
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

	// Content area padding
	contentPadding = 2
)

func statusIcon(s model.Status) string {
	switch s {
	case model.StatusOpen:
		return iconOpen
	case model.StatusInProgress:
		return iconInProgress
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
	// Default: show open, in_progress, blocked
	statuses := map[model.Status]bool{
		model.StatusOpen:       true,
		model.StatusInProgress: true,
		model.StatusBlocked:    true,
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
		return itemsMsg{items: items, err: err}
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
			return detailMsg{err: err}
		}
		deps, err := m.db.GetDeps(id)
		if err != nil {
			return detailMsg{err: err}
		}
		return detailMsg{logs: logs, deps: deps}
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
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case itemsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.items = msg.items
		m.applyFilters()
		return m, nil

	case detailMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.detailLogs = msg.logs
		m.detailDeps = msg.deps
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
			// Live filter for search and project
			switch m.inputMode {
			case InputSearch:
				m.filterSearch = m.inputText
				m.applyFilters()
			case InputProject:
				m.filterProject = m.inputText
				m.applyFilters()
			}
		}

	default:
		// Add character if printable
		if len(msg.String()) == 1 {
			m.inputText += msg.String()
			// Live filter for search and project
			switch m.inputMode {
			case InputSearch:
				m.filterSearch = m.inputText
				m.applyFilters()
			case InputProject:
				m.filterProject = m.inputText
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

	case InputSearch:
		m.filterSearch = text
		m.applyFilters()

	case InputProject:
		m.filterProject = text
		m.applyFilters()

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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}

	case "g", "home":
		m.cursor = 0

	case "G", "end":
		m.cursor = max(0, len(m.filtered)-1)

	case "enter", "l":
		if len(m.filtered) > 0 {
			m.viewMode = ViewDetail
			return m, m.loadDetail()
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
		m.filterStatuses[model.StatusDone] = !m.filterStatuses[model.StatusDone]
		m.applyFilters()
	case "5":
		m.filterStatuses[model.StatusCanceled] = !m.filterStatuses[model.StatusCanceled]
		m.applyFilters()
	case "0":
		// Show all
		for s := range m.filterStatuses {
			m.filterStatuses[s] = true
		}
		m.applyFilters()

	case "esc":
		// Clear filters
		m.filterSearch = ""
		m.filterProject = ""
		m.applyFilters()

	case "r":
		return m, m.loadItems()

	// Dependencies
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
		b.WriteString(m.detailView())
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
	var b strings.Builder

	// Header
	title := "prog"
	b.WriteString(titleStyle.Render(title))
	b.WriteString(fmt.Sprintf("  %d/%d items", len(m.filtered), len(m.items)))

	// Active filters
	filters := m.activeFiltersString()
	if filters != "" {
		b.WriteString("  ")
		b.WriteString(filterStyle.Render(filters))
	}
	b.WriteString("\n\n")

	// Items
	if len(m.filtered) == 0 {
		b.WriteString("No items match filters\n")
	} else {
		visibleHeight := m.height - 8
		if visibleHeight < 5 {
			visibleHeight = 15
		}
		start := 0
		if m.cursor >= visibleHeight {
			start = m.cursor - visibleHeight + 1
		}
		end := min(start+visibleHeight, len(m.filtered))

		rowWidth := m.width - (contentPadding * 2)
		if rowWidth < 60 {
			rowWidth = 80
		}

		for i := start; i < end; i++ {
			item := m.filtered[i]
			selected := i == m.cursor

			if selected {
				// For selected row: plain text, then apply highlight to full width
				line := m.formatItemLinePlain(item, rowWidth)
				b.WriteString(selectedRowStyle.Width(rowWidth).Render(line))
			} else {
				// For non-selected: use styled version
				line := m.formatItemLineStyled(item, rowWidth)
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k:nav  enter:detail  s:start d:done b:block L:log c:cancel"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("/:search p:project 1-5:status 0:all  a:add-dep  r:refresh q:quit"))

	return b.String()
}

// formatItemLinePlain returns a plain text line without any ANSI styling.
// Used for selected rows where we apply a single highlight style.
func (m Model) formatItemLinePlain(item model.Item, width int) string {
	icon := statusIcon(item.Status)

	// Format: icon id title [project]
	project := ""
	projectWidth := 0
	if item.Project != "" {
		project = "[" + item.Project + "]"
		projectWidth = len(project) + 1
	}

	// Calculate available space for title
	// icon(1) + space(1) + id(9) + space(2) + project + space = ~14 + project
	titleWidth := width - 14 - projectWidth
	if titleWidth < 20 {
		titleWidth = 40
	}

	title := item.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-3] + "..."
	}

	return fmt.Sprintf("%s %s  %-*s %s", icon, item.ID, titleWidth, title, project)
}

// formatItemLineStyled returns a styled line with colors for non-selected rows.
func (m Model) formatItemLineStyled(item model.Item, width int) string {
	icon := statusIcon(item.Status)
	color := statusColors[item.Status]
	iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)

	id := dimStyle.Render(item.ID)

	// Format: icon id title [project]
	project := ""
	projectWidth := 0
	if item.Project != "" {
		project = dimStyle.Render("[" + item.Project + "]")
		projectWidth = len(item.Project) + 3 // brackets + space
	}

	// Calculate available space for title
	// icon(1) + space(1) + id(9) + space(2) + project + space = ~14 + project
	titleWidth := width - 14 - projectWidth
	if titleWidth < 20 {
		titleWidth = 40
	}

	title := item.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-3] + "..."
	}

	return fmt.Sprintf("%s %s  %-*s %s", iconStyled, id, titleWidth, title, project)
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
	if len(statuses) < 5 {
		parts = append(parts, "status:"+strings.Join(statuses, ""))
	}

	if m.filterProject != "" {
		parts = append(parts, "project:"+m.filterProject)
	}

	if m.filterSearch != "" {
		parts = append(parts, "search:\""+m.filterSearch+"\"")
	}

	return strings.Join(parts, " ")
}

func (m Model) detailView() string {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return "No item selected"
	}

	item := m.filtered[m.cursor]
	var b strings.Builder

	// Title with status icon
	icon := statusIcon(item.Status)
	color := statusColors[item.Status]
	iconStyled := lipgloss.NewStyle().Foreground(color).Render(icon)
	b.WriteString(iconStyled + " " + titleStyle.Render(item.Title) + "\n\n")

	b.WriteString(detailLabelStyle.Render("ID:       ") + item.ID + "\n")
	b.WriteString(detailLabelStyle.Render("Type:     ") + string(item.Type) + "\n")
	b.WriteString(detailLabelStyle.Render("Project:  ") + item.Project + "\n")

	statusStyled := lipgloss.NewStyle().Foreground(color).Render(string(item.Status))
	b.WriteString(detailLabelStyle.Render("Status:   ") + statusStyled + "\n")
	b.WriteString(detailLabelStyle.Render("Priority: ") + fmt.Sprintf("%d", item.Priority) + "\n")

	if item.ParentID != nil {
		b.WriteString(detailLabelStyle.Render("Parent:   ") + *item.ParentID + "\n")
	}

	// Dependencies
	if len(m.detailDeps) > 0 {
		b.WriteString("\n" + detailLabelStyle.Render("Blocked by:") + "\n")
		for _, dep := range m.detailDeps {
			b.WriteString("  " + dimStyle.Render("→") + " " + dep + "\n")
		}
	}

	// Description
	if item.Description != "" {
		b.WriteString("\n" + detailLabelStyle.Render("Description:") + "\n")
		b.WriteString(item.Description + "\n")
	}

	// Logs
	if len(m.detailLogs) > 0 {
		b.WriteString("\n" + detailLabelStyle.Render("Logs:") + "\n")
		for _, log := range m.detailLogs {
			ts := dimStyle.Render(log.CreatedAt.Format("2006-01-02 15:04"))
			b.WriteString("  " + ts + " " + log.Message + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("esc:back  s:start d:done b:block L:log c:cancel a:add-dep  q:quit"))

	return b.String()
}

// Run starts the TUI.
func Run(database *db.DB) error {
	m := New(database)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
