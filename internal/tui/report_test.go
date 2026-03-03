package tui

import (
	"strings"
	"testing"

	"codeberg.org/snonux/timr/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
)

func TestReportWeekNavigation(t *testing.T) {
	model := NewReportModel(sampleWeeks())
	model.SetSize(120, 12)

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if model.weekIndex != 1 {
		t.Fatalf("weekIndex after ]w = %d, want 1", model.weekIndex)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if model.weekIndex != 0 {
		t.Fatalf("weekIndex after [w = %d, want 0", model.weekIndex)
	}
}

func TestReportScrollingAndTopBottom(t *testing.T) {
	model := NewReportModel(sampleWeeks())
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
	if model.cursor != model.rowCount()-1 {
		t.Fatalf("cursor after G = %d, want %d", model.cursor, model.rowCount()-1)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if model.cursor != 0 {
		t.Fatalf("cursor after gg = %d, want 0", model.cursor)
	}
}

func TestReportVerboseToggle(t *testing.T) {
	model := NewReportModel(sampleWeeks())
	model.SetSize(120, 12)

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if !model.verbose {
		t.Fatal("verbose = false, want true after v")
	}

	view := model.View(DefaultStyles())
	if !strings.Contains(view, "epoch:") {
		t.Fatalf("verbose view missing epoch details: %q", view)
	}
}

func TestReportSummaryBarInView(t *testing.T) {
	model := NewReportModel(sampleWeeks())
	model.SetSize(120, 12)

	view := model.View(DefaultStyles())
	if !strings.Contains(view, "Balance:") {
		t.Fatalf("view missing summary balance: %q", view)
	}
	if !strings.Contains(view, "Work:") {
		t.Fatalf("view missing summary work: %q", view)
	}
	if !strings.Contains(view, "Buffer:") {
		t.Fatalf("view missing summary buffer: %q", view)
	}
}

func sampleWeeks() []worktime.WeekReport {
	return []worktime.WeekReport{
		{
			WeekLabel:                "10",
			CumulativeBalanceSeconds: 2 * 3600,
			BufferSeconds:            1 * 3600,
			Values: map[string]int64{
				"work": 20 * 3600,
			},
			Days: []worktime.DayReport{
				{DayLabel: "Mon 20260302 10", Marker: " ", Epoch: 1, Values: map[string]int64{"work": 8 * 3600}},
				{DayLabel: "Tue 20260303 10", Marker: " ", Epoch: 2, Values: map[string]int64{"work": 7 * 3600, "lunch": 3600}},
				{DayLabel: "Wed 20260304 10", Marker: "*", Epoch: 3, Values: map[string]int64{"off": 8 * 3600}},
			},
		},
		{
			WeekLabel:                "11",
			CumulativeBalanceSeconds: 3 * 3600,
			BufferSeconds:            2 * 3600,
			Values: map[string]int64{
				"work": 18 * 3600,
			},
			Days: []worktime.DayReport{
				{DayLabel: "Mon 20260309 11", Marker: " ", Epoch: 4, Values: map[string]int64{"work": 9 * 3600}},
				{DayLabel: "Tue 20260310 11", Marker: " ", Epoch: 5, Values: map[string]int64{"work": 9 * 3600}},
			},
		},
	}
}
