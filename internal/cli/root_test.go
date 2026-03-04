package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	timesamurai "codeberg.org/snonux/timesamurai/internal"
	"codeberg.org/snonux/timesamurai/internal/worktime"
)

func TestRootVersionFlag(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.TrimSpace(out.String()) != timesamurai.Version {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(out.String()), timesamurai.Version)
	}
}

func TestRootLoadsConfigInPersistentPreRun(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	content := `{
  "hostname": "from-config",
  "worktime_db_dir": "~/custom-db"
}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", cfgPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cfg := currentConfig(cmd)
	if cfg.Hostname != "from-config" {
		t.Fatalf("Hostname = %q, want %q", cfg.Hostname, "from-config")
	}

	wantDir := filepath.Join(tempHome, "custom-db")
	if cfg.WorktimeDBDir != wantDir {
		t.Fatalf("WorktimeDBDir = %q, want %q", cfg.WorktimeDBDir, wantDir)
	}
}

func TestRootInvalidConfigFileReturnsError(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"hostname":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", cfgPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want config parse error")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("Execute() error = %v, want load config context", err)
	}
}

func TestVersionSkipsConfigLoading(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"hostname":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--config", cfgPath, "--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.TrimSpace(out.String()) != timesamurai.Version {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(out.String()), timesamurai.Version)
	}
}

func TestRootUsesDefaultConfigWhenNoFileExists(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	cmd := NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	cfg := currentConfig(cmd)
	if cfg.WeekWorkHours != 40 {
		t.Fatalf("WeekWorkHours = %v, want 40", cfg.WeekWorkHours)
	}

	wantDir := filepath.Join(tempHome, "git", "worktime")
	if cfg.WorktimeDBDir != wantDir {
		t.Fatalf("WorktimeDBDir = %q, want %q", cfg.WorktimeDBDir, wantDir)
	}
}

func TestRootCheckDBIntegrityPasses(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	if _, err := worktime.Login(dbDir, host, "work", time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, err := worktime.Logout(dbDir, host, "work", time.Unix(200, 0), ""); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	cfgPath := writeRootConfig(t, dbDir, host)

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "--check-db-integrity"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v (output: %q)", err, out.String())
	}
	if !strings.Contains(out.String(), "Database integrity check passed.") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRootCheckDBIntegrityFailsOnIssues(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	db := worktime.Database{
		Entries: map[string][]worktime.Entry{
			host: {
				{
					Action: "logout",
					What:   "work",
					Epoch:  100,
					Source: host,
					Human:  time.Unix(100, 0).Format("Mon 02.01.2006 15:04:05"),
				},
			},
		},
	}
	if err := worktime.SaveHost(dbDir, host, db); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	cfgPath := writeRootConfig(t, dbDir, host)

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "--check-db-integrity"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want integrity failure")
	}
	if !strings.Contains(err.Error(), "database integrity check failed") {
		t.Fatalf("Execute() error = %v, want integrity failure", err)
	}
	if !strings.Contains(out.String(), "Database integrity check found") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRootCheckDBIntegrityWarnsForOpenSession(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	if _, err := worktime.Login(dbDir, host, "work", time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	cfgPath := writeRootConfig(t, dbDir, host)

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--config", cfgPath, "--check-db-integrity"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v (output: %q)", err, out.String())
	}
	if !strings.Contains(out.String(), "Database integrity check passed.") {
		t.Fatalf("unexpected output: %q", out.String())
	}
	if !strings.Contains(out.String(), "Warning: currently logged in") {
		t.Fatalf("expected open-session warning in output: %q", out.String())
	}
}

func writeRootConfig(t *testing.T, dbDir, host string) string {
	t.Helper()

	content := `{
  "worktime_db_dir": "` + dbDir + `",
  "hostname": "` + host + `"
}
`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}
