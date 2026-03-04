package worktime

import (
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/timesamurai/internal/config"
)

func TestComprehensiveDBRoundTripAndReportFixture(t *testing.T) {
	dbDir := t.TempDir()
	host := "fixture-host"

	// Seed deterministic fixture entries via public operations.
	if _, err := Login(dbDir, host, "work", mustDate(2026, 1, 5, 9, 0, 0), "start"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, err := Logout(dbDir, host, "work", mustDate(2026, 1, 5, 17, 0, 0), "stop"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := Add(dbDir, host, "lunch", time.Hour, mustDate(2026, 1, 5, 12, 0, 0), "lunch"); err != nil {
		t.Fatalf("Add(lunch) error = %v", err)
	}
	if _, err := Add(dbDir, host, "off", 8*time.Hour, mustDate(2026, 1, 6, 10, 0, 0), "off"); err != nil {
		t.Fatalf("Add(off) error = %v", err)
	}

	// Round-trip from host file back to merged entries.
	hostDB, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}
	if len(hostDB.Entries[host]) != 4 {
		t.Fatalf("host entries len = %d, want 4", len(hostDB.Entries[host]))
	}

	merged, err := LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(merged) != 4 {
		t.Fatalf("merged entries len = %d, want 4", len(merged))
	}

	cfg := config.Default()
	report, err := BuildReport(merged, cfg)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if len(report) != 1 {
		t.Fatalf("weeks len = %d, want 1", len(report))
	}

	week := report[0]
	if week.RequiredSeconds != 32*secondsPerHour {
		t.Fatalf("required seconds = %d, want %d", week.RequiredSeconds, 32*secondsPerHour)
	}
	if week.Values["work"] != 7*secondsPerHour {
		t.Fatalf("week work seconds = %d, want %d", week.Values["work"], 7*secondsPerHour)
	}
	if week.WeeklyBalanceSeconds != -25*secondsPerHour {
		t.Fatalf("weekly balance = %d, want %d", week.WeeklyBalanceSeconds, -25*secondsPerHour)
	}

	rendered := FormatReport(report, false, false)
	if !strings.Contains(rendered, "work:7.00h") {
		t.Fatalf("rendered report missing expected work value: %q", rendered)
	}
	if !strings.Contains(rendered, "off:8.00h") {
		t.Fatalf("rendered report missing expected off value: %q", rendered)
	}
	if !strings.Contains(rendered, "balance:-25.00h") {
		t.Fatalf("rendered report missing expected balance value: %q", rendered)
	}
}

func mustDate(year int, month time.Month, day int, hour int, minute int, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, time.Local)
}
