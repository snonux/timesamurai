package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	timesamurai "codeberg.org/snonux/timesamurai/internal"
	"codeberg.org/snonux/timesamurai/internal/config"
	"codeberg.org/snonux/timesamurai/internal/worktime"
)

type tab int

const (
	tabEntries tab = iota
	tabReport
	tabTimer
	tabCount
)

var tabLabels = []string{"Entries", "Report", "Timer"}

type rootTimerTickMsg struct{}

func rootTimerTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return rootTimerTickMsg{}
	})
}

// Model is the root TUI scaffold model.
type Model struct {
	activeTab tab
	width     int
	height    int

	showHelp    bool
	confirmQuit bool
	pendingG    bool
	pendingZ    bool

	styles Styles
	theme  Theme
	disco  bool

	entries EntriesModel
	report  ReportModel
	timer   TimerModel

	entriesErr string
	reportErr  string

	timerTickScheduled bool
}

// NewModel creates a new root TUI model.
func NewModel() *Model {
	model, _ := NewModelWithConfig(config.Default())
	return model
}

// NewModelWithConfig creates a data-backed root model from config.
func NewModelWithConfig(cfg config.Config) (*Model, error) {
	return NewModelWithConfigAndDisco(cfg, false)
}

// NewModelWithConfigAndDisco creates a data-backed root model and optionally enables disco mode.
func NewModelWithConfigAndDisco(cfg config.Config, disco bool) (*Model, error) {
	theme := DefaultTheme()
	model := &Model{
		activeTab: tabEntries,
		styles:    StylesFromTheme(theme),
		theme:     theme,
		disco:     disco,
		entries:   NewEntriesModel(nil),
		report:    NewReportModel(nil),
	}

	entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
	if err != nil {
		model.entriesErr = err.Error()
	} else {
		host, hostErr := cfg.EffectiveHostname()
		if hostErr != nil {
			host = strings.TrimSpace(cfg.Hostname)
		}
		model.entries.SetEntries(entries)
		model.entries.SetPersistence(cfg.WorktimeDBDir, host)
		weeks, reportErr := worktime.BuildReport(entries, cfg)
		if reportErr != nil {
			model.reportErr = reportErr.Error()
		} else {
			model.report.SetWeeks(weeks)
		}
		if warning := reportOpenSessionWarning(entries); warning != "" {
			model.report.SetWarning(warning)
		}
	}

	timerModel, timerErr := NewTimerModel("doom", cfg)
	if timerErr != nil {
		model.timer = newFallbackTimerModel("timer init error: " + timerErr.Error())
	} else {
		model.timer = timerModel
	}

	if model.disco {
		model.randomizeTheme()
	}

	return model, nil
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return m.startRootTimerTicker()
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case rootTimerTickMsg:
		m.timerTickScheduled = false
		return m, m.startRootTimerTicker()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		bodyWidth, bodyHeight := m.bodySize()
		m.entries.SetSize(bodyWidth, bodyHeight)
		m.report.SetSize(bodyWidth, bodyHeight)
		m.timer.SetSize(bodyWidth, bodyHeight)
		return m, nil

	case tea.KeyPressMsg:
		key := msg.String()
		entriesCapturingText := m.activeTab == tabEntries && m.entries.capturesTextInput()

		if m.confirmQuit {
			switch key {
			case "s":
				if err := m.entries.savePendingChanges(); err != nil {
					m.entries.setStatusError("Save failed: " + err.Error())
					m.confirmQuit = false
					return m, nil
				}
				m.confirmQuit = false
				return m, tea.Quit
			case "d", "n":
				m.confirmQuit = false
				return m, tea.Quit
			case "esc":
				m.confirmQuit = false
				return m, nil
			default:
				return m, nil
			}
		}

		if entriesCapturingText {
			m.pendingG = false
		}

		if m.pendingZ {
			m.pendingZ = false
			if key == "Q" {
				return m.requestQuit()
			}
		}

		if m.pendingG && !entriesCapturingText {
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
		case "?", "H":
			m.showHelp = !m.showHelp
			return m, nil
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
		case "c":
			m.randomizeTheme()
			return m, nil
		case "C":
			m.resetTheme()
			return m, nil
		case "x":
			m.disco = !m.disco
			if m.disco {
				m.randomizeTheme()
			}
			return m, nil
		case "g":
			if entriesCapturingText {
				break
			}
			m.pendingG = true
			return m, nil
		case "Z":
			m.pendingZ = true
			return m, nil
		case "q", "ctrl+c":
			return m.requestQuit()
		}
	}

	return m.updateActiveTab(msg)
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	header := m.renderTabs()
	body := m.renderBody()
	status := m.renderStatusLine()

	if m.confirmQuit {
		body = m.styles.Help.Render(strings.Join([]string{
			"Unsaved entry changes detected.",
			"",
			"Save before quitting?",
			"",
			"s : save and quit",
			"d : discard changes and quit",
			"Esc : cancel",
		}, "\n"))
	}

	if !m.confirmQuit && m.showHelp {
		body = m.styles.Help.Render(strings.Join([]string{
			"Global keys",
			"",
			"Tab / gt / gT / 1 / 2 / 3 : switch tabs",
			"? / H : toggle help",
			"c / C : random/reset theme",
			"x : toggle disco mode",
			"q / ZQ : quit",
			"",
			"Entries",
			"",
			"j/k rows, h/l columns, Enter edit selected cell",
			"/ search, f category filter, e/v quick edit, s save, dd delete entry",
			"D day-off datepicker (8h off entry)",
		}, "\n"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, status)
	rendered := m.styles.App.Render(content)

	if m.width > 0 && m.height > 0 {
		rendered = lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, rendered)
	}

	view := tea.NewView(rendered)
	view.AltScreen = true
	return view
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
	parts = append(parts, lipgloss.NewStyle().Bold(true).Render("timesamurai "+timesamurai.Version))
	for idx, label := range tabLabels {
		if tab(idx) == m.activeTab {
			parts = append(parts, m.styles.ActiveTab.Render(label))
			continue
		}
		parts = append(parts, m.styles.Tab.Render(label))
	}
	if m.disco {
		parts = append(parts, m.styles.ActiveTab.Render("DISCO"))
	}
	if m.entries.hasUnsavedChanges() {
		parts = append(parts, m.styles.ActiveTab.Render("UNSAVED"))
	}
	line := strings.Join(parts, " ")
	return m.styles.Header.Width(m.statusWidth()).Render(line)
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
		return m.timer.View().Content
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
	return m.startRootTimerTicker()
}

