package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExpandHomeExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}

	cases := map[string]string{
		"~":            home,
		"~/foo/bar":    filepath.Join(home, "foo", "bar"),
		"/absolute":    "/absolute",
		"relative/dir": "relative/dir",
	}
	for in, want := range cases {
		got, err := expandHome(in)
		if err != nil {
			t.Fatalf("expandHome(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLoadConfigWithOverridesAppliesDbAndStoreFlags is a white-box test
// (same package) of the --db/--store precedence loadConfigWithOverrides
// implements: both flags must win over config.toml/env/defaults, and both
// go through the same tilde expansion --store's callers rely on.
func TestLoadConfigWithOverridesAppliesDbAndStoreFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dbDir := t.TempDir()
	storeDir := t.TempDir()

	// loadConfigWithOverrides reads cmd.Flags() directly. In production that
	// command is the one cobra.Execute() just parsed, which has already
	// merged the "work" parent's PersistentFlags into it -- a synthetic
	// command built by hand (not run through Execute) has to add the flags
	// to Flags() itself to reproduce that same view.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("db", "", "")
	cmd.Flags().String("store", "", "")
	if err := cmd.Flags().Set("db", dbDir); err != nil {
		t.Fatalf("set --db: %v", err)
	}
	if err := cmd.Flags().Set("store", storeDir); err != nil {
		t.Fatalf("set --store: %v", err)
	}

	cfg, err := loadConfigWithOverrides(context.Background(), cmd)
	if err != nil {
		t.Fatalf("loadConfigWithOverrides: %v", err)
	}
	if cfg.Storage.DBDir != dbDir {
		t.Errorf("Storage.DBDir = %q, want %q", cfg.Storage.DBDir, dbDir)
	}
	if cfg.Storage.StoreDir != storeDir {
		t.Errorf("Storage.StoreDir = %q, want %q", cfg.Storage.StoreDir, storeDir)
	}
}

func TestLoadConfigWithOverridesLeavesDefaultsWhenFlagsUnset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("db", "", "")
	cmd.Flags().String("store", "", "")

	cfg, err := loadConfigWithOverrides(context.Background(), cmd)
	if err != nil {
		t.Fatalf("loadConfigWithOverrides: %v", err)
	}
	if !strings.Contains(cfg.Storage.DBDir, "worktime") {
		t.Errorf("unset --db should keep the built-in default, got %q", cfg.Storage.DBDir)
	}
}

func TestParseAtFlagEmptyMeansZeroTime(t *testing.T) {
	got, err := parseAtFlag("")
	if err != nil {
		t.Fatalf("parseAtFlag(\"\"): %v", err)
	}
	if !got.IsZero() {
		t.Errorf("parseAtFlag(\"\") = %v, want zero time", got)
	}
}

func TestParseAtFlagInvalidReturnsWrappedError(t *testing.T) {
	_, err := parseAtFlag("not-a-time")
	if err == nil {
		t.Fatal("parseAtFlag(\"not-a-time\"): want error, got nil")
	}
	if !strings.Contains(err.Error(), "--at") {
		t.Errorf("error should name the flag, got %q", err.Error())
	}
}

func TestStartWithInvalidAtFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "start", "--at", "not-a-time"); err == nil {
		t.Fatal("start --at not-a-time: want error, got nil")
	}
}

func TestVerboseShorthandFlag(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "start", "-v")
	if err != nil {
		t.Fatalf("work start -v: %v", err)
	}
	if !strings.Contains(out, "action=login") {
		t.Errorf("-v output should include full entry detail, got %q", out)
	}
}

func TestDbFlagAcceptedAndDoesNotBreakMutations(t *testing.T) {
	store := newScratchStore(t)
	dbDir := t.TempDir()

	// --db only affects the legacy worktime.rb JSON directory, which no
	// mutation reads today (that lands with the export/import task), but the
	// flag must still be accepted end-to-end without error.
	if _, err := runWork(t, store, "start", "--db", dbDir); err != nil {
		t.Fatalf("work start --db: %v", err)
	}
}
