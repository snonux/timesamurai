package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tab int

const (
	tabEntries tab = iota
	tabReport
	tabTimer
	tabCount
)

var tabLabels = []string{"Entries", "Report", "Timer"}

// Model is the root TUI scaffold model.
type Model struct {
	activeTab tab
	width     int
	height    int

	showHelp bool
	pendingG bool
	pendingZ bool

	styles Styles
}

// NewModel creates a new root TUI model.
func NewModel() Model {
	return Model{
		activeTab: tabEntries,
		styles:    DefaultStyles(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		key := msg.String()

		if m.pendingZ {
			m.pendingZ = false
			if key == "Q" {
				return m, tea.Quit
			}
		}

		if m.pendingG {
			m.pendingG = false
			switch key {
			case "t":
				m.nextTab()
				return m, nil
			case "T":
				m.prevTab()
				return m, nil
			}
		}

		switch key {
		case "tab":
			m.nextTab()
		case "1":
			m.activeTab = tabEntries
		case "2":
			m.activeTab = tabReport
		case "3":
			m.activeTab = tabTimer
		case "?":
			m.showHelp = !m.showHelp
		case "g":
			m.pendingG = true
		case "Z":
			m.pendingZ = true
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	header := m.renderTabs()
	body := m.styles.Body.Render(m.renderBody())

	help := m.styles.Hint.Render("Press ? for help")
	if m.showHelp {
		help = m.styles.Help.Render("Tab/gt/gT/1/2/3 switch tabs, ? toggles help, q or ZQ quits")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, help)
	rendered := m.styles.App.Render(content)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, rendered)
	}
	return rendered
}

func (m *Model) nextTab() {
	m.activeTab = (m.activeTab + 1) % tabCount
}

func (m *Model) prevTab() {
	if m.activeTab == 0 {
		m.activeTab = tabCount - 1
		return
	}
	m.activeTab--
}

func (m Model) renderTabs() string {
	parts := make([]string, 0, len(tabLabels))
	for idx, label := range tabLabels {
		if tab(idx) == m.activeTab {
			parts = append(parts, m.styles.ActiveTab.Render(label))
			continue
		}
		parts = append(parts, m.styles.Tab.Render(label))
	}
	return m.styles.Header.Render(strings.Join(parts, " "))
}

func (m Model) renderBody() string {
	switch m.activeTab {
	case tabEntries:
		return "Entries screen scaffold.\nList/search/edit wiring lands in next tasks."
	case tabReport:
		return "Report screen scaffold.\nWeekly report table wiring lands in next tasks."
	case tabTimer:
		return "Timer screen scaffold.\nLive timer integration lands in next tasks."
	default:
		return ""
	}
}
