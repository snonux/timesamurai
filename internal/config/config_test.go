package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func skipIOHeavyInShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping filesystem-heavy config tests in short mode")
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.WeekWorkHours != 40 {
		t.Fatalf("WeekWorkHours = %v, want 40", cfg.WeekWorkHours)
	}

	if cfg.WorktimeDBDir != "~/git/worktime" {
		t.Fatalf("WorktimeDBDir = %q, want %q", cfg.WorktimeDBDir, "~/git/worktime")
	}

	if !reflect.DeepEqual(cfg.PlusFor, []string{"off", "bank", "bufferuse", "sick"}) {
		t.Fatalf("PlusFor = %v", cfg.PlusFor)
	}

	if !reflect.DeepEqual(cfg.WeekendDays, []string{"Sat", "Sun"}) {
		t.Fatalf("WeekendDays = %v", cfg.WeekendDays)
	}

	if !reflect.DeepEqual(cfg.MinusFor, []string{"lunch"}) {
		t.Fatalf("MinusFor = %v", cfg.MinusFor)
	}

	if !reflect.DeepEqual(cfg.BufferFor, []string{
		"tools",
		"pet",
		"selfdevelopment",
		"workrebalance",
		"compensate",
		"travel",
		"rebalance",
	}) {
		t.Fatalf("BufferFor = %v", cfg.BufferFor)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfgPath := filepath.Join(t.TempDir(), "missing.json")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WeekWorkHours != 40 {
		t.Fatalf("WeekWorkHours = %v, want 40", cfg.WeekWorkHours)
	}

	wantDBDir := filepath.Join(tempHome, "git", "worktime")
	if cfg.WorktimeDBDir != wantDBDir {
		t.Fatalf("WorktimeDBDir = %q, want %q", cfg.WorktimeDBDir, wantDBDir)
	}
}

func TestLoadAppliesDefaultsAndExpandsWorktimeDir(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	content := `{
  "weekworkhours": 35,
  "plusfor": ["vacation"],
  "worktime_db_dir": "~/custom/worktime"
}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.WeekWorkHours != 35 {
		t.Fatalf("WeekWorkHours = %v, want 35", cfg.WeekWorkHours)
	}

	if !reflect.DeepEqual(cfg.PlusFor, []string{"vacation"}) {
		t.Fatalf("PlusFor = %v, want [vacation]", cfg.PlusFor)
	}

	if !reflect.DeepEqual(cfg.WeekendDays, []string{"Sat", "Sun"}) {
		t.Fatalf("WeekendDays = %v, want default", cfg.WeekendDays)
	}

	wantDBDir := filepath.Join(tempHome, "custom", "worktime")
	if cfg.WorktimeDBDir != wantDBDir {
		t.Fatalf("WorktimeDBDir = %q, want %q", cfg.WorktimeDBDir, wantDBDir)
	}
}

func TestLoadPreservesExplicitEmptyLists(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	content := `{
  "plusfor": [],
  "weekendays": [],
  "minusfor": [],
  "bufferfor": []
}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.PlusFor) != 0 || len(cfg.WeekendDays) != 0 || len(cfg.MinusFor) != 0 || len(cfg.BufferFor) != 0 {
		t.Fatalf("explicit empty lists were not preserved: %+v", cfg)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	skipIOHeavyInShort(t)
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"weekworkhours":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}

	if !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("Load() error = %v, want parse config context", err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfgPath := filepath.Join(t.TempDir(), "config.json")
	in := Config{
		WeekWorkHours:     37.5,
		PlusFor:           []string{"off"},
		WeekendDays:       []string{"Fri"},
		MinusFor:          []string{"lunch", "coffee"},
		BufferFor:         []string{"pet"},
		WorktimeDBDir:     "~/my-worktime",
		Hostname:          "host-from-config",
		AutoWorktimeLogin: true,
	}

	if err := Save(cfgPath, in); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if out.WeekWorkHours != in.WeekWorkHours {
		t.Fatalf("WeekWorkHours = %v, want %v", out.WeekWorkHours, in.WeekWorkHours)
	}

	if !reflect.DeepEqual(out.PlusFor, in.PlusFor) {
		t.Fatalf("PlusFor = %v, want %v", out.PlusFor, in.PlusFor)
	}

	if !reflect.DeepEqual(out.WeekendDays, in.WeekendDays) {
		t.Fatalf("WeekendDays = %v, want %v", out.WeekendDays, in.WeekendDays)
	}

	wantDBDir := filepath.Join(tempHome, "my-worktime")
	if out.WorktimeDBDir != wantDBDir {
		t.Fatalf("WorktimeDBDir = %q, want %q", out.WorktimeDBDir, wantDBDir)
	}

	if out.Hostname != in.Hostname {
		t.Fatalf("Hostname = %q, want %q", out.Hostname, in.Hostname)
	}

	if !out.AutoWorktimeLogin {
		t.Fatal("AutoWorktimeLogin = false, want true")
	}
}

func TestSaveAndLoadUsingDefaultPath(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	cfg := Default()
	cfg.Hostname = "example-host"

	if err := Save("", cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configPath, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config not found at %q: %v", configPath, err)
	}

	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Hostname != "example-host" {
		t.Fatalf("Hostname = %q, want %q", loaded.Hostname, "example-host")
	}
}

func TestEffectiveHostnamePrefersConfigValue(t *testing.T) {
	cfg := Default()
	cfg.Hostname = " configured-host "

	got, err := cfg.EffectiveHostname()
	if err != nil {
		t.Fatalf("EffectiveHostname() error = %v", err)
	}

	if got != "configured-host" {
		t.Fatalf("EffectiveHostname() = %q, want %q", got, "configured-host")
	}
}

func TestEffectiveHostnameUsesOverrideFile(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	overridePath := filepath.Join(tempHome, ".hostnameoverride")
	if err := os.WriteFile(overridePath, []byte("override-host\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg := Default()
	got, err := cfg.EffectiveHostname()
	if err != nil {
		t.Fatalf("EffectiveHostname() error = %v", err)
	}

	if got != "override-host" {
		t.Fatalf("EffectiveHostname() = %q, want %q", got, "override-host")
	}
}

func TestEffectiveHostnameFallsBackToOSHostname(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	want, err := os.Hostname()
	if err != nil {
		t.Fatalf("Hostname() error = %v", err)
	}

	cfg := Default()
	got, err := cfg.EffectiveHostname()
	if err != nil {
		t.Fatalf("EffectiveHostname() error = %v", err)
	}

	if got != want {
		t.Fatalf("EffectiveHostname() = %q, want %q", got, want)
	}
}

func TestEffectiveHostnameIgnoresEmptyOverride(t *testing.T) {
	skipIOHeavyInShort(t)
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	overridePath := filepath.Join(tempHome, ".hostnameoverride")
	if err := os.WriteFile(overridePath, []byte("\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	want, err := os.Hostname()
	if err != nil {
		t.Fatalf("Hostname() error = %v", err)
	}

	cfg := Default()
	got, err := cfg.EffectiveHostname()
	if err != nil {
		t.Fatalf("EffectiveHostname() error = %v", err)
	}

	if got != want {
		t.Fatalf("EffectiveHostname() = %q, want %q", got, want)
	}
}
