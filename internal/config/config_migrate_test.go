package config

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeLegacyJSON(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "timesamurai", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	return path
}

func TestMigrate_JSONOnlyCreatesTOML(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	jsonPath := writeLegacyJSON(t, dir, `{
  "weekworkhours": 35,
  "plusfor": ["off", "sick"],
  "minusfor": ["lunch"],
  "bufferfor": ["pet"],
  "weekendays": ["Sat"],
  "worktime_db_dir": "/tmp/legacy-db",
  "hostname": "earth",
  "auto_worktime_login": true
}`)

	var notice bytes.Buffer
	cfg, err := Load(context.Background(), LoadOptions{NoticeWriter: &notice})
	if err != nil {
		t.Fatal(err)
	}

	tomlPath := filepath.Join(dir, "timesamurai", "config.toml")
	if _, err := os.Stat(tomlPath); err != nil {
		t.Fatalf("expected migrated toml: %v", err)
	}
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("json should remain: %v", err)
	}
	if notice.Len() != 0 {
		t.Fatalf("no ignore notice on first migrate, got %q", notice.String())
	}

	if cfg.Storage.DBDir != "/tmp/legacy-db" {
		t.Fatalf("db_dir: %q", cfg.Storage.DBDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	wantStore := filepath.Join(home, "git/worktime/timesamuraidb")
	if cfg.Storage.StoreDir != wantStore {
		t.Fatalf("store_dir: got %q want %q", cfg.Storage.StoreDir, wantStore)
	}
	if cfg.Accounting.WeekWorkHours != 35 {
		t.Fatalf("hours: %v", cfg.Accounting.WeekWorkHours)
	}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, []string{"off", "sick"}) {
		t.Fatalf("plus_for: %v", cfg.Accounting.PlusFor)
	}
	if !reflect.DeepEqual(cfg.Accounting.WeekendDays, []string{"Sat"}) {
		t.Fatalf("weekend_days: %v", cfg.Accounting.WeekendDays)
	}
	if cfg.General.Hostname != "earth" || !cfg.General.AutoWorktimeLogin {
		t.Fatalf("general: %+v", cfg.General)
	}
	if !cfg.Report.Color || cfg.Report.Verbose {
		t.Fatalf("report defaults should apply: %+v", cfg.Report)
	}
}

func TestMigrate_BothExistTOMLWinsWithNotice(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	writeLegacyJSON(t, dir, `{
  "weekworkhours": 20,
  "plusfor": ["off"],
  "minusfor": ["lunch"],
  "bufferfor": ["pet"],
  "weekendays": ["Sun"],
  "worktime_db_dir": "/tmp/from-json",
  "hostname": "from-json"
}`)
	writeConfig(t, dir, `
[storage]
db_dir = "/tmp/from-toml"
store_dir = "/tmp/store-toml"
[accounting]
week_work_hours = 42
plus_for = ["bank"]
minus_for = ["lunch"]
buffer_for = ["tools"]
weekend_days = ["Sat"]
[general]
hostname = "from-toml"
`)

	var notice bytes.Buffer
	cfg, err := Load(context.Background(), LoadOptions{NoticeWriter: &notice})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Storage.DBDir != "/tmp/from-toml" {
		t.Fatalf("TOML should win db_dir: %q", cfg.Storage.DBDir)
	}
	if cfg.Accounting.WeekWorkHours != 42 {
		t.Fatalf("TOML should win hours: %v", cfg.Accounting.WeekWorkHours)
	}
	if cfg.General.Hostname != "from-toml" {
		t.Fatalf("TOML should win hostname: %q", cfg.General.Hostname)
	}
	if !strings.Contains(notice.String(), "ignoring legacy") {
		t.Fatalf("expected ignore notice, got %q", notice.String())
	}
	if !strings.Contains(notice.String(), "config.json") {
		t.Fatalf("notice should name json: %q", notice.String())
	}
}

func TestMigrate_SecondLoadDoesNotRewrite(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	writeLegacyJSON(t, dir, `{
  "weekworkhours": 36,
  "plusfor": ["off"],
  "minusfor": ["lunch"],
  "bufferfor": ["pet"],
  "weekendays": ["Sat"],
  "worktime_db_dir": "/tmp/once"
}`)

	if _, err := Load(context.Background(), LoadOptions{NoticeWriter: ioDiscard{}}); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(dir, "timesamurai", "config.toml")
	first, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatal(err)
	}

	var notice bytes.Buffer
	if _, err := Load(context.Background(), LoadOptions{NoticeWriter: &notice}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("second load rewrote config.toml")
	}
	if !strings.Contains(notice.String(), "ignoring legacy") {
		t.Fatalf("second load should notice ignored json, got %q", notice.String())
	}
}

