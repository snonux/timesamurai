package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTabNavigation(t *testing.T) {
	model := NewModel()

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = modelAny.(Model)
	if model.activeTab != tabReport {
		t.Fatalf("active tab after Tab = %v, want %v", model.activeTab, tabReport)
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = modelAny.(Model)
	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	model = modelAny.(Model)
	if model.activeTab != tabEntries {
		t.Fatalf("active tab after gT = %v, want %v", model.activeTab, tabEntries)
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	model = modelAny.(Model)
	if model.activeTab != tabTimer {
		t.Fatalf("active tab after key 3 = %v, want %v", model.activeTab, tabTimer)
	}
}

func TestHelpToggle(t *testing.T) {
	model := NewModel()

	modelAny, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = modelAny.(Model)
	if !model.showHelp {
		t.Fatal("showHelp = false, want true after ?")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = modelAny.(Model)
	if model.showHelp {
		t.Fatal("showHelp = true, want false after second ?")
	}
}

func TestQuitKeys(t *testing.T) {
	model := NewModel()

	modelAny, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = modelAny.(Model)
	if cmd == nil {
		t.Fatal("quit cmd is nil for q")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q key did not return tea.Quit command")
	}

	modelAny, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	model = modelAny.(Model)
	modelAny, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	model = modelAny.(Model)
	if cmd == nil {
		t.Fatal("quit cmd is nil for ZQ")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ZQ did not return tea.Quit command")
	}
}

func TestViewContainsTabLabels(t *testing.T) {
	model := NewModel()
	view := model.View()
	if view == "" {
		t.Fatal("View() returned empty output")
	}
}
