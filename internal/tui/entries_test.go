package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/snonux/timesamurai/internal/worktime"
)

func TestEntriesModelSortsChronologically(t *testing.T) {
	entries := []worktime.Entry{
		{Action: "add", What: "work", Epoch: 10, Descr: "old"},
		{Action: "add", What: "work", Epoch: 30, Descr: "new"},
		{Action: "add", What: "work", Epoch: 20, Descr: "mid"},
	}

	model := NewEntriesModel(entries)
	if len(model.visible) != 3 {
		t.Fatalf("visible len = %d, want 3", len(model.visible))
	}
	if model.visible[0].Epoch != 30 || model.visible[1].Epoch != 20 || model.visible[2].Epoch != 10 {
		t.Fatalf("entries are not sorted descending by epoch: %+v", model.visible)
	}
}

func TestEntriesViewRendersTimelineTableHeaders(t *testing.T) {
	model := NewEntriesModel(sampleEntries(2))
	model.SetSize(140, 12)

	view := model.View(DefaultStyles())
	if !strings.Contains(view, "Date") || !strings.Contains(view, "Time") || !strings.Contains(view, "Description") {
		t.Fatalf("timeline table headers missing from view: %q", view)
	}
}

func TestEntriesColumnNavigationKeys(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.SetSize(120, 12)

	if model.selectedColumn != entriesColumnDescription {
		t.Fatalf("selectedColumn = %d, want %d", model.selectedColumn, entriesColumnDescription)
	}

	model, _ = model.Update(keyCode(tea.KeyLeft))
	if model.selectedColumn != entriesColumnSource {
		t.Fatalf("selectedColumn after left = %d, want %d", model.selectedColumn, entriesColumnSource)
	}

	model, _ = model.Update(keyRune('h'))
	if model.selectedColumn != entriesColumnValue {
		t.Fatalf("selectedColumn after h = %d, want %d", model.selectedColumn, entriesColumnValue)
	}

	model, _ = model.Update(keyRune('l'))
	if model.selectedColumn != entriesColumnSource {
		t.Fatalf("selectedColumn after l = %d, want %d", model.selectedColumn, entriesColumnSource)
	}
}

func TestEntriesEnterEditsSelectedColumnValue(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.SetSize(120, 12)
	model.selectedColumn = entriesColumnValue

	model, _ = model.Update(keyCode(tea.KeyEnter))
	if !model.editMode {
		t.Fatal("editMode = false, want true after Enter on value column")
	}
	if model.editField != entryEditFieldValue {
		t.Fatalf("editField = %d, want %d", model.editField, entryEditFieldValue)
	}
}

func TestEntriesEnterEditsSelectedColumnDateAndTime(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.SetSize(120, 12)

	original := model.visible[0].Epoch

	model.selectedColumn = entriesColumnDate
	model, _ = model.Update(keyCode(tea.KeyEnter))
	if !model.editMode || model.editField != entryEditFieldDate {
		t.Fatalf("date edit not entered: editMode=%t editField=%d", model.editMode, model.editField)
	}
	model.input = "2026-02-02"
	model, _ = model.Update(keyCode(tea.KeyEnter))
	if model.visible[0].Epoch == original {
		t.Fatal("epoch did not change after date edit")
	}

	model.selectedColumn = entriesColumnTime
	model, _ = model.Update(keyCode(tea.KeyEnter))
	if !model.editMode || model.editField != entryEditFieldTime {
		t.Fatalf("time edit not entered: editMode=%t editField=%d", model.editMode, model.editField)
	}
	model.input = "13:45"
	model, _ = model.Update(keyCode(tea.KeyEnter))

	updatedTime := time.Unix(model.visible[0].Epoch, 0)
	if updatedTime.Hour() != 13 || updatedTime.Minute() != 45 {
		t.Fatalf("time after edit = %s, want 13:45", updatedTime.Format("15:04"))
	}
}

