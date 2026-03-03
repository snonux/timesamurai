package cli

import "testing"

func TestRootContainsTUISubcommand(t *testing.T) {
	cmd := NewRootCmd()

	found, _, err := cmd.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("Find(tui) error = %v", err)
	}
	if found == nil {
		t.Fatal("Find(tui) returned nil command")
	}
	if found.Use != "tui" {
		t.Fatalf("found.Use = %q, want %q", found.Use, "tui")
	}
}

func TestNewTUICmdMetadata(t *testing.T) {
	cmd := newTUICmd()
	if cmd.Use != "tui" {
		t.Fatalf("Use = %q, want %q", cmd.Use, "tui")
	}
	if cmd.Short == "" {
		t.Fatal("Short description should not be empty")
	}
}
