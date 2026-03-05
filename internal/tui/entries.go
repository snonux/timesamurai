package tui

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"codeberg.org/snonux/timesamurai/internal/duration"
	"codeberg.org/snonux/timesamurai/internal/timefmt"
	"codeberg.org/snonux/timesamurai/internal/worktime"
)

type entryEditField int

const (
	entryEditFieldDescription entryEditField = iota
	entryEditFieldValue
	entryEditFieldDate
	entryEditFieldTime
	entryEditFieldAction
	entryEditFieldCategory
)

type entriesColumn int

const (
	entriesColumnDate entriesColumn = iota
	entriesColumnTime
	entriesColumnAction
	entriesColumnCategory
	entriesColumnValue
	entriesColumnSource
	entriesColumnDescription
	entriesColumnCount
)

// EntriesModel is a chronological worktime entry browser.
type EntriesModel struct {
	allEntries []worktime.Entry
	visible    []worktime.Entry

	cursor         int
	offset         int
	selectedColumn entriesColumn
	width          int
	height         int

	pendingG bool
	pendingD bool

	searchMode  bool
	searchQuery string

	filterMode  bool
	filterQuery string
	dayOffMode  bool
	dayOffDate  time.Time

	editMode      bool
	confirmDelete bool

	input     string
	editField entryEditField

	dbDir         string
	dbHost        string
	statusMessage string
	statusError   bool
	mutationCount int
	dirty         bool
	changedHosts  map[string]struct{}
}

// NewEntriesModel creates an entry browser model.
func NewEntriesModel(entries []worktime.Entry) EntriesModel {
	model := EntriesModel{
		height:         16,
		selectedColumn: entriesColumnDescription,
	}
	model.SetEntries(entries)
	return model
}

// SetPersistence enables edit/delete/create persistence against dbDir and host.
func (m *EntriesModel) SetPersistence(dbDir, host string) {
	m.dbDir = strings.TrimSpace(dbDir)
	m.dbHost = strings.TrimSpace(host)
	if m.changedHosts == nil {
		m.changedHosts = map[string]struct{}{}
	}
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
	m.dirty = false
	m.changedHosts = map[string]struct{}{}
	m.applyFilters()
}

// Update handles keyboard navigation and search/filter/edit interaction.
func (m *EntriesModel) Update(msg tea.Msg) (EntriesModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return *m, nil
	}

	if m.updateConfirmDelete(keyMsg.String()) {
		return *m, nil
	}

	if m.updateDayOffMode(keyMsg) {
		return *m, nil
	}

	if m.updateEditMode(keyMsg) {
		return *m, nil
	}

	if m.updateSearchFilterMode(keyMsg) {
		return *m, nil
	}

	m.updateNormalMode(keyMsg)
	return *m, nil
}

func (m EntriesModel) capturesTextInput() bool {
	return m.editMode || m.searchMode || m.filterMode
}

func (m *EntriesModel) updateConfirmDelete(key string) bool {
	if !m.confirmDelete {
		return false
	}

	switch key {
	case "y":
		if err := m.deleteSelected(); err != nil {
			m.setStatusError("Delete failed: " + err.Error())
		} else {
			m.setStatusInfo("Entry deleted.")
		}
		m.confirmDelete = false
	case "n", "esc":
		m.confirmDelete = false
	}

	return true
}

func (m *EntriesModel) updateEditMode(keyMsg tea.KeyPressMsg) bool {
	if !m.editMode {
		return false
	}

	switch keyMsg.String() {
	case "enter":
		if err := m.saveEdit(); err != nil {
			m.setStatusError("Edit failed: " + err.Error())
		} else {
			m.setStatusInfo("Entry updated.")
		}
		m.editMode = false
		m.input = ""
	case "esc":
		m.editMode = false
		m.input = ""
	case "backspace":
		m.input = trimLastRune(m.input)
	default:
		m.input = appendInputKey(m.input, keyMsg)
	}

	return true
}

