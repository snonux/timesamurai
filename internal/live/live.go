package live

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/common-nighthawk/go-figure"
	"codeberg.org/snonux/timr/internal/ascii"
	timrTimer "codeberg.org/snonux/timr/internal/timer"
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Model is the Bubble Tea model for the live timer view.
type Model struct {
	state       timrTimer.State
	quitting    bool
	helpStyle   lipgloss.Style
	timerStyle  lipgloss.Style
	statusStyle lipgloss.Style
	width       int
	height      int
	font        string // Font selection: "doom" for figlet, or ASCII font names
	fontIndex   int    // Current index in the AllFonts slice for cycling
}

// NewModel creates a new Model with the specified font.
// The font parameter can be "doom" for the original figlet font,
// or one of the ASCII font names: "mono12", "rebel", "ansi", "ansiShadow".
func NewModel(font string) Model {
	state, err := timrTimer.LoadState()
	if err != nil {
		panic(err) // Or handle more gracefully
	}

	// Find the current font index in AllFonts for cycling support
	fontIndex := 0
	for i, f := range ascii.AllFonts {
		if f == font {
			fontIndex = i
			break
		}
	}

	return Model{
		state:       state,
		helpStyle:   lipgloss.NewStyle().Faint(true),
		timerStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00BFFF")),
		statusStyle: lipgloss.NewStyle().Italic(true),
		font:        font,
		fontIndex:   fontIndex,
	}
}

// Init is the first function that will be called.
func (m Model) Init() tea.Cmd {
	if m.state.Running {
		return tick()
	}
	return nil
}

// Update is called when a message is received.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		if !m.state.Running {
			return m, nil
		}
		return m, tick()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			if err := m.state.Save(); err != nil {
				// handle error
			}
			return m, tea.Quit

		case "s":
			if m.state.Running {
				// Stop the timer
				m.state.ElapsedTime += time.Since(m.state.StartTime)
				m.state.Running = false
				return m, nil // Stop ticking
			} else {
				// Start the timer
				m.state.Running = true
				m.state.StartTime = time.Now()
				return m, tick() // Start ticking
			}

		case "r":
			// Reset the timer
			m.state = timrTimer.State{}
			if _, err := timrTimer.ResetTimer(); err != nil {
				// handle error
			}
			return m, nil // Stop ticking

		case "f":
			// Cycle to the next font
			m.fontIndex = (m.fontIndex + 1) % len(ascii.AllFonts)
			m.font = ascii.AllFonts[m.fontIndex]
			return m, nil
		}
	}

	return m, nil
}

// View renders the model's state to the terminal.
// Supports both figlet-style fonts (doom) and large ASCII art fonts (mono12, rebel, ansi, ansiShadow).
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var currentSession time.Duration
	if m.state.Running {
		currentSession = time.Since(m.state.StartTime)
	}

	totalElapsed := (m.state.ElapsedTime + currentSession).Round(time.Second)

	status := "Paused"
	if m.state.Running {
		status = "Running"
	}

	// Render timer based on selected font
	var timerStr string
	if m.font == "doom" {
		// Use original figlet font
		figure := figure.NewFigure(totalElapsed.String(), "doom", true)
		timerStr = m.timerStyle.Render(figure.String())
	} else {
		// Use large ASCII art fonts
		font := ascii.GetFont(m.font)
		// Format time as HH:MM:SS for better ASCII display
		hours := int(totalElapsed.Hours())
		minutes := int(totalElapsed.Minutes()) % 60
		seconds := int(totalElapsed.Seconds()) % 60
		timeStr := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
		asciiTime := ascii.RenderNumber(timeStr, font)
		timerStr = m.timerStyle.Render(asciiTime)
	}

	statusStr := m.statusStyle.Render(status)
	fontInfo := m.helpStyle.Render(fmt.Sprintf("Font: %s", m.font))
	helpStr := m.helpStyle.Render("q: quit, s: start/stop, r: reset, f: change font")

	// Centering
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			timerStr,
			statusStr,
			"", // spacer
			fontInfo,
			helpStr,
		),
	)
}
