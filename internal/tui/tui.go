package tui

import (
	"strings"

	"codeberg.org/snonux/timr/internal/config"
	"codeberg.org/snonux/timr/internal/worktime"
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

	entries EntriesModel
	report  ReportModel
	timer   TimerModel

	entriesErr string
	reportErr  string
}

// NewModel creates a new root TUI model.
func NewModel() *Model {
	model, _ := NewModelWithConfig(config.Default())
	return model
}

// NewModelWithConfig creates a data-backed root model from config.
func NewModelWithConfig(cfg config.Config) (*Model, error) {
	model := &Model{
		activeTab: tabEntries,
		styles:    DefaultStyles(),
		entries:   NewEntriesModel(nil),
		report:    NewReportModel(nil),
	}

	entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
	if err != nil {
		model.entriesErr = err.Error()
	} else {
		model.entries.SetEntries(entries)
		model.entries.SetPersistence(cfg.WorktimeDBDir)
		weeks, reportErr := worktime.BuildReport(entries, cfg)
		if reportErr != nil {
			model.reportErr = reportErr.Error()
		} else {
			model.report.SetWeeks(weeks)
		}
	}

	timerModel, timerErr := NewTimerModel("doom", cfg)
	if timerErr != nil {
		model.timer = newFallbackTimerModel("timer init error: " + timerErr.Error())
	} else {
		model.timer = timerModel
	}

	return model, nil
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	if m.activeTab == tabTimer && m.timer.state.Running {
		return timerTick()
	}
	return nil
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		bodyWidth, bodyHeight := m.bodySize()
		m.entries.SetSize(bodyWidth, bodyHeight)
		m.report.SetSize(bodyWidth, bodyHeight)
		m.timer.SetSize(bodyWidth, bodyHeight)
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
				return m, m.nextTab()
			case "T":
				return m, m.prevTab()
			}
		}

		switch key {
		case "tab":
			return m, m.nextTab()
		case "1":
			return m, m.switchTab(tabEntries)
		case "2":
			return m, m.switchTab(tabReport)
		case "3":
			return m, m.switchTab(tabTimer)
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "g":
			m.pendingG = true
			return m, nil
		case "Z":
			m.pendingZ = true
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m.updateActiveTab(msg)
}

// View implements tea.Model.
func (m *Model) View() string {
	header := m.renderTabs()
	body := m.renderBody()

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

func (m *Model) nextTab() tea.Cmd {
	return m.switchTab((m.activeTab + 1) % tabCount)
}

func (m *Model) prevTab() tea.Cmd {
	next := m.activeTab - 1
	if next < 0 {
		next = tabCount - 1
	}
	return m.switchTab(next)
}

func (m *Model) renderTabs() string {
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

func (m *Model) renderBody() string {
	switch m.activeTab {
	case tabEntries:
		if m.entriesErr != "" {
			return m.styles.Body.Render("Entries\n\nFailed to load entries: " + m.entriesErr)
		}
		return m.entries.View(m.styles)
	case tabReport:
		if m.entriesErr != "" {
			return m.styles.Body.Render("Report\n\nUnavailable because entries failed to load: " + m.entriesErr)
		}
		if m.reportErr != "" {
			return m.styles.Body.Render("Report\n\nFailed to build report: " + m.reportErr)
		}
		return m.report.View(m.styles)
	case tabTimer:
		return m.timer.View()
	default:
		return ""
	}
}

func newFallbackTimerModel(status string) TimerModel {
	return TimerModel{
		helpStyle:   lipgloss.NewStyle().Faint(true),
		timerStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF")),
		statusStyle: lipgloss.NewStyle().Italic(true),
		font:        "doom",
		work: workIntegration{
			status: status,
		},
	}
}

func (m *Model) switchTab(next tab) tea.Cmd {
	m.activeTab = next
	if m.activeTab == tabTimer && m.timer.state.Running {
		return timerTick()
	}
	return nil
}

func (m *Model) bodySize() (int, int) {
	width := m.width - 4
	height := m.height - 6

	if width < 20 {
		width = m.width
	}
	if height < 6 {
		height = m.height
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func (m *Model) updateActiveTab(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.activeTab {
	case tabEntries:
		updated, cmd := m.entries.Update(msg)
		m.entries = updated
		return m, cmd
	case tabReport:
		updated, cmd := m.report.Update(msg)
		m.report = updated
		return m, cmd
	case tabTimer:
		updatedModel, cmd := m.timer.Update(msg)
		if updated, ok := updatedModel.(TimerModel); ok {
			m.timer = updated
		}
		return m, cmd
	default:
		return m, nil
	}
}
