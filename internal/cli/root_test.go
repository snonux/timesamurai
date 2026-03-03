package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	timr "codeberg.org/snonux/timr/internal"
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

	if strings.TrimSpace(out.String()) != timr.Version {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(out.String()), timr.Version)
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

	if strings.TrimSpace(out.String()) != timr.Version {
		t.Fatalf("output = %q, want %q", strings.TrimSpace(out.String()), timr.Version)
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