func (m *EntriesModel) updateDayOffMode(keyMsg tea.KeyPressMsg) bool {
	if !m.dayOffMode {
		return false
	}

	switch keyMsg.String() {
	case "enter":
		if addErr := m.addDayOff(m.dayOffDate); addErr != nil {
			m.setStatusError("Add day off failed: " + addErr.Error())
		} else {
			m.setStatusInfo("Day off added.")
		}

		m.dayOffMode = false
	case "esc":
		m.dayOffMode = false
	case "left", "h":
		m.dayOffDate = m.dayOffDate.AddDate(0, 0, -1)
	case "right", "l":
		m.dayOffDate = m.dayOffDate.AddDate(0, 0, 1)
	case "up", "k":
		m.dayOffDate = m.dayOffDate.AddDate(0, 0, -7)
	case "down", "j":
		m.dayOffDate = m.dayOffDate.AddDate(0, 0, 7)
	case "pgup":
		m.dayOffDate = m.dayOffDate.AddDate(0, -1, 0)
	case "pgdown":
		m.dayOffDate = m.dayOffDate.AddDate(0, 1, 0)
	case "home":
		m.dayOffDate = time.Date(m.dayOffDate.Year(), m.dayOffDate.Month(), 1, 0, 0, 0, 0, m.dayOffDate.Location())
	case "end":
		m.dayOffDate = time.Date(m.dayOffDate.Year(), m.dayOffDate.Month()+1, 0, 0, 0, 0, 0, m.dayOffDate.Location())
	default:
		return true
	}

	return true
}

func (m *EntriesModel) updateSearchFilterMode(keyMsg tea.KeyPressMsg) bool {
	if !m.searchMode && !m.filterMode {
		return false
	}

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
		m.input = trimLastRune(m.input)
	default:
		m.input = appendInputKey(m.input, keyMsg)
	}

	return true
}