func TestEntriesNavigationKeys(t *testing.T) {
	model := NewEntriesModel(sampleEntries(20))
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('j'))
	if model.cursor != 1 {
		t.Fatalf("cursor after j = %d, want 1", model.cursor)
	}

	model, _ = model.Update(keyRune('k'))
	if model.cursor != 0 {
		t.Fatalf("cursor after k = %d, want 0", model.cursor)
	}

	model, _ = model.Update(keyRune('G'))
	if model.cursor != len(model.visible)-1 {
		t.Fatalf("cursor after G = %d, want %d", model.cursor, len(model.visible)-1)
	}

	model, _ = model.Update(keyRune('g'))
	model, _ = model.Update(keyRune('g'))
	if model.cursor != 0 {
		t.Fatalf("cursor after gg = %d, want 0", model.cursor)
	}

	model, _ = model.Update(keyCtrl('d'))
	if model.cursor == 0 {
		t.Fatal("cursor did not move after ctrl+d")
	}

	before := model.cursor
	model, _ = model.Update(keyCtrl('u'))
	if model.cursor >= before {
		t.Fatalf("cursor after ctrl+u = %d, want less than %d", model.cursor, before)
	}

	model.cursor = 0
	model.offset = 0
	model, _ = model.Update(keyCode(tea.KeyPgDown))
	if got, want := model.cursor, model.pageSize(); got != want {
		t.Fatalf("cursor after pgdown = %d, want %d", got, want)
	}

	model, _ = model.Update(keyCode(tea.KeyPgUp))
	if model.cursor != 0 {
		t.Fatalf("cursor after pgup = %d, want 0", model.cursor)
	}
}

func TestEntriesSearchAndFilter(t *testing.T) {
	entries := []worktime.Entry{
		{Action: "add", What: "work", Epoch: localEpoch(2026, 1, 5, 10), Descr: "meeting"},
		{Action: "add", What: "off", Epoch: localEpoch(2026, 1, 4, 10), Descr: "vacation"},
		{Action: "add", What: "work", Epoch: localEpoch(2026, 1, 3, 10), Descr: "coding"},
	}

	model := NewEntriesModel(entries)
	model.SetSize(120, 12)

	// Search for "meeting".
	model, _ = model.Update(keyRune('/'))
	model, _ = model.Update(keyRune('m'))
	model, _ = model.Update(keyRune('e'))
	model, _ = model.Update(keyRune('e'))
	model, _ = model.Update(keyRune('t'))
	model, _ = model.Update(keyRune('i'))
	model, _ = model.Update(keyRune('n'))
	model, _ = model.Update(keyRune('g'))
	model, _ = model.Update(keyCode(tea.KeyEnter))

	if len(model.visible) != 1 || model.visible[0].Descr != "meeting" {
		t.Fatalf("search results mismatch: %+v", model.visible)
	}

	// Apply filter by category "work" on top of search.
	model, _ = model.Update(keyRune('f'))
	model, _ = model.Update(keyRune('w'))
	model, _ = model.Update(keyRune('o'))
	model, _ = model.Update(keyRune('r'))
	model, _ = model.Update(keyRune('k'))
	model, _ = model.Update(keyCode(tea.KeyEnter))

	if len(model.visible) != 1 || model.visible[0].What != "work" {
		t.Fatalf("filter results mismatch: %+v", model.visible)
	}
}

func TestEntriesEditFlow(t *testing.T) {
	model := NewEntriesModel(sampleEntries(3))
	model.SetSize(120, 12)

	original := model.visible[0].Descr

	model, _ = model.Update(keyRune('e'))
	if !model.editMode {
		t.Fatal("editMode = false, want true after e")
	}

	model, _ = model.Update(keyRune('!'))
	model, _ = model.Update(keyCode(tea.KeyEnter))

	if model.editMode {
		t.Fatal("editMode = true, want false after Enter")
	}
	if model.visible[0].Descr != original+"!" {
		t.Fatalf("edited description = %q, want %q", model.visible[0].Descr, original+"!")
	}
}

func TestEntriesEditModeAcceptsSpaceKey(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.editMode = true
	model.input = "hello"

	model, _ = model.Update(keyRune(' '))
	if model.input != "hello " {
		t.Fatalf("input after space = %q, want %q", model.input, "hello ")
	}
}

