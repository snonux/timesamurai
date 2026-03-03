package worktime

import (
	"strings"
	"testing"
	"time"
)

func TestParseImportDateFallbackLayout(t *testing.T) {
	parsed, err := ParseImportDate("06.01.2026")
	if err != nil {
		t.Fatalf("ParseImportDate() error = %v", err)
	}

	expected := time.Date(2026, 1, 6, 0, 0, 0, 0, time.Local)
	if parsed.Year() != expected.Year() || parsed.Month() != expected.Month() || parsed.Day() != expected.Day() {
		t.Fatalf("parsed date = %v, want %v", parsed, expected)
	}
}

func TestParseImportDateIncludesUnderlyingParseError(t *testing.T) {
	_, err := ParseImportDate("not-a-date")
	if err == nil {
		t.Fatal("ParseImportDate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported import date") {
		t.Fatalf("ParseImportDate() error = %v, want unsupported import date context", err)
	}
}

func TestParseImportLine(t *testing.T) {
	line := "Mon 06.01.2026: +8.00h lunch: +0.50h off: +1.00h"
	parsed, err := ParseImportLine(line)
	if err != nil {
		t.Fatalf("ParseImportLine() error = %v", err)
	}

	if parsed.WorkHours != 8 {
		t.Fatalf("WorkHours = %v, want 8", parsed.WorkHours)
	}
	if parsed.LunchHours != 0.5 {
		t.Fatalf("LunchHours = %v, want 0.5", parsed.LunchHours)
	}
	if parsed.OffHours != 1 {
		t.Fatalf("OffHours = %v, want 1", parsed.OffHours)
	}
}

func TestImportReport(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"
	report := strings.NewReader("Mon 06.01.2026: +8.00h lunch: +0.50h off: +0.00h\n")

	imported, err := ImportReport(dbDir, host, report)
	if err != nil {
		t.Fatalf("ImportReport() error = %v", err)
	}
	if imported != 2 {
		t.Fatalf("imported = %d, want 2", imported)
	}

	db, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}
	entries := db.Entries[host]
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0].What != "work" || entries[0].Value != 30600 {
		t.Fatalf("work entry = %+v, want 8.5h in seconds", entries[0])
	}
	if entries[1].What != "lunch" || entries[1].Value != 1800 {
		t.Fatalf("lunch entry = %+v, want 0.5h in seconds", entries[1])
	}
}