func (m *EntriesModel) updateNormalMode(keyMsg tea.KeyPressMsg) {
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
	case "enter":
		m.beginEditSelectedField()
		m.pendingG = false
		m.pendingD = false
	case "e":
		m.beginEditDescription()
		m.pendingG = false
		m.pendingD = false
	case "v":
		m.beginEditValue()
		m.pendingG = false
		m.pendingD = false
	case "h", "left":
		m.moveColumn(-1)
		m.pendingG = false
		m.pendingD = false
	case "l", "right":
		m.moveColumn(1)
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
	case "D":
		m.dayOffMode = true
		m.dayOffDate = time.Now()
		m.pendingG = false
		m.pendingD = false
	case "s":
		if err := m.savePendingChanges(); err != nil {
			m.setStatusError("Save failed: " + err.Error())
		}
		m.pendingG = false
		m.pendingD = false
	case "d":
		if m.pendingD {
			m.confirmDelete = true
			m.pendingD = false
			return
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
			return
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
	case "pgdown":
		m.moveCursor(m.pageSize())
		m.pendingG = false
		m.pendingD = false
	case "pgup":
		m.moveCursor(-m.pageSize())
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
}

// View renders the entries timeline table.
func (m *EntriesModel) View(styles Styles) string {
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
	if m.dayOffMode {
		return styles.Body.Render(m.renderDayOffPicker(title, styles))
	}
	if m.editMode {
		prompt := m.editPrompt()
		return styles.Body.Render(title + "\n\n" + prompt + m.input)
	}
	if m.confirmDelete {
		return styles.Body.Render(title + "\n\nDelete selected entry? (y/n)")
	}

	if len(m.visible) == 0 {
		body := title + "\n\nNo entries match current search/filter."
		return styles.Body.Render(body + m.renderStatus(styles))
	}

	rows := m.renderTimelineTable(styles)
	body := title + "\n\n" + rows
	body += "\n\n" + styles.Hint.Render("j/k rows, PgUp/PgDn page, h/l columns, Enter edit cell, s save, / search, D day-off datepicker, dd delete")
	return styles.Body.Render(body + m.renderStatus(styles))
}

func (m *EntriesModel) applyFilters() {
	search := strings.ToLower(m.searchQuery)
	filter := strings.ToLower(m.filterQuery)

	m.visible = m.visible[:0]
	for _, entry := range m.allEntries {
		if filter != "" && !strings.Contains(strings.ToLower(entry.What), filter) {
			continue
		}
		if search != "" && !entryMatchesSearch(entry, search) {
			continue
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

func (m *EntriesModel) moveColumn(delta int) {
	m.selectedColumn += entriesColumn(delta)
	if m.selectedColumn < entriesColumnDate {
		m.selectedColumn = entriesColumnDate
	}
	if m.selectedColumn >= entriesColumnCount {
		m.selectedColumn = entriesColumnCount - 1
	}
}

func (m *EntriesModel) beginEditSelectedField() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	entry := m.visible[m.cursor]
	entryTime := time.Unix(entry.Epoch, 0)

	switch m.selectedColumn {
	case entriesColumnDate:
		m.editMode = true
		m.editField = entryEditFieldDate
		m.input = entryTime.Format("2006-01-02")
	case entriesColumnTime:
		m.editMode = true
		m.editField = entryEditFieldTime
		m.input = entryTime.Format("15:04")
	case entriesColumnAction:
		m.editMode = true
		m.editField = entryEditFieldAction
		m.input = entry.Action
	case entriesColumnCategory:
		m.editMode = true
		m.editField = entryEditFieldCategory
		m.input = entry.What
	case entriesColumnValue:
		m.beginEditValue()
	case entriesColumnDescription:
		m.beginEditDescription()
	case entriesColumnSource:
		m.setStatusInfo("Source column is read-only.")
	}
}

func (m *EntriesModel) beginEditDescription() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	m.selectedColumn = entriesColumnDescription
	m.editMode = true
	m.editField = entryEditFieldDescription
	m.input = m.visible[m.cursor].Descr
}

func (m *EntriesModel) beginEditValue() {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return
	}

	entry := m.visible[m.cursor]
	if strings.ToLower(strings.TrimSpace(entry.Action)) != "add" {
		m.setStatusError("Only 'add' entries have an editable value.")
		return
	}

	m.selectedColumn = entriesColumnValue
	m.editMode = true
	m.editField = entryEditFieldValue
	m.input = strconv.FormatInt(entry.Value, 10)
}

func (m *EntriesModel) saveEdit() error {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return nil
	}

	oldEntry := m.visible[m.cursor]
	newEntry := oldEntry

	switch m.editField {
	case entryEditFieldDate:
		updatedTime, err := parseEditedDate(m.input, time.Unix(newEntry.Epoch, 0))
		if err != nil {
			return err
		}
		newEntry.Epoch = updatedTime.Unix()
		newEntry.Human = updatedTime.Format("Mon 02.01.2006 15:04:05")
	case entryEditFieldTime:
		updatedTime, err := parseEditedTime(m.input, time.Unix(newEntry.Epoch, 0))
		if err != nil {
			return err
		}
		newEntry.Epoch = updatedTime.Unix()
		newEntry.Human = updatedTime.Format("Mon 02.01.2006 15:04:05")
	case entryEditFieldAction:
		newEntry.Action = strings.TrimSpace(m.input)
	case entryEditFieldCategory:
		newEntry.What = strings.TrimSpace(m.input)
	case entryEditFieldValue:
		if strings.ToLower(strings.TrimSpace(newEntry.Action)) != "add" {
			return errors.New("only 'add' entries have an editable value")
		}
		parsedValue, err := duration.Parse(m.input)
		if err != nil {
			return err
		}
		newEntry.Value = int64(parsedValue / time.Second)
	default:
		newEntry.Descr = strings.TrimSpace(m.input)
	}

	host := strings.TrimSpace(newEntry.Source)
	if host == "" {
		host = strings.TrimSpace(oldEntry.Source)
	}
	if host == "" {
		host = strings.TrimSpace(m.dbHost)
	}
	if host == "" {
		host = "local"
	}

	normalized, err := worktime.NormalizeEditedEntry(newEntry, host)
	if err != nil {
		return err
	}
	newEntry = normalized

	if err := m.persistReplacement(oldEntry, newEntry); err != nil {
		return err
	}
	m.replaceEntry(oldEntry, newEntry)
	return nil
}

