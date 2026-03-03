package tui

import (
	"fmt"
	"testing"
	"time"

	"codeberg.org/snonux/timr/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
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

func TestEntriesNavigationKeys(t *testing.T) {
	model := NewEntriesModel(sampleEntries(20))
	model.SetSize(120, 12)

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if model.cursor != 1 {
		t.Fatalf("cursor after j = %d, want 1", model.cursor)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if model.cursor != 0 {
		t.Fatalf("cursor after k = %d, want 0", model.cursor)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if model.cursor != len(model.visible)-1 {
		t.Fatalf("cursor after G = %d, want %d", model.cursor, len(model.visible)-1)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if model.cursor != 0 {
		t.Fatalf("cursor after gg = %d, want 0", model.cursor)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if model.cursor == 0 {
		t.Fatal("cursor did not move after ctrl+d")
	}

	before := model.cursor
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	if model.cursor >= before {
		t.Fatalf("cursor after ctrl+u = %d, want less than %d", model.cursor, before)
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
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(model.visible) != 1 || model.visible[0].Descr != "meeting" {
		t.Fatalf("search results mismatch: %+v", model.visible)
	}

	// Apply filter by category "work" on top of search.
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(model.visible) != 1 || model.visible[0].What != "work" {
		t.Fatalf("filter results mismatch: %+v", model.visible)
	}
}

func TestEntriesEditFlow(t *testing.T) {
	model := NewEntriesModel(sampleEntries(3))
	model.SetSize(120, 12)

	original := model.visible[0].Descr

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !model.editMode {
		t.Fatal("editMode = false, want true after e")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if model.editMode {
		t.Fatal("editMode = true, want false after Enter")
	}
	if model.visible[0].Descr != original+"!" {
		t.Fatalf("edited description = %q, want %q", model.visible[0].Descr, original+"!")
	}
}

func TestEntriesDeleteWithConfirmation(t *testing.T) {
	model := NewEntriesModel(sampleEntries(3))
	model.SetSize(120, 12)

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !model.confirmDelete {
		t.Fatal("confirmDelete = false, want true after dd")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if model.confirmDelete {
		t.Fatal("confirmDelete = true after cancel")
	}
	if len(model.visible) != 3 {
		t.Fatalf("entries len = %d, want 3 after cancel", len(model.visible))
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})

	if len(model.visible) != 2 {
		t.Fatalf("entries len = %d, want 2 after delete confirmation", len(model.visible))
	}
}

func TestEntriesInsertWithOAndShiftO(t *testing.T) {
	model := NewEntriesModel(sampleEntries(2))
	model.SetSize(120, 12)

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if len(model.visible) != 3 {
		t.Fatalf("entries len = %d, want 3 after o", len(model.visible))
	}
	if !model.editMode {
		t.Fatal("editMode = false after o insertion")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if len(model.visible) != 4 {
		t.Fatalf("entries len = %d, want 4 after O", len(model.visible))
	}
}

func sampleEntries(count int) []worktime.Entry {
	entries := make([]worktime.Entry, 0, count)
	for idx := 0; idx < count; idx++ {
		entries = append(entries, worktime.Entry{
			Action: "add",
			What:   "work",
			Epoch:  int64(1000 + idx),
			Descr:  fmt.Sprintf("entry-%d", idx),
			Value:  int64(time.Hour / time.Second),
		})
	}
	return entries
}

func localEpoch(year int, month time.Month, day int, hour int) int64 {
	return time.Date(year, month, day, hour, 0, 0, 0, time.Local).Unix()
}