func TestEntriesValueShowsSessionDurationForLoginLogoutPairs(t *testing.T) {
	entries := []worktime.Entry{
		{
			Action: "login",
			What:   "work",
			Epoch:  localEpoch(2026, 1, 5, 9),
			Source: "host-a",
			Human:  time.Unix(localEpoch(2026, 1, 5, 9), 0).Format("Mon 02.01.2006 15:04:05"),
		},
		{
			Action: "logout",
			What:   "work",
			Epoch:  localEpoch(2026, 1, 5, 11),
			Source: "host-a",
			Human:  time.Unix(localEpoch(2026, 1, 5, 11), 0).Format("Mon 02.01.2006 15:04:05"),
		},
	}

	model := NewEntriesModel(entries)
	model.SetSize(120, 12)

	table := model.renderTimelineTable(DefaultStyles())
	if count := strings.Count(table, "+2.00h"); count != 2 {
		t.Fatalf("rendered session duration count = %d, want 2 in table:\n%s", count, table)
	}
}

func TestEntriesValueEditFlow(t *testing.T) {
	model := NewEntriesModel(sampleEntries(3))
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('v'))
	if !model.editMode {
		t.Fatal("editMode = false, want true after v")
	}

	model.input = "120"
	model, _ = model.Update(keyCode(tea.KeyEnter))
	if model.editMode {
		t.Fatal("editMode = true, want false after Enter")
	}
	if model.visible[0].Value != 120 {
		t.Fatalf("edited value = %d, want 120", model.visible[0].Value)
	}
}

func TestEntriesBackspaceIsRuneSafeInEditMode(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.editMode = true
	model.input = "aй"

	model, _ = model.Update(keyCode(tea.KeyBackspace))
	if model.input != "a" {
		t.Fatalf("input after backspace = %q, want %q", model.input, "a")
	}
}

func TestEntriesBackspaceIsRuneSafeInSearchMode(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.searchMode = true
	model.input = "тест"

	model, _ = model.Update(keyCode(tea.KeyBackspace))
	if model.input != "тес" {
		t.Fatalf("input after backspace = %q, want %q", model.input, "тес")
	}
}

func TestEntriesDeleteWithConfirmation(t *testing.T) {
	model := NewEntriesModel(sampleEntries(3))
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('d'))
	model, _ = model.Update(keyRune('d'))
	if !model.confirmDelete {
		t.Fatal("confirmDelete = false, want true after dd")
	}

	model, _ = model.Update(keyRune('n'))
	if model.confirmDelete {
		t.Fatal("confirmDelete = true after cancel")
	}
	if len(model.visible) != 3 {
		t.Fatalf("entries len = %d, want 3 after cancel", len(model.visible))
	}

	model, _ = model.Update(keyRune('d'))
	model, _ = model.Update(keyRune('d'))
	model, _ = model.Update(keyRune('y'))

	if len(model.visible) != 2 {
		t.Fatalf("entries len = %d, want 2 after delete confirmation", len(model.visible))
	}
}

func TestEntriesInsertWithOAndShiftO(t *testing.T) {
	model := NewEntriesModel(sampleEntries(2))
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('o'))
	if len(model.visible) != 3 {
		t.Fatalf("entries len = %d, want 3 after o", len(model.visible))
	}
	if !model.editMode {
		t.Fatal("editMode = false after o insertion")
	}
	model, _ = model.Update(keyCode(tea.KeyEscape))

	model, _ = model.Update(keyRune('O'))
	if len(model.visible) != 4 {
		t.Fatalf("entries len = %d, want 4 after O", len(model.visible))
	}
}

func TestEntriesDeletePersistsToDB(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	db := worktime.Database{
		Entries: map[string][]worktime.Entry{
			host: sampleEntries(3),
		},
	}
	if err := worktime.SaveHost(dbDir, host, db); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	entries, err := worktime.LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	model := NewEntriesModel(entries)
	model.SetPersistence(dbDir, host)
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('d'))
	model, _ = model.Update(keyRune('d'))
	model, _ = model.Update(keyRune('y'))

	if len(model.visible) != 2 {
		t.Fatalf("entries len = %d, want 2 after persisted delete", len(model.visible))
	}
	if !model.hasUnsavedChanges() {
		t.Fatal("hasUnsavedChanges = false, want true after delete before save")
	}

	reloaded, err := worktime.LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}
	if len(reloaded.Entries[host]) != 3 {
		t.Fatalf("host entries len before save = %d, want 3", len(reloaded.Entries[host]))
	}

	model, _ = model.Update(keyRune('s'))
	if model.hasUnsavedChanges() {
		t.Fatal("hasUnsavedChanges = true, want false after save")
	}

	reloaded, err = worktime.LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}
	if len(reloaded.Entries[host]) != 2 {
		t.Fatalf("host entries len after save = %d, want 2", len(reloaded.Entries[host]))
	}
}

