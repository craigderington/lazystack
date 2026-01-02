package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LogViewer represents the log viewer component
type LogViewer struct {
	viewport     viewport.Model
	title        string
	content      string
	width        int
	height       int
	autoScroll   bool
	ready        bool
}

// NewLogViewer creates a new log viewer
func NewLogViewer(title string) LogViewer {
	return LogViewer{
		title:      title,
		autoScroll: true,
		ready:      false,
	}
}

// SetSize sets the size of the log viewer
func (lv *LogViewer) SetSize(width, height int) {
	lv.width = width
	lv.height = height

	if !lv.ready {
		lv.viewport = viewport.New(width, height-4) // Account for title and help
		lv.viewport.Style = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)
		lv.ready = true
	} else {
		lv.viewport.Width = width
		lv.viewport.Height = height - 4
	}

	// Re-set content to trigger recalculation
	if lv.content != "" {
		lv.SetContent(lv.content)
	}
}

// SetContent sets the log content
func (lv *LogViewer) SetContent(content string) {
	lv.content = content
	lv.viewport.SetContent(content)

	if lv.autoScroll {
		lv.viewport.GotoBottom()
	}
}

// AppendContent appends new content to the logs
func (lv *LogViewer) AppendContent(newContent string) {
	if lv.content != "" {
		lv.content += "\n" + newContent
	} else {
		lv.content = newContent
	}
	lv.SetContent(lv.content)
}

// ToggleAutoScroll toggles auto-scrolling
func (lv *LogViewer) ToggleAutoScroll() {
	lv.autoScroll = !lv.autoScroll
	if lv.autoScroll {
		lv.viewport.GotoBottom()
	}
}

// Update handles messages
func (lv *LogViewer) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	lv.viewport, cmd = lv.viewport.Update(msg)
	return cmd
}

// View renders the log viewer
func (lv *LogViewer) View() string {
	if !lv.ready {
		return "Initializing log viewer..."
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Padding(0, 1)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")).
		Padding(0, 1)

	// Title
	title := titleStyle.Render(fmt.Sprintf("📜 %s", lv.title))

	// Help text
	helpText := "↑/↓: scroll • PgUp/PgDn: page • Home/End: top/bottom • a: toggle auto-scroll • esc: close"
	help := helpStyle.Render(helpText)

	// Status line
	scrollPercent := int(lv.viewport.ScrollPercent() * 100)
	autoScrollStatus := ""
	if lv.autoScroll {
		autoScrollStatus = "[AUTO] "
	}
	status := statusStyle.Render(fmt.Sprintf("%s%d%% (%d lines)", autoScrollStatus, scrollPercent, lv.viewport.TotalLineCount()))

	// Combine all parts
	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		lv.viewport.View(),
		status,
		help,
	)
}

// GetContent returns the current content
func (lv *LogViewer) GetContent() string {
	return lv.content
}

// Clear clears the log content
func (lv *LogViewer) Clear() {
	lv.content = ""
	lv.viewport.SetContent("")
}

// LineCount returns the number of lines in the logs
func (lv *LogViewer) LineCount() int {
	return len(strings.Split(lv.content, "\n"))
}

// IsAtBottom returns whether the viewport is scrolled to bottom
func (lv *LogViewer) IsAtBottom() bool {
	return lv.viewport.AtBottom()
}

// GotoBottom scrolls to the bottom
func (lv *LogViewer) GotoBottom() {
	lv.viewport.GotoBottom()
}

// GotoTop scrolls to the top
func (lv *LogViewer) GotoTop() {
	lv.viewport.GotoTop()
}