func (m *EntriesModel) editPrompt() string {
	switch m.editField {
	case entryEditFieldDate:
		return "Edit date (e.g. 2026-03-04, today, yesterday): "
	case entryEditFieldTime:
		return "Edit time (HH:MM or HH:MM:SS): "
	case entryEditFieldAction:
		return "Edit action (login/logout/add): "
	case entryEditFieldCategory:
		return "Edit category: "
	case entryEditFieldValue:
		return "Edit value (e.g. 90m, 3600, -600): "
	default:
		return "Edit description: "
	}
}

func (m *EntriesModel) deleteSelected() error {
	if len(m.visible) == 0 || m.cursor >= len(m.visible) {
		return nil
	}

	target := m.visible[m.cursor]
	idx := findEntryIndex(m.allEntries, target)
	if idx < 0 {
		return errors.New("selected entry not found in current list")
	}

	if err := m.persistDelete(target); err != nil {
		return err
	}

	m.allEntries = append(m.allEntries[:idx], m.allEntries[idx+1:]...)
	m.applyFilters()
	m.mutationCount++
	return nil
}

func (m *EntriesModel) insertEntry(above bool) {
	sourceHost := "local"
	if configuredHost := strings.TrimSpace(m.dbHost); configuredHost != "" {
		sourceHost = configuredHost
	}

	newEntry := worktime.Entry{
		Action: "add",
		What:   "work",
		Epoch:  time.Now().Unix(),
		Source: sourceHost,
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
	m.mutationCount++
	m.markHostChanged(newEntry.Source)

	if idx := findEntryIndex(m.visible, newEntry); idx >= 0 {
		m.cursor = idx
		m.ensureCursorVisible()
	}

	m.editMode = true
	m.input = ""
}

func (m *EntriesModel) addDayOff(day time.Time) error {
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())

	if m.dbDir != "" {
		host := strings.TrimSpace(m.dbHost)
		if host == "" {
			return errors.New("persistence host is not configured")
		}

		entry := worktime.Entry{
			Action: "add",
			What:   "off",
			Epoch:  dayStart.Unix(),
			Source: host,
			Human:  dayStart.Format("Mon 02.01.2006 15:04:05"),
			Value:  int64((8 * time.Hour) / time.Second),
		}
		m.appendEntry(entry)
		return nil
	}

	entry := worktime.Entry{
		Action: "add",
		What:   "off",
		Epoch:  dayStart.Unix(),
		Source: "local",
		Human:  dayStart.Format("Mon 02.01.2006 15:04:05"),
		Value:  int64((8 * time.Hour) / time.Second),
	}
	m.appendEntry(entry)
	return nil
}

func (m *EntriesModel) renderDayOffPicker(title string, styles Styles) string {
	selected := m.dayOffDate
	if selected.IsZero() {
		selected = time.Now()
	}
	selected = dayAtMidnight(selected)

	lines := []string{
		title,
		"",
		styles.Hint.Render("Day Off Datepicker"),
		selected.Format("2006-01-02 (Mon)"),
		"",
		renderCalendarMonth(selected),
		"",
		styles.Hint.Render("h/l or ←/→ day, j/k or ↑/↓ week, PgUp/PgDn month, Enter confirm, Esc cancel"),
	}
	return strings.Join(lines, "\n")
}

