package tui

import (
	"path/filepath"
	"testing"

	"codeberg.org/snonux/timr/internal/config"
	timrTimer "codeberg.org/snonux/timr/internal/timer"
	"codeberg.org/snonux/timr/internal/worktime"
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

	if !model.workEnabled {
		t.Fatal("work integration should be enabled")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if !model.workLoggedIn {
		t.Fatal("work should be logged in after first l")
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if model.workLoggedIn {
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
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
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

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if model.workStatus != "work integration disabled" {
		t.Fatalf("workStatus = %q, want work integration disabled", model.workStatus)
	}
}

func setupTimerStateForTUI(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	timrTimer.SetStateFilePathOverride(filepath.Join(tempDir, ".timr_state"))
	t.Cleanup(func() {
		timrTimer.SetStateFilePathOverride("")
	})
}
