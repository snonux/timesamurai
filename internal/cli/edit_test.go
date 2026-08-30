package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEditCommandStubEditorRoundTrip runs the full `work edit` command with
// $EDITOR stubbed to a tiny shell script that rewrites one entry's
// description in place, confirming the end-to-end wiring (render -> launch
// $EDITOR -> parse -> diff -> apply) works without a real interactive
// editor. The heavier diff/apply logic itself is covered in isolation by
// edit_format_test.go/edit_apply_test.go; this only exercises the plumbing
// between edit.go, edit_format.go, edit_apply.go, and edit_editor.go. The
// stub script assumes a POSIX shell, matching this project's Linux-only
// environment (see ~/.claude/CLAUDE.md's Fedora Linux dev machine).
func TestEditCommandStubEditorRoundTrip(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "before edit"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	script := filepath.Join(t.TempDir(), "fake-editor.sh")
	// The stub "editor" just replaces "before edit" with "after edit" in
	// place, standing in for a human doing the same thing interactively.
	content := "#!/bin/sh\nsed -i 's/before edit/after edit/' \"$1\"\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write stub editor: %v", err)
	}
	t.Setenv("EDITOR", script)

	out, err := runWork(t, store, "edit")
	if err != nil {
		t.Fatalf("work edit: %v", err)
	}
	if !strings.Contains(out, "modify") {
		t.Errorf("expected a modify to be reported, got %q", out)
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 1 || entries[0].Descr != "after edit" {
		t.Fatalf("want descr \"after edit\", got %+v", entries)
	}
}

// TestEditCommandNoEditorFails confirms a missing $EDITOR fails clearly
// instead of silently doing nothing or guessing a default editor.
func TestEditCommandNoEditorFails(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	t.Setenv("EDITOR", "")

	if _, err := runWork(t, store, "edit"); err == nil {
		t.Fatal("work edit with no $EDITOR: want error, got nil")
	}
}
