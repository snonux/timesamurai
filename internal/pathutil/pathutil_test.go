package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestExpandHome covers the cases both former call sites (internal/config's
// path fields and internal/cli's --db/--store flags) relied on before this
// helper was consolidated here: empty input, an already-absolute or relative
// path passed through unchanged, bare "~", and "~/..." joined onto the home
// directory.
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}

	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"/abs/path", "/abs/path"},
		{"relative/dir", "relative/dir"},
		{"~", home},
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
	}
	for _, tt := range tests {
		got, err := ExpandHome(tt.in)
		if err != nil {
			t.Fatalf("ExpandHome(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("ExpandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