func renderCalendarMonth(selected time.Time) string {
	firstOfMonth := time.Date(selected.Year(), selected.Month(), 1, 0, 0, 0, 0, selected.Location())
	lastOfMonth := time.Date(selected.Year(), selected.Month()+1, 0, 0, 0, 0, 0, selected.Location())

	weekdayOffset := int(firstOfMonth.Weekday())
	if weekdayOffset == 0 {
		weekdayOffset = 7
	}
	weekdayOffset--

	var builder strings.Builder
	builder.WriteString(selected.Format("January 2006"))
	builder.WriteString("\nMo Tu We Th Fr Sa Su\n")

	for idx := 0; idx < weekdayOffset; idx++ {
		builder.WriteString("   ")
	}

	for day := 1; day <= lastOfMonth.Day(); day++ {
		current := time.Date(selected.Year(), selected.Month(), day, 0, 0, 0, 0, selected.Location())
		cell := fmt.Sprintf("%2d", day)
		if sameDay(current, selected) {
			cell = "[" + cell + "]"
		} else {
			cell = " " + cell + " "
		}
		builder.WriteString(cell)

		column := (weekdayOffset + day) % 7
		if column == 0 {
			builder.WriteByte('\n')
		} else {
			builder.WriteByte(' ')
		}
	}

	output := strings.TrimRight(builder.String(), " \n")
	return output
}

