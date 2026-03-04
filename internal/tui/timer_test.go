package tui

import (
	"testing"

	"codeberg.org/snonux/timesamurai/internal/config"
	"codeberg.org/snonux/timesamurai/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTimerModelToggleWorkLogin(t *testing.T) {
	setupTimerStateForTUI(t)

	dbDir := t.TempDir()
	cfg := config.Default()
	cfg.WorktimeDBDir = dbDir
	cfg.Hostname = "host-a"

	model, err := NewTimerModel("doom", cfg)
	if err != nil {
		t.Fatalf("NewTimerModel() error = %v", err)
	}

	if !model.work.enabled {
		t.Fatal("work integration should be enabled")
	}

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = modelAny.(TimerModel)
	if !model.work.loggedIn {
		t.Fatal("work should be logged in after first l")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = modelAny.(TimerModel)
	if model.work.loggedIn {
		t.Fatal("work should be logged out after second l")
	}

	entries, err := worktime.LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("entries len = %d, want at least 2", len(entries))
	}
}

func TestTimerModelFontCycling(t *testing.T) {
	setupTimerStateForTUI(t)

	model, err := NewTimerModel("doom", config.Default())
	if err != nil {
		t.Fatalf("NewTimerModel() error = %v", err)
	}

	originalFont := model.font
	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = modelAny.(TimerModel)
	if model.font == originalFont {
		t.Fatalf("font did not change after f: %q", model.font)
	}
}

func TestTimerModelWorkToggleWhenDisabled(t *testing.T) {
	setupTimerStateForTUI(t)

	cfg := config.Default()
	cfg.WorktimeDBDir = ""

	model, err := NewTimerModel("doom", cfg)
	if err != nil {
		t.Fatalf("NewTimerModel() error = %v", err)
	}

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = modelAny.(TimerModel)
	if model.work.status != "work integration disabled" {
		t.Fatalf("workStatus = %q, want work integration disabled", model.work.status)
	}
}

func setupTimerStateForTUI(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)
}