func TestMigrate_PartialJSONKeepsDefaults(t *testing.T) {
	clearTimesamuraiEnv(t)
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"worktime_db_dir":"/tmp/partial-db"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(context.Background(), LoadOptions{
		ConfigPath:   tomlPath,
		IgnoreEnv:    true,
		NoticeWriter: ioDiscard{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.DBDir != "/tmp/partial-db" {
		t.Fatalf("db_dir: %q", cfg.Storage.DBDir)
	}
	if cfg.Accounting.WeekWorkHours != 40 {
		t.Fatalf("hours default: %v", cfg.Accounting.WeekWorkHours)
	}
	if !reflect.DeepEqual(cfg.Accounting.PlusFor, defaultPlusFor) {
		t.Fatalf("plus_for default: %v", cfg.Accounting.PlusFor)
	}
}

func TestMigrate_Negatives(t *testing.T) {
	clearTimesamuraiEnv(t)

	tests := []struct {
		name    string
		json    string
		wantSub string
	}{
		{
			name:    "invalid JSON",
			json:    `{weekworkhours:`,
			wantSub: "parse legacy config",
		},
		{
			name: "empty plusfor fails validate after migrate",
			json: `{
  "weekworkhours": 40,
  "plusfor": [],
  "minusfor": ["lunch"],
  "bufferfor": ["pet"],
  "weekendays": ["Sat"],
  "worktime_db_dir": "/tmp/db"
}`,
			wantSub: "plus_for must list at least one",
		},
		{
			name: "invalid weekend day from JSON",
			json: `{
  "weekworkhours": 40,
  "plusfor": ["off"],
  "minusfor": ["lunch"],
  "bufferfor": ["pet"],
  "weekendays": ["Saturday"],
  "worktime_db_dir": "/tmp/db"
}`,
			wantSub: "invalid day",
		},
		{
			name: "zero week hours uses default then ok",
			json: `{
  "weekworkhours": 0,
  "worktime_db_dir": "/tmp/db"
}`,
			wantSub: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearTimesamuraiEnv(t)
			dir := t.TempDir()
			tomlPath := filepath.Join(dir, "config.toml")
			jsonPath := filepath.Join(dir, "config.json")
			if err := os.WriteFile(jsonPath, []byte(tt.json), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(context.Background(), LoadOptions{
				ConfigPath:   tomlPath,
				IgnoreEnv:    true,
				NoticeWriter: ioDiscard{},
			})
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

func TestMigrate_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"worktime_db_dir":"/tmp/db"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := maybeMigrateLegacyJSON(ctx, tomlPath, nil)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("got %v, want cancelled", err)
	}
}

func TestLegacyJSONPath(t *testing.T) {
	got := legacyJSONPath("/home/x/.config/timesamurai/config.toml")
	want := "/home/x/.config/timesamurai/config.json"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMigrateDoc_Table(t *testing.T) {
	hours := 33.0
	auto := true
	db := "~/git/worktime"
	host := "earth"

	tests := []struct {
		name  string
		leg   legacyJSON
		check func(t *testing.T, doc migrateDoc)
	}{
		{
			name: "full mapping",
			leg: legacyJSON{
				WeekWorkHours:     &hours,
				PlusFor:           []string{"off"},
				MinusFor:          []string{"lunch"},
				BufferFor:         []string{"pet"},
				WeekendDays:       []string{"Sat", "Sun"},
				WorktimeDBDir:     &db,
				Hostname:          &host,
				AutoWorktimeLogin: &auto,
			},
			check: func(t *testing.T, doc migrateDoc) {
				t.Helper()
				if doc.Storage == nil || doc.Storage.DBDir != db {
					t.Fatalf("storage: %+v", doc.Storage)
				}
				if doc.Storage.StoreDir != defaultWorktimeStoreDir {
					t.Fatalf("store_dir seed: %q", doc.Storage.StoreDir)
				}
				if doc.Accounting == nil || *doc.Accounting.WeekWorkHours != 33 {
					t.Fatalf("accounting: %+v", doc.Accounting)
				}
				if doc.General == nil || doc.General.Hostname != "earth" || !*doc.General.AutoWorktimeLogin {
					t.Fatalf("general: %+v", doc.General)
				}
			},
		},
		{
			name: "empty json seeds store only",
			leg:  legacyJSON{},
			check: func(t *testing.T, doc migrateDoc) {
				t.Helper()
				if doc.Storage == nil || doc.Storage.StoreDir != defaultWorktimeStoreDir {
					t.Fatalf("storage: %+v", doc.Storage)
				}
				if doc.Storage.DBDir != "" {
					t.Fatalf("db_dir should be omitted: %q", doc.Storage.DBDir)
				}
				if doc.Accounting != nil || doc.General != nil {
					t.Fatalf("unexpected sections: %+v", doc)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, buildMigrateDoc(tt.leg))
		})
	}
}

// ioDiscard is a tiny io.Writer that drops bytes (avoids importing io solely for Discard in helpers).
type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