func TestEntriesDayOffPromptPersistsToDB(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	db := worktime.Database{
		Entries: map[string][]worktime.Entry{
			host: {},
		},
	}
	if err := worktime.SaveHost(dbDir, host, db); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	model := NewEntriesModel(nil)
	model.SetPersistence(dbDir, host)
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('D'))
	if !model.dayOffMode {
		t.Fatal("dayOffMode = false, want true after D")
	}

	model.dayOffDate = time.Date(2026, 1, 22, 12, 34, 0, 0, time.Local)
	model, _ = model.Update(keyCode(tea.KeyEnter))
	if model.dayOffMode {
		t.Fatal("dayOffMode = true, want false after Enter")
	}
	if !model.hasUnsavedChanges() {
		t.Fatal("hasUnsavedChanges = false, want true after day off before save")
	}

	db, err := worktime.LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}

	entries := db.Entries[host]
	if len(entries) != 0 {
		t.Fatalf("entries len before save = %d, want 0", len(entries))
	}

	model, _ = model.Update(keyRune('s'))
	if model.hasUnsavedChanges() {
		t.Fatal("hasUnsavedChanges = true, want false after save")
	}

	db, err = worktime.LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}

	entries = db.Entries[host]
	if len(entries) != 1 {
		t.Fatalf("entries len after save = %d, want 1", len(entries))
	}

	entry := entries[0]
	wantEpoch := time.Date(2026, 1, 22, 0, 0, 0, 0, time.Local).Unix()
	if entry.What != "off" || entry.Action != "add" {
		t.Fatalf("unexpected day-off entry: %+v", entry)
	}
	if entry.Value != 8*3600 {
		t.Fatalf("entry.Value = %d, want 28800", entry.Value)
	}
	if entry.Epoch != wantEpoch {
		t.Fatalf("entry.Epoch = %d, want %d", entry.Epoch, wantEpoch)
	}
}

func TestEntriesDayOffDatepickerNavigation(t *testing.T) {
	model := NewEntriesModel(sampleEntries(1))
	model.SetSize(120, 12)

	model, _ = model.Update(keyRune('D'))
	if !model.dayOffMode {
		t.Fatal("dayOffMode = false, want true after D")
	}

	model.dayOffDate = time.Date(2026, 2, 12, 0, 0, 0, 0, time.Local)
	model, _ = model.Update(keyCode(tea.KeyRight))
	if !sameDay(model.dayOffDate, time.Date(2026, 2, 13, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("dayOffDate after right = %v, want 2026-02-13", model.dayOffDate)
	}

	model, _ = model.Update(keyCode(tea.KeyDown))
	if !sameDay(model.dayOffDate, time.Date(2026, 2, 20, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("dayOffDate after down = %v, want 2026-02-20", model.dayOffDate)
	}

	model, _ = model.Update(keyCode(tea.KeyPgUp))
	if !sameDay(model.dayOffDate, time.Date(2026, 1, 20, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("dayOffDate after pgup = %v, want 2026-01-20", model.dayOffDate)
	}
}

func sampleEntries(count int) []worktime.Entry {
	entries := make([]worktime.Entry, 0, count)
	for idx := 0; idx < count; idx++ {
		entries = append(entries, worktime.Entry{
			Action: "add",
			What:   "work",
			Epoch:  int64(1000 + idx),
			Source: "host-a",
			Human:  time.Unix(int64(1000+idx), 0).Format("Mon 02.01.2006 15:04:05"),
			Descr:  fmt.Sprintf("entry-%d", idx),
			Value:  int64(time.Hour / time.Second),
		})
	}
	return entries
}

func localEpoch(year int, month time.Month, day int, hour int) int64 {
	return time.Date(year, month, day, hour, 0, 0, 0, time.Local).Unix()
}
