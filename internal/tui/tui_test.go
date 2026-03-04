package tui

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/timesamurai/internal/config"
	"codeberg.org/snonux/timesamurai/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTabNavigation(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = modelAny.(*Model)
	if model.activeTab != tabReport {
		t.Fatalf("active tab after Tab = %v, want %v", model.activeTab, tabReport)
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = modelAny.(*Model)
	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	model = modelAny.(*Model)
	if model.activeTab != tabEntries {
		t.Fatalf("active tab after gT = %v, want %v", model.activeTab, tabEntries)
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	model = modelAny.(*Model)
	if model.activeTab != tabTimer {
		t.Fatalf("active tab after key 3 = %v, want %v", model.activeTab, tabTimer)
	}
}

func TestHelpToggle(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = modelAny.(*Model)
	if !model.showHelp {
		t.Fatal("showHelp = false, want true after ?")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = modelAny.(*Model)
	if model.showHelp {
		t.Fatal("showHelp = true, want false after second ?")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	model = modelAny.(*Model)
	if !model.showHelp {
		t.Fatal("showHelp = false, want true after H")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = modelAny.(*Model)
	if model.showHelp {
		t.Fatal("showHelp = true, want false after Esc")
	}
}

func TestQuitKeys(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = modelAny.(*Model)
	if cmd == nil {
		t.Fatal("quit cmd is nil for q")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q key did not return tea.Quit command")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	model = modelAny.(*Model)
	modelAny, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	model = modelAny.(*Model)
	if cmd == nil {
		t.Fatal("quit cmd is nil for ZQ")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ZQ did not return tea.Quit command")
	}
}

func TestQuitWithUnsavedChangesPromptsConfirmation(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	model = modelAny.(*Model)
	if !model.entries.hasUnsavedChanges() {
		t.Fatal("entries.hasUnsavedChanges() = false, want true after insertion")
	}

	modelAny, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = modelAny.(*Model)
	if cmd != nil {
		t.Fatal("quit command should be deferred until quit confirmation")
	}
	if !model.confirmQuit {
		t.Fatal("confirmQuit = false, want true after q with unsaved changes")
	}

	modelAny, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = modelAny.(*Model)
	if cmd != nil {
		t.Fatal("Esc in quit confirmation should not quit")
	}
	if model.confirmQuit {
		t.Fatal("confirmQuit = true, want false after Esc")
	}
}

func TestQuitConfirmationSaveAndQuitPersistsEntries(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	model = modelAny.(*Model)
	if !model.entries.hasUnsavedChanges() {
		t.Fatal("entries.hasUnsavedChanges() = false, want true after insertion")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = modelAny.(*Model)
	if !model.confirmQuit {
		t.Fatal("confirmQuit = false, want true after q with unsaved changes")
	}

	modelAny, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = modelAny.(*Model)
	if cmd == nil {
		t.Fatal("quit cmd is nil for save-and-quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("save-and-quit did not return tea.Quit command")
	}
	if model.entries.hasUnsavedChanges() {
		t.Fatal("entries.hasUnsavedChanges() = true, want false after save-and-quit")
	}

	db, err := worktime.LoadHost(model.entries.dbDir, model.entries.dbHost)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}
	if got := len(db.Entries[model.entries.dbHost]); got != 1 {
		t.Fatalf("saved entries len = %d, want 1", got)
	}
}

func TestViewContainsTabLabels(t *testing.T) {
	model := newRootModelForTest(t)
	view := model.View()
	if view == "" {
		t.Fatal("View() returned empty output")
	}
}

func TestEntriesTabUsesEntriesModelView(t *testing.T) {
	model := newRootModelForTest(t)
	view := model.renderBody()
	if strings.Contains(view, "scaffold") {
		t.Fatalf("renderBody() should not return scaffold text: %q", view)
	}
}

func TestDiscoToggleAndThemeResetKeys(t *testing.T) {
	model := newRootModelForTest(t)

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = modelAny.(*Model)
	if !model.disco {
		t.Fatal("disco = false, want true after x")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	model = modelAny.(*Model)
	if model.theme != DefaultTheme() {
		t.Fatalf("theme after C = %+v, want default theme", model.theme)
	}
}

func TestModelSetsReportWarningForOpenSession(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)

	cfg := config.Default()
	cfg.WorktimeDBDir = tempDir
	cfg.Hostname = "host-a"

	if _, err := worktime.Login(cfg.WorktimeDBDir, cfg.Hostname, "work", localTime(2026, 3, 4, 10, 0), ""); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	model, err := NewModelWithConfig(cfg)
	if err != nil {
		t.Fatalf("NewModelWithConfig() error = %v", err)
	}

	if !strings.Contains(model.report.warn, "currently logged in") {
		t.Fatalf("report warning = %q, want currently logged in warning", model.report.warn)
	}
}

func localTime(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.Local)
}

func newRootModelForTest(t *testing.T) *Model {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)

	cfg := config.Default()
	cfg.WorktimeDBDir = tempDir
	cfg.Hostname = "host-a"

	model, err := NewModelWithConfig(cfg)
	if err != nil {
		t.Fatalf("NewModelWithConfig() error = %v", err)
	}
	return model
}
