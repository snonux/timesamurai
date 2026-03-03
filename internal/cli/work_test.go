package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	timrTimer "codeberg.org/snonux/timr/internal/timer"
)

func TestWorkLoginStatusLogoutFlow(t *testing.T) {
	dbDir := t.TempDir()
	cfgPath := writeWorkConfig(t, dbDir, "host-a")

	out, err := runRootCommand("--config", cfgPath, "work", "login", "--at", "2026-01-05T09:00")
	if err != nil {
		t.Fatalf("work login error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged in:") {
		t.Fatalf("unexpected login output: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "status")
	if err != nil {
		t.Fatalf("work status error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged in: work") {
		t.Fatalf("unexpected status output after login: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "logout", "--at", "2026-01-05T18:00")
	if err != nil {
		t.Fatalf("work logout error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Logged out:") {
		t.Fatalf("unexpected logout output: %q", out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "status")
	if err != nil {
		t.Fatalf("work status error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "Not logged in.") {
		t.Fatalf("unexpected status output after logout: %q", out)
	}
}

func TestWorkAddSubUseBufferAndReport(t *testing.T) {
	dbDir := t.TempDir()
	cfgPath := writeWorkConfig(t, dbDir, "host-b")

	out, err := runRootCommand("--config", cfgPath, "work", "add", "1h", "--at", "2026-01-06T10:00")
	if err != nil {
		t.Fatalf("work add error = %v (output: %q)", err, out)
	}
	out, err = runRootCommand("--config", cfgPath, "work", "sub", "30m", "--at", "2026-01-06T11:00")
	if err != nil {
		t.Fatalf("work sub error = %v (output: %q)", err, out)
	}
	out, err = runRootCommand("--config", cfgPath, "work", "use-buffer", "15m", "--at", "2026-01-06T12:00")
	if err != nil {
		t.Fatalf("work use-buffer error = %v (output: %q)", err, out)
	}

	out, err = runRootCommand("--config", cfgPath, "work", "report", "--no-color")
	if err != nil {
		t.Fatalf("work report error = %v (output: %q)", err, out)
	}
	if !strings.Contains(out, "work:") {
		t.Fatalf("work report missing work field: %q", out)
	}
	if !strings.Contains(out, "buffer:") {
		t.Fatalf("work report missing buffer field: %q", out)
	}
}

func TestWorkLoginLogoutWithTimerFlags(t *testing.T) {
	setupTimerState(t)

	dbDir := t.TempDir()
	cfgPath := writeWorkConfig(t, dbDir, "host-c")

	out, err := runRootCommand("--config", cfgPath, "work", "login", "--at", "2026-01-08T09:00", "--start-timer")
	if err != nil {
		t.Fatalf("work login --start-timer error = %v (output: %q)", err, out)
	}
	state, err := timrTimer.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if !state.Running {
		t.Fatal("timer should be running after --start-timer")
	}

	out, err = runRootCommand("--config", cfgPath, "work", "logout", "--at", "2026-01-08T10:00", "--stop-timer")
	if err != nil {
		t.Fatalf("work logout --stop-timer error = %v (output: %q)", err, out)
	}
	state, err = timrTimer.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Running {
		t.Fatal("timer should be stopped after --stop-timer")
	}
}

func TestParseImportDateFallbackLayout(t *testing.T) {
	parsed, err := parseImportDate("06.01.2026")
	if err != nil {
		t.Fatalf("parseImportDate() error = %v", err)
	}

	expected := time.Date(2026, 1, 6, 0, 0, 0, 0, time.Local)
	if parsed.Year() != expected.Year() || parsed.Month() != expected.Month() || parsed.Day() != expected.Day() {
		t.Fatalf("parsed date = %v, want %v", parsed, expected)
	}
}

func TestParseImportDateIncludesUnderlyingParseError(t *testing.T) {
	_, err := parseImportDate("not-a-date")
	if err == nil {
		t.Fatal("parseImportDate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported import date") {
		t.Fatalf("parseImportDate() error = %v, want unsupported import date context", err)
	}
}

func writeWorkConfig(t *testing.T, dbDir, host string) string {
	return writeWorkConfigWithAuto(t, dbDir, host, false)
}

func writeWorkConfigWithAuto(t *testing.T, dbDir, host string, auto bool) string {
	t.Helper()

	content := `{
  "worktime_db_dir": "` + dbDir + `",
  "hostname": "` + host + `",
  "auto_worktime_login": ` + strconv.FormatBool(auto) + `
}
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func runRootCommand(args ...string) (string, error) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
