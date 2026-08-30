package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParseImportLine confirms the ported field-index parsing (date at
// fields[1], work at fields[2], lunch at fields[4], off at fields[6])
// matches the pre-rewrite tool's ParseImportLine on the same input line
// (see `git show pre-rewrite:internal/worktime/import_test.go`).
func TestParseImportLine(t *testing.T) {
	line := "Mon 06.01.2026: +8.00h lunch: +0.50h off: +1.00h"
	parsed, err := parseImportLine(line)
	if err != nil {
		t.Fatalf("parseImportLine() error = %v", err)
	}
	if parsed.workHours != 8 {
		t.Fatalf("workHours = %v, want 8", parsed.workHours)
	}
	if parsed.lunchHours != 0.5 {
		t.Fatalf("lunchHours = %v, want 0.5", parsed.lunchHours)
	}
	if parsed.offHours != 1 {
		t.Fatalf("offHours = %v, want 1", parsed.offHours)
	}
	if parsed.when.Year() != 2026 || parsed.when.Month() != time.January || parsed.when.Day() != 6 {
		t.Fatalf("when = %v, want 2026-01-06", parsed.when)
	}
}

// TestParseImportLine_TooFewFields confirms a malformed line (missing the
// off token) is rejected rather than silently misparsed.
func TestParseImportLine_TooFewFields(t *testing.T) {
	_, err := parseImportLine("Mon 06.01.2026: +8.00h lunch: +0.50h")
	if err == nil {
		t.Fatal("parseImportLine() error = nil, want an error for a too-short line")
	}
	if !strings.Contains(err.Error(), "unsupported import line") {
		t.Fatalf("error = %v, want it to mention unsupported import line", err)
	}
}

// TestParseImportDate_FallbackLayout confirms the "day.month.year" legacy
// layout still parses now that timefmt.ParseTime is tried first (it does
// not understand that layout, so the fallback must fire).
func TestParseImportDate_FallbackLayout(t *testing.T) {
	parsed, err := parseImportDate("06.01.2026")
	if err != nil {
		t.Fatalf("parseImportDate() error = %v", err)
	}
	if parsed.Year() != 2026 || parsed.Month() != time.January || parsed.Day() != 6 {
		t.Fatalf("parsed = %v, want 2026-01-06", parsed)
	}
}

// TestParseImportDate_CompactLayout confirms the other legacy layout
// ("20060102") also still parses.
func TestParseImportDate_CompactLayout(t *testing.T) {
	parsed, err := parseImportDate("20260106")
	if err != nil {
		t.Fatalf("parseImportDate() error = %v", err)
	}
	if parsed.Year() != 2026 || parsed.Month() != time.January || parsed.Day() != 6 {
		t.Fatalf("parsed = %v, want 2026-01-06", parsed)
	}
}

// TestParseImportDate_Invalid confirms an unparseable token is a clear
// error rather than a zero time.Time silently used as "now".
func TestParseImportDate_Invalid(t *testing.T) {
	_, err := parseImportDate("not-a-date")
	if err == nil {
		t.Fatal("parseImportDate() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported import date") {
		t.Fatalf("error = %v, want it to mention unsupported import date", err)
	}
}

// TestImport_AppliesWorkLunchOffEntries confirms `work import <file>`
// applies a report.txt-format file as worktime.Add calls: lunch folded into
// work's duration plus its own lunch entry, and a separate off entry, all
// dated to the parsed day.
func TestImport_AppliesWorkLunchOffEntries(t *testing.T) {
	store := newScratchStore(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	content := "===== Report =====\n" +
		"Mon 06.01.2026: +8.00h lunch: +0.50h off: +1.00h\n" +
		"------------------\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write report file: %v", err)
	}

	out, err := runWork(t, store, "import", file)
	if err != nil {
		t.Fatalf("work import: %v (output: %s)", err, out)
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 3 {
		t.Fatalf("entries = %+v, want 3 (work, lunch, off)", entries)
	}
	byTag := map[string]int64{}
	for _, e := range entries {
		byTag[e.Tags[0]] = e.Value
	}
	if byTag["work"] != 30600 {
		t.Fatalf("work value = %d, want 30600 (8.5h, lunch folded in)", byTag["work"])
	}
	if byTag["lunch"] != 1800 {
		t.Fatalf("lunch value = %d, want 1800 (0.5h)", byTag["lunch"])
	}
	if byTag["off"] != 3600 {
		t.Fatalf("off value = %d, want 3600 (1h)", byTag["off"])
	}
}

// TestImport_SkipsNonDayLines confirms lines without "lunch:" (headers,
// separators, totals) are skipped rather than rejected as malformed.
func TestImport_SkipsNonDayLines(t *testing.T) {
	store := newScratchStore(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	content := "this line has no lunch marker and would fail to parse\n" +
		"Mon 06.01.2026: +2.00h lunch: +0.00h off: +0.00h\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write report file: %v", err)
	}

	if _, err := runWork(t, store, "import", file); err != nil {
		t.Fatalf("work import: %v", err)
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 1 || entries[0].Tags[0] != "work" || entries[0].Value != 7200 {
		t.Fatalf("entries = %+v, want a single 2h work entry", entries)
	}
}

// TestImport_MalformedDayLineErrors confirms a line that does contain
// "lunch:" but is otherwise malformed surfaces a parse error through the
// command rather than being silently skipped or partially applied.
func TestImport_MalformedDayLineErrors(t *testing.T) {
	store := newScratchStore(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(file, []byte("garbled lunch: line\n"), 0o644); err != nil {
		t.Fatalf("write report file: %v", err)
	}

	_, err := runWork(t, store, "import", file)
	if err == nil {
		t.Fatal("work import: expected an error for a malformed day line, got nil")
	}
	if len(readEntries(t, store, currentHost(t))) != 0 {
		t.Fatalf("no entries should have been applied on a parse error")
	}
}

// TestImport_MissingFileErrors confirms a nonexistent file is a clear
// open error, not a confusing downstream failure.
func TestImport_MissingFileErrors(t *testing.T) {
	store := newScratchStore(t)
	_, err := runWork(t, store, "import", filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatal("work import: expected an error for a missing file, got nil")
	}
	if !strings.Contains(err.Error(), "open import file") {
		t.Fatalf("error = %v, want it to mention opening the import file", err)
	}
}
