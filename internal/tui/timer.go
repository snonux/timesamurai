package tui

import (
	"fmt"
	"strings"
	"time"

	"codeberg.org/snonux/timr/internal/ascii"
	"codeberg.org/snonux/timr/internal/config"
	timrTimer "codeberg.org/snonux/timr/internal/timer"
	"codeberg.org/snonux/timr/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
)

type timerTickMsg time.Time

func timerTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}

// TimerModel is the timer screen model used inside the TUI.
type TimerModel struct {
	state       timrTimer.State
	quitting    bool
	helpStyle   lipgloss.Style
	timerStyle  lipgloss.Style
	statusStyle lipgloss.Style
	width       int
	height      int
	font        string
	fontIndex   int

	work workIntegration
}

type workIntegration struct {
	enabled  bool
	dbDir    string
	host     string
	loggedIn bool
	status   string
}

// NewTimerModel builds the timer screen model.
func NewTimerModel(font string, cfg config.Config) (TimerModel, error) {
	state, err := timrTimer.LoadState()
	if err != nil {
		return TimerModel{}, err
	}

	selectedFont := strings.TrimSpace(font)
	if selectedFont == "" {
		selectedFont = "doom"
	}

	fontIndex := 0
	for idx, available := range ascii.AllFonts {
		if available == selectedFont {
			fontIndex = idx
			break
		}
	}

	model := TimerModel{
		state:       state,
		helpStyle:   lipgloss.NewStyle().Faint(true),
		timerStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF")),
		statusStyle: lipgloss.NewStyle().Italic(true),
		font:        selectedFont,
		fontIndex:   fontIndex,
		work: workIntegration{
			status: "work integration disabled",
		},
	}

	if strings.TrimSpace(cfg.WorktimeDBDir) != "" {
		host, err := cfg.EffectiveHostname()
		if err == nil {
			model.work = workIntegration{
				enabled: true,
				dbDir:   cfg.WorktimeDBDir,
				host:    host,
				status:  "work integration enabled",
			}
			model.work.refreshStatus()
		}
	}

	return model, nil
}

// SetSize updates timer viewport dimensions.
func (m *TimerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Init is called when the model starts.
func (m TimerModel) Init() tea.Cmd {
	if m.state.Running {
		return timerTick()
	}
	return nil
}

// Update handles timer screen updates.
func (m TimerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case timerTickMsg:
		if !m.state.Running {
			return m, nil
		}
		return m, timerTick()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			_ = m.state.Save()
			return m, tea.Quit

		case "s", " ":
			if m.state.Running {
				m.state.ElapsedTime += time.Since(m.state.StartTime)
				m.state.Running = false
			} else {
				m.state.Running = true
				m.state.StartTime = time.Now()
				if err := m.state.Save(); err != nil {
					m.work.status = "save error: " + err.Error()
				}
				return m, timerTick()
			}
			if err := m.state.Save(); err != nil {
				m.work.status = "save error: " + err.Error()
			}
			return m, nil

		case "r":
			m.state = timrTimer.State{}
			if _, err := timrTimer.ResetTimer(); err != nil {
				m.work.status = "reset error: " + err.Error()
			}
			return m, nil

		case "f":
			m.fontIndex = (m.fontIndex + 1) % len(ascii.AllFonts)
			m.font = ascii.AllFonts[m.fontIndex]
			return m, nil

		case "l":
			m.work.toggle()
			return m, nil
		}
	}

	return m, nil
}

// View renders the timer screen.
func (m TimerModel) View() string {
	if m.quitting {
		return ""
	}

	elapsed := m.state.ElapsedTime
	if m.state.Running {
		elapsed += time.Since(m.state.StartTime)
	}
	elapsed = elapsed.Round(time.Second)

	timerOutput := m.renderTimer(elapsed)

	status := "Paused"
	if m.state.Running {
		status = "Running"
	}

	lines := []string{
		timerOutput,
		m.statusStyle.Render(status),
		"",
		m.helpStyle.Render(fmt.Sprintf("Font: %s", m.font)),
		m.helpStyle.Render(fmt.Sprintf("Work: %s", m.work.status)),
		m.helpStyle.Render("s/Space: start-stop, r: reset, f: change font, l: work login/logout, q: quit"),
	}

	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, lines...),
	)
}

func (m TimerModel) renderTimer(elapsed time.Duration) string {
	if m.font == "doom" {
		figureOutput := figure.NewFigure(elapsed.String(), "doom", true)
		return m.timerStyle.Render(figureOutput.String())
	}

	font := ascii.GetFont(m.font)
	hours := int(elapsed.Hours())
	minutes := int(elapsed.Minutes()) % 60
	seconds := int(elapsed.Seconds()) % 60
	timeString := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	asciiTime := ascii.RenderNumber(timeString, font)
	return m.timerStyle.Render(asciiTime)
}

func (w *workIntegration) toggle() {
	if !w.enabled {
		w.status = "work integration disabled"
		return
	}

	now := time.Now()
	var err error
	if w.loggedIn {
		_, err = worktime.Logout(w.dbDir, w.host, "work", now, "tui timer toggle")
	} else {
		_, err = worktime.Login(w.dbDir, w.host, "work", now, "tui timer toggle")
	}
	if err != nil {
		w.status = "work toggle error: " + err.Error()
		return
	}

	w.refreshStatus()
}

func (w *workIntegration) refreshStatus() {
	if !w.enabled {
		w.status = "work integration disabled"
		return
	}

	entries, err := worktime.LoadAll(w.dbDir)
	if err != nil {
		w.status = "work status error: " + err.Error()
		return
	}

	w.loggedIn = workCategoryLoggedIn(entries)
	if w.loggedIn {
		w.status = "logged in"
	} else {
		w.status = "logged out"
	}
}

func workCategoryLoggedIn(entries []worktime.Entry) bool {
	loggedIn := false
	for _, entry := range entries {
		category := strings.TrimSpace(entry.What)
		if category == "" {
			category = "work"
		}
		if category != "work" {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(entry.Action)) {
		case "login":
			loggedIn = true
		case "logout":
			loggedIn = false
		}
	}
	return loggedIn
}
