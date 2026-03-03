package worktime

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"codeberg.org/snonux/timr/internal/config"
)

func TestBuildReportBalanceAndMarkers(t *testing.T) {
	cfg := config.Default()

	entries := []Entry{
		{Action: "login", What: "work", Epoch: localEpoch(2026, 1, 5, 9, 0, 0)},
		{Action: "add", What: "lunch", Epoch: localEpoch(2026, 1, 5, 12, 0, 0), Value: 3600},
		{Action: "logout", What: "work", Epoch: localEpoch(2026, 1, 5, 17, 0, 0)},
		{Action: "add", What: "off", Epoch: localEpoch(2026, 1, 6, 12, 0, 0), Value: 8 * 3600},
		{Action: "login", What: "work", Epoch: localEpoch(2026, 1, 7, 9, 0, 0)},
		{Action: "logout", What: "work", Epoch: localEpoch(2026, 1, 7, 17, 0, 0)},
	}

	weeks, err := BuildReport(entries, cfg)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}

	if len(weeks) != 1 {
		t.Fatalf("weeks len = %d, want 1", len(weeks))
	}

	week := weeks[0]
	if len(week.Days) != 3 {
		t.Fatalf("week days len = %d, want 3", len(week.Days))
	}

	mon := week.Days[0]
	if mon.Values["work"] != 7*secondsPerHour {
		t.Fatalf("monday work = %d, want %d", mon.Values["work"], 7*secondsPerHour)
	}

	tue := week.Days[1]
	if tue.Marker != "*" {
		t.Fatalf("tuesday marker = %q, want *", tue.Marker)
	}
	if tue.RequiredSeconds != 0 {
		t.Fatalf("tuesday required = %d, want 0", tue.RequiredSeconds)
	}

	if week.RequiredSeconds != 32*secondsPerHour {
		t.Fatalf("week required = %d, want %d", week.RequiredSeconds, 32*secondsPerHour)
	}

	if week.Values["work"] != 15*secondsPerHour {
		t.Fatalf("week work = %d, want %d", week.Values["work"], 15*secondsPerHour)
	}

	if week.WeeklyBalanceSeconds != -17*secondsPerHour {
		t.Fatalf("weekly balance = %d, want %d", week.WeeklyBalanceSeconds, -17*secondsPerHour)
	}

	if week.CumulativeBalanceSeconds != -17*secondsPerHour {
		t.Fatalf("cumulative balance = %d, want %d", week.CumulativeBalanceSeconds, -17*secondsPerHour)
	}
}

func TestBuildReportTracksBufferTotals(t *testing.T) {
	cfg := config.Default()

	entries := []Entry{
		{Action: "add", What: "selfdevelopment", Epoch: localEpoch(2026, 1, 5, 11, 0, 0), Value: 2 * 3600},
		{Action: "add", What: "work", Epoch: localEpoch(2026, 1, 5, 12, 0, 0), Value: 3600},
	}

	weeks, err := BuildReport(entries, cfg)
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}

	if len(weeks) != 1 {
		t.Fatalf("weeks len = %d, want 1", len(weeks))
	}

	if weeks[0].BufferSeconds != 2*secondsPerHour {
		t.Fatalf("buffer seconds = %d, want %d", weeks[0].BufferSeconds, 2*secondsPerHour)
	}
}

func TestBuildReportRejectsInvalidLoginSequences(t *testing.T) {
	cfg := config.Default()

	_, err := BuildReport([]Entry{
		{Action: "logout", What: "work", Epoch: localEpoch(2026, 1, 5, 10, 0, 0)},
	}, cfg)
	if err == nil {
		t.Fatal("BuildReport() accepted logout without login")
	}

	_, err = BuildReport([]Entry{
		{Action: "login", What: "work", Epoch: localEpoch(2026, 1, 5, 9, 0, 0)},
		{Action: "login", What: "work", Epoch: localEpoch(2026, 1, 5, 10, 0, 0)},
	}, cfg)
	if err == nil {
		t.Fatal("BuildReport() accepted double login")
	}
}

func TestBuildReportRejectsUnknownAction(t *testing.T) {
	cfg := config.Default()

	_, err := BuildReport([]Entry{
		{Action: "mystery", What: "work", Epoch: localEpoch(2026, 1, 5, 10, 0, 0)},
	}, cfg)
	if err == nil {
		t.Fatal("BuildReport() accepted unknown action")
	}
}

func TestBuildReportEmptyInput(t *testing.T) {
	weeks, err := BuildReport(nil, config.Default())
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}
	if len(weeks) != 0 {
		t.Fatalf("weeks len = %d, want 0", len(weeks))
	}
}

func TestFormatReportVerboseAndColor(t *testing.T) {
	entries := []Entry{
		{Action: "add", What: "work", Epoch: localEpoch(2026, 1, 5, 10, 0, 0), Value: 3600},
	}
	weeks, err := BuildReport(entries, config.Default())
	if err != nil {
		t.Fatalf("BuildReport() error = %v", err)
	}

	colored := FormatReport(weeks, true, true)
	if !strings.Contains(colored, "\x1b[") {
		t.Fatalf("colored output does not contain ANSI color sequences: %q", colored)
	}
	coloredPlain := stripANSI(colored)
	if !strings.Contains(coloredPlain, "work:") {
		t.Fatalf("colored output missing work field: %q", colored)
	}
	if !strings.Contains(coloredPlain, "epoch:") {
		t.Fatalf("colored output missing verbose epoch: %q", colored)
	}

	plain := FormatReport(weeks, false, false)
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain output contains ANSI color sequences: %q", plain)
	}
}

func localEpoch(year int, month time.Month, day int, hour int, minute int, second int) int64 {
	return time.Date(year, month, day, hour, minute, second, 0, time.Local).Unix()
}

func stripANSI(value string) string {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiPattern.ReplaceAllString(value, "")
}
