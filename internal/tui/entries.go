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
	pendingD bool

	searchMode  bool
	searchQuery string

	filterMode  bool
	filterQuery string

	editMode      bool
	confirmDelete bool

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

// Update handles keyboard navigation and search/filter/edit interaction.
func (m EntriesModel) Update(msg tea.Msg) (EntriesModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.confirmDelete {
		switch keyMsg.String() {
		case "y":
			m.deleteSelected()
			m.confirmDelete = false
		case "n", "esc":
			m.confirmDelete = false
		}
		return m, nil
	}

	if m.editMode {
		switch keyMsg.String() {
		case "enter":
			m.saveEdit()
			m.editMode = false
			m.input = ""
		case "esc":
			m.editMode = false
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
		m.pendingD = false
	case "f":
		m.filterMode = true
		m.input = m.filterQuery
		m.pendingG = false
		m.pendingD = false
	case "e", "enter":
		m.beginEdit()
		m.pendingG = false
		m.pendingD = false
	case "o":
		m.insertEntry(false)
		m.pendingG = false
		m.pendingD = false
	case "O":
		m.insertEntry(true)
		m.pendingG = false
		m.pendingD = false
	case "d":
		if m.pendingD {
			m.confirmDelete = true
			m.pendingD = false
			return m, nil
		}
		m.pendingD = true
		m.pendingG = false
	case "j", "down":
		m.moveCursor(1)
		m.pendingG = false
		m.pendingD = false
	case "k", "up":
		m.moveCursor(-1)
		m.pendingG = false
		m.pendingD = false
	case "G":
		m.cursor = len(m.visible) - 1
		m.ensureCursorVisible()
		m.pendingG = false
		m.pendingD = false
	case "g":
		if m.pendingG {
			m.cursor = 0
			m.ensureCursorVisible()
			m.pendingG = false
			m.pendingD = false
			return m, nil
		}
		m.pendingG = true
		m.pendingD = false
	case "ctrl+d":
		m.moveCursor(m.halfPage())
		m.pendingG = false
		m.pendingD = false
	case "ctrl+u":
		m.moveCursor(-m.halfPage())
		m.pendingG = false
		m.pendingD = false
	case "ctrl+f":
		m.moveCursor(m.pageSize())
		m.pendingG = false
		m.pendingD = false
	case "ctrl+b":
		m.moveCursor(-m.pageSize())
		m.pendingG = false
		m.pendingD = false
	default:
		m.pendingG = false
		m.pendingD = false
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
	if m.editMode {
		return styles.Body.Render(title + "\n\nEdit description: " + m.input)
	}
	if m.confirmDelete {
		return styles.Body.Render(title + "\n\nDelete selected entry? (y/n)")
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

func (m *EntriesModel) beginEdit() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	m.editMode = true
	m.input = m.visible[m.cursor].Descr
}

func (m *EntriesModel) saveEdit() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	oldEntry := m.visible[m.cursor]
	newEntry := oldEntry
	newEntry.Descr = strings.TrimSpace(m.input)
	m.replaceEntry(oldEntry, newEntry)
}

func (m *EntriesModel) deleteSelected() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	target := m.visible[m.cursor]
	idx := findEntryIndex(m.allEntries, target)
	if idx < 0 {
		return
	}

	m.allEntries = append(m.allEntries[:idx], m.allEntries[idx+1:]...)
	m.applyFilters()
}

func (m *EntriesModel) insertEntry(above bool) {
	newEntry := worktime.Entry{
		Action: "add",
		What:   "work",
		Epoch:  time.Now().Unix(),
		Source: "local",
		Human:  time.Now().Format("Mon 02.01.2006 15:04:05"),
		Value:  int64(time.Hour / time.Second),
	}

	insertAt := len(m.allEntries)
	if len(m.visible) > 0 && m.cursor < len(m.visible) {
		target := m.visible[m.cursor]
		if idx := findEntryIndex(m.allEntries, target); idx >= 0 {
			insertAt = idx
			if !above {
				insertAt++
			}
		}
	}

	m.allEntries = insertEntryAt(m.allEntries, insertAt, newEntry)
	m.applyFilters()

	if idx := findEntryIndex(m.visible, newEntry); idx >= 0 {
		m.cursor = idx
		m.ensureCursorVisible()
	}

	m.editMode = true
	m.input = ""
}

func (m *EntriesModel) replaceEntry(oldEntry, newEntry worktime.Entry) {
	idx := findEntryIndex(m.allEntries, oldEntry)
	if idx < 0 {
		return
	}

	m.allEntries[idx] = newEntry
	m.applyFilters()

	if newIdx := findEntryIndex(m.visible, newEntry); newIdx >= 0 {
		m.cursor = newIdx
		m.ensureCursorVisible()
	}
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

func findEntryIndex(entries []worktime.Entry, target worktime.Entry) int {
	for idx, entry := range entries {
		if entry == target {
			return idx
		}
	}
	return -1
}

func insertEntryAt(entries []worktime.Entry, idx int, entry worktime.Entry) []worktime.Entry {
	if idx < 0 {
		idx = 0
	}
	if idx > len(entries) {
		idx = len(entries)
	}

	entries = append(entries, worktime.Entry{})
	copy(entries[idx+1:], entries[idx:])
	entries[idx] = entry
	return entries
}
