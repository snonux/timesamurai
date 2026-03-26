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
	m.deleteRange(m.cursor-1, m.cursor)
}

func (m *Model) deleteAtCursor() {
	m.deleteRange(m.cursor, m.cursor+1)
}

func (m *Model) deleteLine() {
	m.deleteRange(0, len(m.runes))
}

func (m *Model) deleteToLineEnd() {
	m.deleteRange(m.cursor, len(m.runes))
}

func (m *Model) deleteFromLineStart() {
	m.deleteRange(0, m.cursor)
}

func (m *Model) deleteWordForward() {
	m.deleteRange(m.cursor, wordForward(m.runes, m.cursor))
}

func (m *Model) deleteWordEnd() {
	end := wordEnd(m.runes, m.cursor)
	if end < len(m.runes) {
		end++
	}
	m.deleteRange(m.cursor, end)
}

func (m *Model) deleteWordBackward() {
	m.deleteRange(wordBackward(m.runes, m.cursor), m.cursor)
}

func (m *Model) changeToLineEnd() {
	m.deleteToLineEnd()
	m.mode = ModeInsert
	m.pending = 0
}

func (m *Model) deleteRange(start, end int) {
	if len(m.runes) == 0 {
		return
	}

	start = clampInt(start, 0, len(m.runes))
	end = clampInt(end, 0, len(m.runes))
	if start > end {
		start, end = end, start
	}
	if start == end {
		return
	}

	m.snapshot()
	next := make([]rune, 0, len(m.runes)-(end-start))
	next = append(next, m.runes[:start]...)
	next = append(next, m.runes[end:]...)
	m.runes = next
	m.cursor = start
}