func dayAtMidnight(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func sameDay(a, b time.Time) bool {
	a = dayAtMidnight(a)
	b = dayAtMidnight(b)
	return a.Equal(b)
}

func (m *EntriesModel) appendEntry(entry worktime.Entry) {
	m.allEntries = append(m.allEntries, entry)
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
	m.mutationCount++
	m.markHostChanged(entry.Source)

	if idx := findEntryIndex(m.visible, entry); idx >= 0 {
		m.cursor = idx
		m.ensureCursorVisible()
	}
}

func (m *EntriesModel) replaceEntry(oldEntry, newEntry worktime.Entry) {
	idx := findEntryIndex(m.allEntries, oldEntry)
	if idx < 0 {
		return
	}

	m.allEntries[idx] = newEntry
	m.applyFilters()
	m.mutationCount++
	m.markHostChanged(oldEntry.Source)
	m.markHostChanged(newEntry.Source)

	if newIdx := findEntryIndex(m.visible, newEntry); newIdx >= 0 {
		m.cursor = newIdx
		m.ensureCursorVisible()
	}
}

func (m *EntriesModel) persistReplacement(oldEntry, newEntry worktime.Entry) error {
	if m.dbDir == "" {
		return nil
	}

	host := strings.TrimSpace(oldEntry.Source)
	if host == "" {
		return errors.New("selected entry has no source host")
	}

	m.markHostChanged(host)

	newHost := strings.TrimSpace(newEntry.Source)
	if newHost != "" && newHost != host {
		m.markHostChanged(newHost)
	}

	return nil
}

func (m *EntriesModel) persistDelete(target worktime.Entry) error {
	if m.dbDir == "" {
		return nil
	}

	host := strings.TrimSpace(target.Source)
	if host == "" {
		return errors.New("selected entry has no source host")
	}

	m.markHostChanged(host)
	return nil
}

func (m *EntriesModel) markHostChanged(host string) {
	normalized := strings.TrimSpace(host)
	if normalized == "" {
		return
	}

	if m.changedHosts == nil {
		m.changedHosts = map[string]struct{}{}
	}
	m.changedHosts[normalized] = struct{}{}
	m.dirty = true
}

func (m *EntriesModel) savePendingChanges() error {
	if m.dbDir == "" {
		return errors.New("persistence is not configured")
	}
	if !m.dirty || len(m.changedHosts) == 0 {
		m.setStatusInfo("No unsaved changes.")
		return nil
	}

	hosts := make([]string, 0, len(m.changedHosts))
	for host := range m.changedHosts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	for _, host := range hosts {
		hostEntries := make([]worktime.Entry, 0)
		for _, entry := range m.allEntries {
			if strings.TrimSpace(entry.Source) == host {
				hostEntries = append(hostEntries, entry)
			}
		}

		db := worktime.Database{
			Entries: map[string][]worktime.Entry{
				host: hostEntries,
			},
		}
		if err := worktime.SaveHost(m.dbDir, host, db); err != nil {
			return err
		}
	}

	m.changedHosts = map[string]struct{}{}
	m.dirty = false
	m.setStatusInfo(fmt.Sprintf("Saved %d host database(s).", len(hosts)))
	return nil
}

func (m EntriesModel) hasUnsavedChanges() bool {
	return m.dirty && len(m.changedHosts) > 0
}

func (m *EntriesModel) setStatusInfo(message string) {
	m.statusMessage = strings.TrimSpace(message)
	m.statusError = false
}

func (m *EntriesModel) setStatusError(message string) {
	m.statusMessage = strings.TrimSpace(message)
	m.statusError = true
}

func (m *EntriesModel) renderStatus(styles Styles) string {
	if strings.TrimSpace(m.statusMessage) == "" {
		return ""
	}

	if m.statusError {
		return "\n\n" + styles.Error.Render(m.statusMessage)
	}

	return "\n\n" + styles.Hint.Render(m.statusMessage)
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

func (m *EntriesModel) listRows() int {
	rows := m.height - 6
	if rows < 1 {
		return 1
	}
	return rows
}

func (m *EntriesModel) halfPage() int {
	page := m.listRows() / 2
	if page < 1 {
		return 1
	}
	return page
}

func (m *EntriesModel) pageSize() int {
	page := m.listRows()
	if page < 1 {
		return 1
	}
	return page
}

func formatEntryValue(entry worktime.Entry, sessionDurations map[worktime.Entry]int64) string {
	action := strings.ToLower(strings.TrimSpace(entry.Action))
	switch action {
	case "add":
		hours := float64(entry.Value) / 3600
		return fmt.Sprintf("%+.2fh", hours)
	case "login", "logout":
		span, ok := sessionDurations[entry]
		if !ok || span <= 0 {
			return "-"
		}
		hours := float64(span) / 3600
		return fmt.Sprintf("+%.2fh", hours)
	default:
		return "-"
	}
}

func buildSessionDurations(entries []worktime.Entry) map[worktime.Entry]int64 {
	if len(entries) == 0 {
		return map[worktime.Entry]int64{}
	}

	sorted := append([]worktime.Entry(nil), entries...)
	slices.SortFunc(sorted, func(a, b worktime.Entry) int {
		if a.Epoch != b.Epoch {
			if a.Epoch < b.Epoch {
				return -1
			}
			return 1
		}
		if a.Source != b.Source {
			return strings.Compare(a.Source, b.Source)
		}
		return strings.Compare(a.Action, b.Action)
	})

	activeByCategory := map[string]worktime.Entry{}
	durations := map[worktime.Entry]int64{}
	for _, entry := range sorted {
		action := strings.ToLower(strings.TrimSpace(entry.Action))
		category := normalizeEntryCategory(entry.What)

		switch action {
		case "login":
			activeByCategory[category] = entry
		case "logout":
			loginEntry, ok := activeByCategory[category]
			if !ok {
				continue
			}

			span := entry.Epoch - loginEntry.Epoch
			if span > 0 {
				durations[loginEntry] = span
				durations[entry] = span
			}
			delete(activeByCategory, category)
		}
	}

	return durations
}

func normalizeEntryCategory(raw string) string {
	category := strings.TrimSpace(raw)
	if category == "" {
		return "work"
	}
	return category
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func findEntryIndex(entries []worktime.Entry, target worktime.Entry) int {
	for idx, entry := range entries {
		if sameEntryIdentity(entry, target) {
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

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}

	_, size := utf8.DecodeLastRuneInString(value)
	if size <= 0 {
		return ""
	}

	return value[:len(value)-size]
}

func appendInputKey(input string, keyMsg tea.KeyPressMsg) string {
	if keyMsg.String() == "space" {
		return input + " "
	}

	if keyMsg.Text != "" {
		return input + keyMsg.Text
	}

	return input
}

func entryMatchesSearch(entry worktime.Entry, search string) bool {
	return strings.Contains(strings.ToLower(entry.Action), search) ||
		strings.Contains(strings.ToLower(entry.What), search) ||
		strings.Contains(strings.ToLower(entry.Source), search) ||
		strings.Contains(strings.ToLower(entry.Human), search) ||
		strings.Contains(strings.ToLower(entry.Descr), search)
}

func (m *EntriesModel) renderTimelineTable(styles Styles) string {
	widths := m.timelineColumnWidths()
	headers := []string{"Date", "Time", "Action", "Category", "Value", "Source", "Description"}
	sessionDurations := buildSessionDurations(m.allEntries)

	headerStyles := make([]lipgloss.Style, len(headers))
	for idx := range headers {
		headerStyles[idx] = styles.TableHeader
		if entriesColumn(idx) == m.selectedColumn {
			headerStyles[idx] = styles.TableSelected
		}
	}

	lines := []string{
		renderTimelineRow(headers, widths, headerStyles),
	}

	maxRows := m.listRows()
	end := minInt(len(m.visible), m.offset+maxRows)
	for idx := m.offset; idx < end; idx++ {
		entry := m.visible[idx]
		moment := time.Unix(entry.Epoch, 0)
		row := []string{
			moment.Format("2006-01-02"),
			moment.Format("15:04"),
			entry.Action,
			entry.What,
			formatEntryValue(entry, sessionDurations),
			entry.Source,
			entry.Descr,
		}

		cellStyles := make([]lipgloss.Style, len(row))
		for colIdx := range row {
			cellStyles[colIdx] = styles.TableCell
			if idx == m.cursor {
				cellStyles[colIdx] = styles.TableCell.Bold(true)
				if entriesColumn(colIdx) == m.selectedColumn {
					cellStyles[colIdx] = styles.TableSelected
				}
			}
		}
		lines = append(lines, renderTimelineRow(row, widths, cellStyles))
	}

	return strings.Join(lines, "\n")
}

func (m *EntriesModel) timelineColumnWidths() []int {
	total := m.width
	if total <= 0 {
		total = 120
	}

	dateWidth := 10
	timeWidth := 5
	actionWidth := 8
	categoryWidth := 12
	valueWidth := 9
	sourceWidth := 12

	fixed := dateWidth + timeWidth + actionWidth + categoryWidth + valueWidth + sourceWidth + 6
	descriptionWidth := total - fixed
	if descriptionWidth < 16 {
		descriptionWidth = 16
	}

	return []int{
		dateWidth,
		timeWidth,
		actionWidth,
		categoryWidth,
		valueWidth,
		sourceWidth,
		descriptionWidth,
	}
}

func renderTimelineRow(values []string, widths []int, styles []lipgloss.Style) string {
	cells := make([]string, 0, len(values))
	for idx, raw := range values {
		width := widths[idx]
		cell := lipgloss.NewStyle().Width(width).MaxWidth(width).Render(trimToWidth(raw, width))
		cells = append(cells, styles[idx].Render(cell))
	}
	return strings.Join(cells, " ")
}

func trimToWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func parseEditedDate(input string, current time.Time) (time.Time, error) {
	parsed, err := timefmt.Parse(strings.TrimSpace(input))
	if err != nil {
		return time.Time{}, err
	}

	year, month, day := parsed.Date()
	hour, minute, second := current.Clock()
	return time.Date(year, month, day, hour, minute, second, 0, current.Location()), nil
}

func parseEditedTime(input string, current time.Time) (time.Time, error) {
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return time.Time{}, errors.New("time value must not be empty")
	}

	layouts := []string{"15:04", "15:04:05"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, candidate, current.Location())
		if err == nil {
			year, month, day := current.Date()
			hour, minute, second := parsed.Clock()
			return time.Date(year, month, day, hour, minute, second, 0, current.Location()), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported time format %q", input)
}

func sameEntryIdentity(a, b worktime.Entry) bool {
	return a.Epoch == b.Epoch && a.Action == b.Action && a.Source == b.Source
}
