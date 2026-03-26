package viinput

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Mode represents the current vi-style input state.
type Mode int

const (
	// ModeInsert accepts normal text entry and cursor editing keys.
	ModeInsert Mode = iota
	// ModeNormal accepts vi-style commands.
	ModeNormal
)

// Model is the state container for the vi-style input component.
type Model struct {
	Prompt    string
	runes     []rune
	cursor    int
	mode      Mode
	focused   bool
	pending   rune
	history   [][]rune
	wantsExit bool
}

// New returns a Model initialized for insert mode.
func New() Model {
	return Model{mode: ModeInsert}
}

// Focus marks the model as active and returns a no-op command.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	m.mode = ModeInsert
	m.pending = 0
	m.wantsExit = false
	return nil
}

// Blur marks the model as inactive.
func (m *Model) Blur() {
	m.focused = false
	m.pending = 0
	m.wantsExit = false
}

// SetValue replaces the current buffer contents.
func (m *Model) SetValue(value string) {
	m.runes = []rune(value)
	m.cursor = len(m.runes)
	m.pending = 0
	m.wantsExit = false
}

// Value returns the current buffer contents.
func (m Model) Value() string {
	return string(m.runes)
}

// Mode returns the current editing mode.
func (m Model) Mode() Mode {
	return m.mode
}

// WantsExit reports whether normal mode requested that editing end.
func (m Model) WantsExit() bool {
	return m.wantsExit
}

// Update applies a key event to the model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok || !m.focused {
		return m, nil
	}

	if m.mode == ModeNormal && m.pending != 0 {
		if m.pending == 'g' {
			switch keyMsg.String() {
			case "g", "h":
				m.cursor = 0
				m.pending = 0
				return m, nil
			case "l":
				m.cursor = len(m.runes)
				m.pending = 0
				return m, nil
			}
		}
		m.pending = 0
	}

	switch m.mode {
	case ModeInsert:
		return m.updateInsertMode(keyMsg)
	case ModeNormal:
		return m.updateNormalMode(keyMsg)
	default:
		return m, nil
	}
}

// View renders the prompt, value and cursor.
func (m Model) View() string {
	var builder strings.Builder
	builder.WriteString(m.Prompt)

	cursorRune := "█"
	if m.mode == ModeInsert {
		cursorRune = "▏"
	}

	cursor := clampInt(m.cursor, 0, len(m.runes))
	builder.WriteString(string(m.runes[:cursor]))
	builder.WriteString(cursorRune)
	builder.WriteString(string(m.runes[cursor:]))
	return builder.String()
}

func (m Model) updateInsertMode(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc":
		m.mode = ModeNormal
		m.pending = 0
	case "left":
		m.cursor = clampInt(m.cursor-1, 0, len(m.runes))
	case "right":
		m.cursor = clampInt(m.cursor+1, 0, len(m.runes))
	case "home", "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = len(m.runes)
	case "backspace", "ctrl+h":
		m.deleteBeforeCursor()
	case "delete", "ctrl+d":
		m.deleteAtCursor()
	default:
		if text, ok := insertedText(keyMsg); ok {
			m.insertText(text)
		}
	}

	return m, nil
}

func (m Model) updateNormalMode(keyMsg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch keyMsg.String() {
	case "esc":
		m.wantsExit = true
		return m, nil
	case "h", "left":
		m.cursor = clampInt(m.cursor-1, 0, len(m.runes))
	case "l", "right":
		m.cursor = clampInt(m.cursor+1, 0, len(m.runes))
	case "w":
		m.cursor = wordForward(m.runes, m.cursor)
	case "b":
		m.cursor = wordBackward(m.runes, m.cursor)
	case "e":
		m.cursor = wordEnd(m.runes, m.cursor)
	case "0":
		m.cursor = 0
	case "$":
		m.cursor = len(m.runes)
	case "g":
		m.pending = 'g'
	case "i":
		m.mode = ModeInsert
	case "a":
		m.cursor = clampInt(m.cursor+1, 0, len(m.runes))
		m.mode = ModeInsert
	case "I":
		m.cursor = 0
		m.mode = ModeInsert
	case "A":
		m.cursor = len(m.runes)
		m.mode = ModeInsert
	default:
		m.pending = 0
	}

	return m, nil
}

func insertedText(keyMsg tea.KeyPressMsg) (string, bool) {
	text := keyMsg.Text
	if text != "" {
		return text, true
	}

	value := keyMsg.String()
	if len([]rune(value)) == 1 {
		return value, true
	}

	return "", false
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
