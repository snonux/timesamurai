package viinput

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModelNormalModeMotionDispatch(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.SetValue("alpha beta")
	model.mode = ModeNormal
	model.cursor = 0

	model, _ = model.Update(key("w"))
	if got := model.cursor; got != 6 {
		t.Fatalf("w cursor = %d, want 6", got)
	}

	model, _ = model.Update(key("b"))
	if got := model.cursor; got != 0 {
		t.Fatalf("b cursor = %d, want 0", got)
	}

	model, _ = model.Update(key("e"))
	if got := model.cursor; got != 4 {
		t.Fatalf("e cursor = %d, want 4", got)
	}

	model, _ = model.Update(key("0"))
	if got := model.cursor; got != 0 {
		t.Fatalf("0 cursor = %d, want 0", got)
	}

	model, _ = model.Update(key("$"))
	if got := model.cursor; got != len(model.runes) {
		t.Fatalf("$ cursor = %d, want %d", got, len(model.runes))
	}
}

func TestModelHelixPendingG(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.SetValue("alpha beta")
	model.mode = ModeNormal
	model.cursor = 5

	model, _ = model.Update(key("g"))
	if got := model.pending; got != 'g' {
		t.Fatalf("pending = %q, want 'g'", got)
	}

	model, _ = model.Update(key("h"))
	if got := model.cursor; got != 0 {
		t.Fatalf("gh cursor = %d, want 0", got)
	}

	model.cursor = 3
	model, _ = model.Update(key("g"))
	model, _ = model.Update(key("l"))
	if got := model.cursor; got != len(model.runes) {
		t.Fatalf("gl cursor = %d, want %d", got, len(model.runes))
	}
}

func TestModelModeTransitions(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.SetValue("abc")

	model, _ = model.Update(special(tea.KeyEscape))
	if got := model.Mode(); got != ModeNormal {
		t.Fatalf("mode = %v, want ModeNormal", got)
	}
	if model.WantsExit() {
		t.Fatal("esc in insert mode should not request exit")
	}

	model, _ = model.Update(key("i"))
	if got := model.Mode(); got != ModeInsert {
		t.Fatalf("mode = %v, want ModeInsert", got)
	}

	model, _ = model.Update(special(tea.KeyEscape))
	model, _ = model.Update(special(tea.KeyEscape))
	if !model.WantsExit() {
		t.Fatal("esc in normal mode should request exit")
	}
}

func TestModelInsertModeEditing(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.SetValue("abc")

	model.cursor = 2
	model, _ = model.Update(special(tea.KeyBackspace))
	if got := model.Value(); got != "ac" {
		t.Fatalf("value = %q, want %q", got, "ac")
	}

	model, _ = model.Update(special(tea.KeyEnd))
	model, _ = model.Update(key("d"))
	if got := model.Value(); got != "acd" {
		t.Fatalf("value = %q, want %q", got, "acd")
	}

	other := New()
	other.Focus()
	other.SetValue("abc")
	other.cursor = 1
	other, _ = other.Update(key("h"))
	if got := other.Value(); got != "ahbc" {
		t.Fatalf("insert-mode h value = %q, want %q", got, "ahbc")
	}
}

func key(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 0, Text: value}
}

func special(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}
