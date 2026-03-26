package viinput

func (m *Model) snapshot() {
	history := make([]rune, len(m.runes))
	copy(history, m.runes)
	m.history = append(m.history, history)
}

func (m *Model) undo() {
	if len(m.history) == 0 {
		return
	}

	last := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.runes = append([]rune(nil), last...)
	m.cursor = clampInt(m.cursor, 0, len(m.runes))
}

func (m *Model) insertText(text string) {
	if text == "" {
		return
	}

	m.snapshot()
	runes := []rune(text)
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.runes) {
		m.cursor = len(m.runes)
	}

	next := make([]rune, 0, len(m.runes)+len(runes))
	next = append(next, m.runes[:m.cursor]...)
	next = append(next, runes...)
	next = append(next, m.runes[m.cursor:]...)
	m.runes = next
	m.cursor += len(runes)
}

func (m *Model) deleteBeforeCursor() {
	if m.cursor <= 0 || len(m.runes) == 0 {
		return
	}

	m.snapshot()
	m.runes = append(append([]rune(nil), m.runes[:m.cursor-1]...), m.runes[m.cursor:]...)
	m.cursor--
}

func (m *Model) deleteAtCursor() {
	if len(m.runes) == 0 || m.cursor >= len(m.runes) {
		return
	}

	m.snapshot()
	m.runes = append(append([]rune(nil), m.runes[:m.cursor]...), m.runes[m.cursor+1:]...)
}
