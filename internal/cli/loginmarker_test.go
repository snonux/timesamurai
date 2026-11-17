package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// markerPath is the default marker location under the scratch HOME that
// newScratchStore installs.
func markerPath() string { return filepath.Join(os.Getenv("HOME"), ".wtloggedin") }

func markerExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return err == nil
}

func TestStartCreatesAndStopRemovesLoginMarker(t *testing.T) {
	store := newScratchStore(t)
	if markerExists(t, markerPath()) {
		t.Fatal("marker present before start")
	}
	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !markerExists(t, markerPath()) {
		t.Fatal("start did not create the marker")
	}
	if _, err := runWork(t, store, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if markerExists(t, markerPath()) {
		t.Fatal("stop did not remove the marker")
	}
}

// A marker that drifted from the store heals on the next start/stop even
// though the store refuses the action.
func TestLoginMarkerHealsWhenStateAlreadyAsRequested(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := os.Remove(markerPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := runWork(t, store, "start"); err == nil {
		t.Fatal("second start should be refused")
	}
	if !markerExists(t, markerPath()) {
		t.Fatal("refused start (already logged in) should restore the marker")
	}
	if _, err := runWork(t, store, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	writeFile(t, markerPath(), "")
	out, err := runWork(t, store, "stop")
	if err == nil {
		t.Fatalf("second stop should be refused, got %q", out)
	}
	if markerExists(t, markerPath()) {
		t.Fatal("refused stop (not logged in) should remove the stale marker")
	}
}

func TestLoginLogoutAliasesAndLegacyFlagsManageMarker(t *testing.T) {
	store := newScratchStore(t)
	steps := []struct {
		args []string
		want bool
	}{
		{[]string{"login"}, true},
		{[]string{"logout"}, false},
		{[]string{"--login"}, true},
		{[]string{"--logout"}, false},
	}
	for _, s := range steps {
		if _, err := runWork(t, store, s.args...); err != nil {
			t.Fatalf("work %v: %v", s.args, err)
		}
		if got := markerExists(t, markerPath()); got != s.want {
			t.Fatalf("after work %v: marker exists = %v, want %v", s.args, got, s.want)
		}
	}
}

func TestFailedStartLeavesMarkerAlone(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "start", "--at", "not-a-time"); err == nil {
		t.Fatal("expected invalid --at to fail")
	}
	if markerExists(t, markerPath()) {
		t.Fatal("a failed start created the marker")
	}
}

func TestLoginMarkerSkippedForOtherHost(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "start", "--host", "someotherbox"); err != nil {
		t.Fatalf("start --host: %v", err)
	}
	if markerExists(t, markerPath()) {
		t.Fatal("start for another host created this machine's marker")
	}
	// ...and a stop for another host must not clear this machine's marker.
	if _, err := runWork(t, store, "start", "pet"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := runWork(t, store, "stop", "--host", "someotherbox"); err != nil {
		t.Fatalf("stop --host: %v", err)
	}
	if !markerExists(t, markerPath()) {
		t.Fatal("stop for another host removed this machine's marker")
	}
}

func TestLoginMarkerCanBeRelocatedOrDisabled(t *testing.T) {
	store := newScratchStore(t)
	custom := filepath.Join(t.TempDir(), "custom-marker")
	t.Setenv("TIMESAMURAI_LOGIN_MARKER", custom)
	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !markerExists(t, custom) || markerExists(t, markerPath()) {
		t.Fatal("marker should live only at the configured path")
	}
	if _, err := runWork(t, store, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// An explicit empty login_marker in config.toml disables it.
	t.Setenv("TIMESAMURAI_LOGIN_MARKER", "")
	cfgDir := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(cfgDir, "timesamurai"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfgDir, "timesamurai", "config.toml"), "[general]\nlogin_marker = \"\"\n")
	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if markerExists(t, markerPath()) {
		t.Fatal("disabled marker was still created")
	}
}

func TestLoginMarkerFailureOnlyWarns(t *testing.T) {
	store := newScratchStore(t)
	// Parent directory does not exist, so creating the marker fails.
	t.Setenv("TIMESAMURAI_LOGIN_MARKER", filepath.Join(t.TempDir(), "missing", "marker"))
	out, err := runWork(t, store, "start")
	if err != nil {
		t.Fatalf("a marker failure must not fail the (already persisted) start: %v", err)
	}
	if !strings.Contains(out, "warning: login marker") {
		t.Errorf("expected a marker warning, got %q", out)
	}
}
