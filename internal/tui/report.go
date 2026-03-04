package tui

import (
	"fmt"
	"strings"

	"codeberg.org/snonux/timesamurai/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
)

// ReportModel is a weekly report browser screen.
type ReportModel struct {
	weeks []worktime.WeekReport
	warn  string

	weekIndex int
	cursor    int
	offset    int
	width     int
	height    int

	verbose bool

	pendingG       bool
	pendingBracket int
}

// NewReportModel creates a report screen model.
func NewReportModel(weeks []worktime.WeekReport) ReportModel {
	model := ReportModel{
		height: 16,
	}
	model.SetWeeks(weeks)
	return model
}

// SetSize updates viewport dimensions.
func (m *ReportModel) SetSize(width, height int) {
	m.width = width
	if height > 0 {
		m.height = height
	}
	m.ensureCursorVisible()
}

// SetWeeks replaces report data.
func (m *ReportModel) SetWeeks(weeks []worktime.WeekReport) {
	m.weeks = append([]worktime.WeekReport(nil), weeks...)
	if len(m.weeks) == 0 {
		m.weekIndex = 0
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.weekIndex >= len(m.weeks) {
		m.weekIndex = len(m.weeks) - 1
	}
	m.cursor = 0
	m.offset = 0
}

// SetWarning sets a status warning shown at the bottom of the report view.
func (m *ReportModel) SetWarning(warning string) {
	m.warn = strings.TrimSpace(warning)
}

// Update handles keyboard interaction.
func (m *ReportModel) Update(msg tea.Msg) (ReportModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *m, nil
	}

	if m.pendingBracket != 0 {
		if keyMsg.String() == "w" {
			if m.pendingBracket > 0 {
				m.nextWeek()
			} else {
				m.prevWeek()
			}
			m.pendingBracket = 0
			return *m, nil
		}
		m.pendingBracket = 0
	}

	switch keyMsg.String() {
	case "j", "down":
		m.moveCursor(1)
		m.pendingG = false
	case "k", "up":
		m.moveCursor(-1)
		m.pendingG = false
	case "G":
		m.cursor = m.rowCount() - 1
		m.ensureCursorVisible()
		m.pendingG = false
	case "g":
		if m.pendingG {
			m.cursor = 0
			m.ensureCursorVisible()
			m.pendingG = false
			return *m, nil
		}
		m.pendingG = true
	case "]":
		m.pendingBracket = 1
		m.pendingG = false
	case "[":
		m.pendingBracket = -1
		m.pendingG = false
	case "v":
		m.verbose = !m.verbose
		m.pendingG = false
	default:
		m.pendingG = false
	}

	return *m, nil
}

// View renders the report screen.
func (m *ReportModel) View(styles Styles) string {
	if len(m.weeks) == 0 {
		return styles.Body.Render("Report\n\nNo report data.")
	}

	week := &m.weeks[m.weekIndex]
	header := fmt.Sprintf(
		"Report  Week %s  [%d/%d]  verbose:%t",
		week.WeekLabel,
		m.weekIndex+1,
		len(m.weeks),
		m.verbose,
	)

	rows := m.dayRows(week)
	maxRows := m.listRows()
	end := minInt(len(rows), m.offset+maxRows)

	lines := make([]string, 0, end-m.offset)
	for idx := m.offset; idx < end; idx++ {
		cursor := " "
		if idx == m.cursor {
			cursor = ">"
		}
		lines = append(lines, cursor+" "+rows[idx])
	}

	summary := fmt.Sprintf(
		"Balance:%+.2fh  Work:%+.2fh  Buffer:%+.2fh",
		toHours(week.CumulativeBalanceSeconds),
		toHours(week.Values["work"]),
		toHours(week.BufferSeconds),
	)
	hint := "j/k scroll, gg/G top/bottom, ]w/[w week nav, v verbose"

	body := header + "\n\n" + strings.Join(lines, "\n") + "\n\n" + summary + "\n" + hint
	if m.warn != "" {
		body += "\n" + styles.Warning.Render("Warning: "+m.warn)
	}
	return styles.Body.Render(body)
}

func (m *ReportModel) dayRows(week *worktime.WeekReport) []string {
	rows := make([]string, 0, len(week.Days))
	for _, day := range week.Days {
		row := fmt.Sprintf(
			"%-1s %-18s work:%+6.2fh lunch:%+6.2fh off:%+6.2fh bank:%+6.2fh",
			day.Marker,
			day.DayLabel,
			toHours(day.Values["work"]),
			toHours(day.Values["lunch"]),
			toHours(day.Values["off"]),
			toHours(day.Values["bank"]),
		)
		if m.verbose {
			row += fmt.Sprintf(" epoch:%d", day.Epoch)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		rows = append(rows, "No days in this week.")
	}
	return rows
}

func (m *ReportModel) moveCursor(delta int) {
	if m.rowCount() == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}

	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.rowCount() {
		m.cursor = m.rowCount() - 1
	}
	m.ensureCursorVisible()
}

func (m *ReportModel) ensureCursorVisible() {
	if m.rowCount() == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
	}

	maxRows := m.listRows()
	if maxRows <= 0 {
		return
	}
	if m.cursor >= m.offset+maxRows {
		m.offset = m.cursor - maxRows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *ReportModel) nextWeek() {
	if len(m.weeks) == 0 {
		return
	}
	if m.weekIndex < len(m.weeks)-1 {
		m.weekIndex++
	}
	m.cursor = 0
	m.offset = 0
}

func (m *ReportModel) prevWeek() {
	if len(m.weeks) == 0 {
		return
	}
	if m.weekIndex > 0 {
		m.weekIndex--
	}
	m.cursor = 0
	m.offset = 0
}

func (m *ReportModel) listRows() int {
	rows := m.height - 7
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *ReportModel) rowCount() int {
	if len(m.weeks) == 0 {
		return 0
	}
	return len(m.dayRows(&m.weeks[m.weekIndex]))
}

func toHours(seconds int64) float64 {
	return float64(seconds) / 3600
}