func (m *Model) bodySize() (width int, height int) {
	width = m.width - 4
	height = m.height - 6

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
		beforeMutations := m.entries.mutationCount
		updated, cmd := m.entries.Update(msg)
		m.entries = updated
		if m.disco && m.entries.mutationCount != beforeMutations {
			m.randomizeTheme()
		}
		return m, cmd
	case tabReport:
		updated, cmd := m.report.Update(msg)
		m.report = updated
		return m, cmd
	case tabTimer:
		updatedModel, _ := m.timer.Update(msg)
		if updated, ok := updatedModel.(TimerModel); ok {
			m.timer = updated
		}
		return m, m.startRootTimerTicker()
	default:
		return m, nil
	}
}

func (m *Model) startRootTimerTicker() tea.Cmd {
	if m.activeTab != tabTimer || !m.timer.state.Running {
		return nil
	}
	if m.timerTickScheduled {
		return nil
	}

	m.timerTickScheduled = true
	return rootTimerTick()
}

func (m *Model) randomizeTheme() {
	m.theme = RandomTheme()
	m.styles = StylesFromTheme(m.theme)
}

func (m *Model) resetTheme() {
	m.theme = DefaultTheme()
	m.styles = StylesFromTheme(m.theme)
}

func (m *Model) renderStatusLine() string {
	status := fmt.Sprintf(
		"Entries timeline table | unsaved:%t | disco:%t | H help | c/C theme | x disco | q quit",
		m.entries.hasUnsavedChanges(),
		m.disco,
	)
	if m.showHelp {
		status = "Help mode active (press H or ? to close)"
	}
	if m.confirmQuit {
		status = "Unsaved changes: s save+quit, d discard+quit, Esc cancel"
	}

	return m.styles.Status.Width(m.statusWidth()).Render(status)
}

func (m *Model) statusWidth() int {
	if m.width <= 0 {
		return 80
	}
	if m.width <= 2 {
		return m.width
	}
	return m.width - 2
}

func (m *Model) requestQuit() (tea.Model, tea.Cmd) {
	if m.entries.hasUnsavedChanges() {
		m.confirmQuit = true
		return m, nil
	}
	return m, tea.Quit
}

func reportOpenSessionWarning(entries []worktime.Entry) string {
	openSessions := worktime.OpenSessions(entries)
	if len(openSessions) == 0 {
		return ""
	}

	items := make([]string, 0, len(openSessions))
	for _, session := range openSessions {
		items = append(items, fmt.Sprintf(
			"%s (since %s)",
			session.Category,
			time.Unix(session.Login.Epoch, 0).Format("2006-01-02 15:04"),
		))
	}
	return "currently logged in: " + strings.Join(items, ", ")
}
