package viinput

import (
	"strings"
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

func TestModelNormalModeDeletes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      string
		cursor     int
		steps      []string
		wantValue  string
		wantCursor int
		wantMode   Mode
	}{
		{
			name:       "x deletes character at cursor",
			value:      "alpha beta",
			cursor:     0,
			steps:      []string{"x"},
			wantValue:  "lpha beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "X deletes character before cursor",
			value:      "alpha beta",
			cursor:     1,
			steps:      []string{"X"},
			wantValue:  "lpha beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "D deletes to line end",
			value:      "alpha beta",
			cursor:     6,
			steps:      []string{"D"},
			wantValue:  "alpha ",
			wantCursor: 6,
			wantMode:   ModeNormal,
		},
		{
			name:       "dd clears line",
			value:      "alpha beta",
			cursor:     4,
			steps:      []string{"d", "d"},
			wantValue:  "",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "dw deletes forward by word",
			value:      "alpha beta",
			cursor:     0,
			steps:      []string{"d", "w"},
			wantValue:  "beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "de deletes to word end",
			value:      "alpha beta",
			cursor:     0,
			steps:      []string{"d", "e"},
			wantValue:  " beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "db deletes backward by word",
			value:      "alpha beta",
			cursor:     6,
			steps:      []string{"d", "b"},
			wantValue:  "beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "d0 deletes from line start",
			value:      "alpha beta",
			cursor:     6,
			steps:      []string{"d", "0"},
			wantValue:  "beta",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "d$ deletes from cursor to line end",
			value:      "alpha beta",
			cursor:     0,
			steps:      []string{"d", "$"},
			wantValue:  "",
			wantCursor: 0,
			wantMode:   ModeNormal,
		},
		{
			name:       "C deletes to line end and enters insert mode",
			value:      "alpha beta",
			cursor:     6,
			steps:      []string{"C"},
			wantValue:  "alpha ",
			wantCursor: 6,
			wantMode:   ModeInsert,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := New()
			model.Focus()
			model.SetValue(tt.value)
			model.mode = ModeNormal
			model.cursor = tt.cursor

			for _, step := range tt.steps {
				model, _ = model.Update(key(step))
			}

			if got := model.Value(); got != tt.wantValue {
				t.Fatalf("value = %q, want %q", got, tt.wantValue)
			}
			if got := model.cursor; got != tt.wantCursor {
				t.Fatalf("cursor = %d, want %d", got, tt.wantCursor)
			}
			if got := model.Mode(); got != tt.wantMode {
				t.Fatalf("mode = %v, want %v", got, tt.wantMode)
			}
		})
	}
}

func TestModelUndoRestoresPriorState(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.SetValue("alpha beta")
	model.mode = ModeNormal
	model.cursor = 0

	model, _ = model.Update(key("x"))
	model, _ = model.Update(key("u"))
	if got := model.Value(); got != "alpha beta" {
		t.Fatalf("undo after x value = %q, want %q", got, "alpha beta")
	}
	if got := model.cursor; got != 0 {
		t.Fatalf("undo after x cursor = %d, want 0", got)
	}

	model, _ = model.Update(key("d"))
	model, _ = model.Update(key("w"))
	if got := model.Value(); got != "beta" {
		t.Fatalf("dw value = %q, want %q", got, "beta")
	}

	model, _ = model.Update(key("u"))
	if got := model.Value(); got != "alpha beta" {
		t.Fatalf("undo after dw value = %q, want %q", got, "alpha beta")
	}
	if got := model.cursor; got != 0 {
		t.Fatalf("undo after dw cursor = %d, want 0", got)
	}
}

func TestModelViewUsesModeSpecificCursorStyles(t *testing.T) {
	t.Parallel()

	model := New()
	model.Focus()
	model.Prompt = "edit> "
	model.SetValue("abc")
	model.cursor = 1
	model.mode = ModeNormal

	got := model.View()
	if !strings.HasPrefix(got, model.Prompt) {
		t.Fatalf("view = %q, want prefix %q", got, model.Prompt)
	}
	if !strings.Contains(got, "a") || !strings.Contains(got, "bc") {
		t.Fatalf("view = %q, want rendered contents", got)
	}
	if !strings.Contains(got, cursorGlyph(ModeNormal)) {
		t.Fatalf("normal view = %q, want block cursor", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("normal view = %q, want lipgloss styling", got)
	}

	model.mode = ModeInsert
	got = model.View()
	if !strings.Contains(got, cursorGlyph(ModeInsert)) {
		t.Fatalf("insert view = %q, want bar cursor", got)
	}
}

func key(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 0, Text: value}
}

func special(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}
