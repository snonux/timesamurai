package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func clearTimesamuraiEnv(t *testing.T) {
	t.Helper()
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "TIMESAMURAI_") {
			kv := strings.SplitN(e, "=", 2)
			t.Setenv(kv[0], "")
		}
	}
}

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "timesamurai", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Accounting.WeekWorkHours != 40 {
		t.Fatalf("WeekWorkHours: got %v want 40", cfg.Accounting.WeekWorkHours)
	}
	wantPlus := []string{"off", "bank", "bufferuse", "sick"}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, wantPlus) {
		t.Fatalf("PlusFor: got %v want %v", cfg.Accounting.PlusFor, wantPlus)
	}
	if cfg.Storage.DBDir != "~/git/worktime" {
		t.Fatalf("DBDir: got %q", cfg.Storage.DBDir)
	}
	if cfg.Storage.StoreDir != "~/git/worktime/timesamuraidb" {
		t.Fatalf("StoreDir: got %q", cfg.Storage.StoreDir)
	}
	if !cfg.Report.Color || cfg.Report.Verbose || cfg.General.AutoWorktimeLogin {
		t.Fatalf("unexpected report/general defaults: %+v %+v", cfg.Report, cfg.General)
	}
}

func TestConfigPath_XDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "timesamurai", "config.toml")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := Load(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantStore := filepath.Join(home, "git/worktime/timesamuraidb")
	wantDB := filepath.Join(home, "git/worktime")
	if cfg.Storage.StoreDir != wantStore || cfg.Storage.DBDir != wantDB {
		t.Fatalf("expanded paths: store=%q db=%q", cfg.Storage.StoreDir, cfg.Storage.DBDir)
	}
	if cfg.Accounting.WeekWorkHours != 40 {
		t.Fatalf("expected default hours, got %v", cfg.Accounting.WeekWorkHours)
	}
}

func TestLoad_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Load(ctx, LoadOptions{})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestLoad_FileMerge(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, `
[storage]
store_dir = "/tmp/store"
db_dir = "/tmp/db"

[accounting]
week_work_hours = 35
plus_for = ["off", "sick"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]

[general]
hostname = "earth"
auto_worktime_login = true

[report]
color = false
verbose = true
`)

	cfg, err := Load(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.StoreDir != "/tmp/store" || cfg.Storage.DBDir != "/tmp/db" {
		t.Fatalf("storage: %+v", cfg.Storage)
	}
	if cfg.Accounting.WeekWorkHours != 35 {
		t.Fatalf("hours: %v", cfg.Accounting.WeekWorkHours)
	}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, []string{"off", "sick"}) {
		t.Fatalf("plus_for: %v", cfg.Accounting.PlusFor)
	}
	if cfg.General.Hostname != "earth" || !cfg.General.AutoWorktimeLogin {
		t.Fatalf("general: %+v", cfg.General)
	}
	if cfg.Report.Color || !cfg.Report.Verbose {
		t.Fatalf("report: %+v", cfg.Report)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, `
[storage]
store_dir = "/tmp/from-file"
db_dir = "/tmp/db-file"

[accounting]
week_work_hours = 35
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat", "Sun"]

[report]
color = true
verbose = false
`)
	t.Setenv("TIMESAMURAI_STORE_DIR", "/tmp/from-env")
	t.Setenv("TIMESAMURAI_WEEK_WORK_HOURS", "42")
	t.Setenv("TIMESAMURAI_COLOR", "false")
	t.Setenv("TIMESAMURAI_VERBOSE", "1")
	t.Setenv("TIMESAMURAI_HOSTNAME", "env-host")

	cfg, err := Load(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.StoreDir != "/tmp/from-env" {
		t.Fatalf("store_dir env should win: got %q", cfg.Storage.StoreDir)
	}
	if cfg.Storage.DBDir != "/tmp/db-file" {
		t.Fatalf("db_dir should stay from file: got %q", cfg.Storage.DBDir)
	}
	if cfg.Accounting.WeekWorkHours != 42 {
		t.Fatalf("hours env should win: got %v", cfg.Accounting.WeekWorkHours)
	}
	if cfg.Report.Color {
		t.Fatal("TIMESAMURAI_COLOR=false should win")
	}
	if !cfg.Report.Verbose {
		t.Fatal("TIMESAMURAI_VERBOSE=1 should win")
	}
	if cfg.General.Hostname != "env-host" {
		t.Fatalf("hostname: %q", cfg.General.Hostname)
	}
}

