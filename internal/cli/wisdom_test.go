package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/snonux/timesamurai/internal/ansi"
)

func TestPrintWisdomPicksABulletLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), wisdomsFileName)
	writeFile(t, path, "# Wisdoms\n\n* first one\nprose line\n* second one \n*not a bullet\n* \n")
	var out bytes.Buffer
	var gotN int
	printWisdom(&out, ansi.Theme{}, path, func(n int) int { gotN = n; return 1 })
	if gotN != 2 {
		t.Fatalf("picked among %d wisdoms, want 2", gotN)
	}
	if got := out.String(); got != "second one\n" {
		t.Fatalf("got %q, want %q", got, "second one\n")
	}
}

func TestPrintWisdomSilentWithoutFileOrBullets(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	pick := func(int) int { t.Fatal("pick called"); return 0 }
	printWisdom(&out, ansi.Theme{}, filepath.Join(dir, "missing.md"), pick)
	empty := filepath.Join(dir, wisdomsFileName)
	writeFile(t, empty, "just prose\n")
	printWisdom(&out, ansi.Theme{}, empty, pick)
	if out.Len() != 0 {
		t.Fatalf("expected no output, got %q", out.String())
	}
}

func TestPrintWisdomWarnsOnUnreadableFile(t *testing.T) {
	// A directory in place of the file: exists, but cannot be read as one.
	path := filepath.Join(t.TempDir(), wisdomsFileName)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printWisdom(&out, ansi.Theme{}, path, func(int) int { return 0 })
	if !bytes.Contains(out.Bytes(), []byte("warning: wisdoms")) {
		t.Fatalf("expected a warning, got %q", out.String())
	}
}

// The post-run hook must stay quiet when stderr is not a terminal (as in
// every test here, see TestMain), even with a wisdoms file in place.
func TestWisdomNotPrintedToNonTerminal(t *testing.T) {
	store := newScratchStore(t)
	db := t.TempDir()
	writeFile(t, filepath.Join(db, wisdomsFileName), "* stay focused\n")
	out, err := runWork(t, store, "status", "--db", db)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if bytes.Contains([]byte(out), []byte("stay focused")) {
		t.Fatalf("wisdom leaked into non-terminal output: %q", out)
	}
}
