package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/worktime"
)

// stripSGR removes all ANSI escape sequences, so tests can assert that a
// colorized rendering still carries exactly the same visible text as the
// plain one.
var stripSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func enabledTheme(t *testing.T) ansi.Theme {
	t.Helper()
	theme, err := ansi.NewTheme(true, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	return theme
}

func disabledTheme(t *testing.T) ansi.Theme {
	t.Helper()
	theme, err := ansi.NewTheme(false, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	return theme
}

func TestColorizeReportDisabledIsIdentity(t *testing.T) {
	plain := sampleReport(t, disabledTheme(t))
	if strings.ContainsRune(plain, '\x1b') {
		t.Fatalf("disabled theme emitted escape sequences: %q", plain)
	}
}

func TestColorizeReportStylesSeparatorAndBalance(t *testing.T) {
	theme := enabledTheme(t)
	plain := sampleReport(t, disabledTheme(t))
	styled := colorizeReport(theme, plain)

	if !strings.Contains(styled, "\x1b[2m====") {
		t.Errorf("separator line not faint-styled:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[31m balance:-33.00h\x1b[0m") {
		t.Errorf("negative balance not red:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[32m balance:7.00h\x1b[0m") {
		t.Errorf("positive balance not green:\n%q", styled)
	}
	// Visible text must be byte-identical once escapes are stripped.
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

// sampleReport builds a two-week report through the real formatter. The
// balance is the running cross-week total (7h then 80h of work against a
// 40h weekly target gives -33.00h then +7.00h cumulative), exercising both
// balance color roles on real FormatReport output shapes.
func sampleReport(t *testing.T, theme ansi.Theme) string {
	t.Helper()
	cfg := worktime.AccountingConfig{
		WeekWorkHours: 40,
		PlusFor:       []string{"off", "bank", "bufferuse", "sick"},
		MinusFor:      []string{"lunch"},
		WeekendDays:   []string{"Sat", "Sun"},
	}
	entries := []worktime.Entry{
		{Host: "h1", ID: 1, Action: "add", Epoch: time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC).Unix(), Value: 7 * 3600, Tags: []string{"work"}},
		{Host: "h1", ID: 2, Action: "add", Epoch: time.Date(2024, 1, 9, 9, 0, 0, 0, time.UTC).Unix(), Value: 80 * 3600, Tags: []string{"work"}},
	}
	weeks, err := worktime.BuildReport(entries, cfg, nil)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	return colorizeReport(theme, worktime.FormatReport(weeks, false))
}

func TestColorizeTableStylesHeaderAndAddresses(t *testing.T) {
	theme := enabledTheme(t)
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000, Value: 3600, Tags: []string{"work"}}},
	}
	plain := worktime.FormatTable(rows)
	styled := colorizeTable(theme, plain)

	if !strings.HasPrefix(styled, "\x1b[1mADDRESS") {
		t.Errorf("header line not bold:\n%q", styled)
	}
	if !strings.Contains(styled, "\x1b[36mhost1:1\x1b[0m") {
		t.Errorf("address cell not cyan:\n%q", styled)
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("colorized visible text differs from plain:\n got %q\nwant %q", got, plain)
	}
}

func TestColorizeTableDisabledIsIdentity(t *testing.T) {
	rows := []worktime.Row{
		{Address: "host1:1", Entry: worktime.Entry{Host: "host1", ID: 1, Action: "add", Epoch: 1700000000}},
	}
	plain := worktime.FormatTable(rows)
	if got := colorizeTable(disabledTheme(t), plain); got != plain {
		t.Errorf("disabled theme changed table output:\n got %q\nwant %q", got, plain)
	}
}

// TestReportOutputColoredWhenForced runs the real `work report` command with
// CLICOLOR_FORCE=1 (colors on despite the test's non-TTY stdout) and asserts
// escape sequences reach the output.
func TestReportOutputColoredWhenForced(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	forced, err := runWork(t, store, "report", "2024-01-01..2024-01-31")
	if err != nil {
		t.Fatalf("report (forced): %v", err)
	}
	if !strings.Contains(forced, "\x1b[") {
		t.Errorf("forced-color report has no escape sequences:\n%q", forced)
	}
}

// TestReportOutputPlainWithoutForce guards the inverse: without
// CLICOLOR_FORCE (and without a tty), report output must stay byte-plain.
func TestReportOutputPlainWithoutForce(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}
	out, err := runWork(t, store, "report", "2024-01-01..2024-01-31")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("plain report leaked escape sequences:\n%q", out)
	}
}

// TestNoColorFlagBeatsForce pins --no-color's position in the precedence:
// even with CLICOLOR_FORCE=1, an explicit --no-color disables all styling.
func TestNoColorFlagBeatsForce(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "--at", "2024-01-02T09:00:00Z"); err != nil {
		t.Fatalf("seed add: %v", err)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	out, err := runWork(t, store, "report", "2024-01-01..2024-01-31", "--no-color")
	if err != nil {
		t.Fatalf("report --no-color: %v", err)
	}
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("--no-color still emitted escape sequences:\n%q", out)
	}
}

// TestColorizeReportEmptyStaysEmpty pins the guard against the empty-range
// report gaining stray newlines when colors are enabled (review finding).
func TestColorizeReportEmptyStaysEmpty(t *testing.T) {
	theme := enabledTheme(t)
	if got := colorizeReport(theme, ""); got != "" {
		t.Errorf("colorizeReport(\"\") = %q, want empty", got)
	}
	if got := colorizeTable(theme, ""); got != "" {
		t.Errorf("colorizeTable(\"\") = %q, want empty", got)
	}
}
