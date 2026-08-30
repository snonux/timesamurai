package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newScratchStore returns a directory this test alone owns, and points
// XDG_CONFIG_HOME at another scratch directory with no config.toml in it --
// so config.Load falls back to built-in defaults rather than ever reading
// the real ~/.config/timesamurai/config.toml. Every test in this package
// must route through this (or runWork, which does) instead of touching real
// data or ~/git/worktime.
func newScratchStore(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return t.TempDir()
}

// runWork executes a fresh `timesamurai work` command tree with args, always
// appending "--store storeDir" so the run never touches the real store.
// Returns combined stdout+stderr (cobra's SilenceErrors/SilenceUsage aren't
// set on this subtree, so an error's usage text would otherwise land only in
// the returned error, not the buffer -- tests that need the message text use
// the returned error directly).
func runWork(t *testing.T, storeDir string, args ...string) (string, error) {
	t.Helper()
	return runWorkWithStdin(t, storeDir, "", args...)
}

// runWorkWithStdin behaves like runWork but wires stdin to input, for
// `work delete`'s multi-address confirmation prompt: it reads from
// cmd.InOrStdin(), which defaults to os.Stdin, so tests must inject a fake
// reader to answer (or deliberately not answer) that prompt without a real
// terminal.
func runWorkWithStdin(t *testing.T, storeDir, input string, args ...string) (string, error) {
	t.Helper()
	cmd := NewWorkCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(append(args, "--store", storeDir))
	err := cmd.Execute()
	return out.String(), err
}

// currentHost mirrors resolveHost's fallback path (no config.General.Hostname
// override in these tests, since XDG_CONFIG_HOME points at an empty
// directory) so tests can look up the entries a command just wrote.
func currentHost(t *testing.T) string {
	t.Helper()
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("os.Hostname: %v", err)
	}
	return host
}

// readEntries opens storeDir directly (independently of any runWork call)
// and returns host's entries, for assertions on what a command actually
// wrote to disk.
func readEntries(t *testing.T, storeDir, host string) []worktime.Entry {
	t.Helper()
	store, err := worktime.Open(context.Background(), storeDir)
	if err != nil {
		t.Fatalf("worktime.Open: %v", err)
	}
	return store.Entries(host)
}