func TestLoad_IgnoreEnv(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := writeConfig(t, dir, `
[storage]
store_dir = "/tmp/file-store"
db_dir = "/tmp/file-db"

[accounting]
week_work_hours = 30
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sun"]
`)
	t.Setenv("TIMESAMURAI_STORE_DIR", "/tmp/env-store")
	t.Setenv("TIMESAMURAI_WEEK_WORK_HOURS", "99")

	cfg, err := Load(context.Background(), LoadOptions{ConfigPath: path, IgnoreEnv: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.StoreDir != "/tmp/file-store" {
		t.Fatalf("IgnoreEnv: got store %q", cfg.Storage.StoreDir)
	}
	if cfg.Accounting.WeekWorkHours != 30 {
		t.Fatalf("IgnoreEnv: got hours %v", cfg.Accounting.WeekWorkHours)
	}
}

func TestLoad_StoreDirDefaultsToDBDir(t *testing.T) {
	cfg := Default()
	cfg.Storage.StoreDir = ""
	cfg.Storage.DBDir = "/tmp/only-db"
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.StoreDir != "/tmp/only-db" {
		t.Fatalf("store_dir should fall back to db_dir, got %q", cfg.Storage.StoreDir)
	}
}

func TestLoad_OnlyDBDirInFileFallsBackStore(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	path := writeConfig(t, dir, `
[storage]
db_dir = "/tmp/only-db"

[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`)
	cfg, err := Load(context.Background(), LoadOptions{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.DBDir != "/tmp/only-db" {
		t.Fatalf("db_dir: %q", cfg.Storage.DBDir)
	}
	if cfg.Storage.StoreDir != "/tmp/only-db" {
		t.Fatalf("store_dir should fall back to db_dir, got %q", cfg.Storage.StoreDir)
	}
}

func TestLoad_WeekWorkHoursZeroRejected(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	path := writeConfig(t, dir, `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 0
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`)
	_, err := Load(context.Background(), LoadOptions{ConfigPath: path})
	if err == nil || !strings.Contains(err.Error(), "week_work_hours must be > 0") {
		t.Fatalf("got %v, want week_work_hours error", err)
	}
}

func TestLoad_BadEnvFails(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	path := writeConfig(t, dir, `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`)
	t.Setenv("TIMESAMURAI_WEEK_WORK_HOURS", "abc")
	_, err := Load(context.Background(), LoadOptions{ConfigPath: path})
	if err == nil || !strings.Contains(err.Error(), "TIMESAMURAI_WEEK_WORK_HOURS") {
		t.Fatalf("got %v, want env float error", err)
	}

	t.Setenv("TIMESAMURAI_WEEK_WORK_HOURS", "NaN")
	_, err = Load(context.Background(), LoadOptions{ConfigPath: path})
	if err == nil || !strings.Contains(err.Error(), "non-finite") {
		t.Fatalf("got %v, want non-finite error", err)
	}
}

func TestLoad_BoolEnvCaseInsensitive(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	path := writeConfig(t, dir, `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`)
	t.Setenv("TIMESAMURAI_COLOR", "TRUE")
	t.Setenv("TIMESAMURAI_VERBOSE", "NO")
	cfg, err := Load(context.Background(), LoadOptions{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Report.Color {
		t.Fatal("TRUE should enable color")
	}
	if cfg.Report.Verbose {
		t.Fatal("NO should disable verbose")
	}
}

func TestLoad_NegativeCases(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
		env     map[string]string
		wantSub string
	}{
		{
			name:    "invalid TOML",
			content: `[storage\nstore_dir = `,
			wantSub: "parse config",
		},
		{
			name: "flat top-level key",
			content: `
week_work_hours = 40
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			wantSub: "unsupported top-level key",
		},
		{
			name: "week_work_hours not positive",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = -1
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			wantSub: "week_work_hours must be > 0",
		},
		{
			name: "unknown nested key",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
store_dri = "/tmp/typo"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			wantSub: `unsupported key "storage"."store_dri"`,
		},
		{
			name: "invalid bool env",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			env:     map[string]string{"TIMESAMURAI_COLOR": "maybe"},
			wantSub: "TIMESAMURAI_COLOR",
		},
		{
			name: "empty plus_for",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = []
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			wantSub: "plus_for must list at least one",
		},
		{
			name: "invalid weekend day",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Saturday"]
`,
			wantSub: "invalid day",
		},
		{
			name: "empty db_dir via env",
			content: `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`,
			env:     map[string]string{"TIMESAMURAI_DB_DIR": "   "},
			wantSub: "", // whitespace-only env is ignored; still valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTimesamuraiEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			path := filepath.Join(dir, tt.name+".toml")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(context.Background(), LoadOptions{ConfigPath: path})
			if tt.wantSub == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error %q should contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestValidate_Table(t *testing.T) {
	valid := Default()
	_ = valid.normalize()

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{
			name:    "ok",
			mutate:  func(*Config) {},
			wantSub: "",
		},
		{
			name: "empty db_dir",
			mutate: func(c *Config) {
				c.Storage.DBDir = ""
			},
			wantSub: "db_dir must not be empty",
		},
		{
			name: "empty store_dir",
			mutate: func(c *Config) {
				c.Storage.StoreDir = ""
			},
			wantSub: "store_dir must not be empty",
		},
		{
			name: "zero hours",
			mutate: func(c *Config) {
				c.Accounting.WeekWorkHours = 0
			},
			wantSub: "week_work_hours must be > 0",
		},
		{
			name: "blank tag",
			mutate: func(c *Config) {
				c.Accounting.MinusFor = []string{"lunch", "  "}
			},
			wantSub: "minus_for[1] must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			home, _ := os.UserHomeDir()
			cfg.Storage.DBDir = filepath.Join(home, "git/worktime")
			cfg.Storage.StoreDir = filepath.Join(home, "git/worktime/timesamuraidb")
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantSub == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("got %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestMerge_PartialSectionsPreserveDefaults(t *testing.T) {
	cfg := Default()
	ov := &overlay{
		Accounting: accountingOverlay{
			WeekWorkHours: floatPtr(32),
		},
		Report: reportOverlay{
			Verbose: boolPtr(true),
		},
	}
	cfg.mergeWith(ov)
	if cfg.Accounting.WeekWorkHours != 32 {
		t.Fatalf("hours: %v", cfg.Accounting.WeekWorkHours)
	}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, defaultPlusFor) {
		t.Fatalf("plus_for should stay default: %v", cfg.Accounting.PlusFor)
	}
	if !cfg.Report.Color {
		t.Fatal("color default should remain true")
	}
	if !cfg.Report.Verbose {
		t.Fatal("verbose should be overridden")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got := DefaultConfigPath()
	want := filepath.Join(dir, "timesamurai", "config.toml")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEnv_CSVLists(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	path := writeConfig(t, dir, `
[storage]
db_dir = "/tmp/db"
store_dir = "/tmp/store"
[accounting]
week_work_hours = 40
plus_for = ["off"]
minus_for = ["lunch"]
buffer_for = ["pet"]
weekend_days = ["Sat"]
`)
	t.Setenv("TIMESAMURAI_PLUS_FOR", "off, bank, sick")
	t.Setenv("TIMESAMURAI_WEEKEND_DAYS", "Sat,Sun")

	cfg, err := Load(context.Background(), LoadOptions{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, []string{"off", "bank", "sick"}) {
		t.Fatalf("plus_for: %v", cfg.Accounting.PlusFor)
	}
	if !reflect.DeepEqual(cfg.Accounting.WeekendDays, []string{"Sat", "Sun"}) {
		t.Fatalf("weekend_days: %v", cfg.Accounting.WeekendDays)
	}
}

func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }
