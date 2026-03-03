package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"codeberg.org/snonux/timr/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EntriesModel is a chronological worktime entry browser.
type EntriesModel struct {
	allEntries []worktime.Entry
	visible    []worktime.Entry

	cursor int
	offset int
	width  int
	height int

	pendingG bool

	searchMode  bool
	searchQuery string

	filterMode  bool
	filterQuery string

	input string
}

// NewEntriesModel creates an entry browser model.
func NewEntriesModel(entries []worktime.Entry) EntriesModel {
	model := EntriesModel{
		height: 16,
	}
	model.SetEntries(entries)
	return model
}

// SetSize updates viewport size used for scrolling.
func (m *EntriesModel) SetSize(width, height int) {
	m.width = width
	if height > 0 {
		m.height = height
	}
	m.ensureCursorVisible()
}

// SetEntries replaces entry data (most recent first).
func (m *EntriesModel) SetEntries(entries []worktime.Entry) {
	m.allEntries = append([]worktime.Entry(nil), entries...)
	slices.SortFunc(m.allEntries, func(a, b worktime.Entry) int {
		if a.Epoch == b.Epoch {
			return strings.Compare(a.Action, b.Action)
		}
		if a.Epoch > b.Epoch {
			return -1
		}
		return 1
	})
	m.applyFilters()
}

// Update handles keyboard navigation and search/filter interaction.
func (m EntriesModel) Update(msg tea.Msg) (EntriesModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.searchMode || m.filterMode {
		switch keyMsg.String() {
		case "enter":
			if m.searchMode {
				m.searchQuery = strings.TrimSpace(m.input)
				m.searchMode = false
			} else {
				m.filterQuery = strings.TrimSpace(m.input)
				m.filterMode = false
			}
			m.input = ""
			m.applyFilters()
		case "esc":
			m.searchMode = false
			m.filterMode = false
			m.input = ""
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if keyMsg.Type == tea.KeyRunes {
				m.input += string(keyMsg.Runes)
			}
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "/":
		m.searchMode = true
		m.input = ""
		m.pendingG = false
	case "f":
		m.filterMode = true
		m.input = m.filterQuery
		m.pendingG = false
	case "j", "down":
		m.moveCursor(1)
		m.pendingG = false
	case "k", "up":
		m.moveCursor(-1)
		m.pendingG = false
	case "G":
		m.cursor = len(m.visible) - 1
		m.ensureCursorVisible()
		m.pendingG = false
	case "g":
		if m.pendingG {
			m.cursor = 0
			m.ensureCursorVisible()
			m.pendingG = false
			return m, nil
		}
		m.pendingG = true
	case "ctrl+d":
		m.moveCursor(m.halfPage())
		m.pendingG = false
	case "ctrl+u":
		m.moveCursor(-m.halfPage())
		m.pendingG = false
	case "ctrl+f":
		m.moveCursor(m.pageSize())
		m.pendingG = false
	case "ctrl+b":
		m.moveCursor(-m.pageSize())
		m.pendingG = false
	default:
		m.pendingG = false
	}

	return m, nil
}

// View renders the entries list.
func (m EntriesModel) View(styles Styles) string {
	title := fmt.Sprintf("Entries  [%d/%d]", minInt(m.cursor+1, len(m.visible)), len(m.visible))
	if m.filterQuery != "" {
		title += "  filter:" + m.filterQuery
	}
	if m.searchQuery != "" {
		title += "  search:" + m.searchQuery
	}

	if m.searchMode {
		return styles.Body.Render(title + "\n\n/" + m.input)
	}
	if m.filterMode {
		return styles.Body.Render(title + "\n\nf " + m.input)
	}

	if len(m.visible) == 0 {
		return styles.Body.Render(title + "\n\nNo entries match current search/filter.")
	}

	maxRows := m.listRows()
	end := minInt(len(m.visible), m.offset+maxRows)
	lines := make([]string, 0, end-m.offset)
	for idx := m.offset; idx < end; idx++ {
		entry := m.visible[idx]
		cursor := " "
		if idx == m.cursor {
			cursor = ">"
		}

		timestamp := time.Unix(entry.Epoch, 0).Format("2006-01-02 15:04")
		category := colorizeCategory(entry.What)
		value := formatEntryValue(entry)
		line := fmt.Sprintf("%s %s %-7s %-18s %-8s %s", cursor, timestamp, entry.Action, category, value, entry.Descr)
		lines = append(lines, line)
	}

	return styles.Body.Render(title + "\n\n" + strings.Join(lines, "\n"))
}

func (m *EntriesModel) applyFilters() {
	search := strings.ToLower(m.searchQuery)
	filter := strings.ToLower(m.filterQuery)

	m.visible = m.visible[:0]
	for _, entry := range m.allEntries {
		if filter != "" && !strings.Contains(strings.ToLower(entry.What), filter) {
			continue
		}
		if search != "" {
			joined := strings.ToLower(strings.Join([]string{
				entry.Action,
				entry.What,
				entry.Source,
				entry.Human,
				entry.Descr,
			}, " "))
			if !strings.Contains(joined, search) {
				continue
			}
		}
		m.visible = append(m.visible, entry)
	}

	if len(m.visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.ensureCursorVisible()
}

func (m *EntriesModel) moveCursor(delta int) {
	if len(m.visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}

	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.ensureCursorVisible()
}

func (m *EntriesModel) ensureCursorVisible() {
	if len(m.visible) == 0 {
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

func (m EntriesModel) listRows() int {
	rows := m.height - 4
	if rows < 1 {
		return 1
	}
	return rows
}

func (m EntriesModel) halfPage() int {
	page := m.listRows() / 2
	if page < 1 {
		return 1
	}
	return page
}

func (m EntriesModel) pageSize() int {
	page := m.listRows()
	if page < 1 {
		return 1
	}
	return page
}

func formatEntryValue(entry worktime.Entry) string {
	if entry.Action != "add" {
		return "-"
	}

	hours := float64(entry.Value) / 3600
	return fmt.Sprintf("%+.2fh", hours)
}

func colorizeCategory(category string) string {
	if strings.TrimSpace(category) == "" {
		category = "work"
	}

	colors := map[string]string{
		"work":            "#8BD3DD",
		"lunch":           "#F6BD60",
		"off":             "#CDB4DB",
		"bank":            "#A8DADC",
		"sick":            "#FFB4A2",
		"selfdevelopment": "#B8E1A9",
	}

	color, ok := colors[category]
	if !ok {
		color = "#D9D9D9"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(category)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
