package worktime

import (
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/config"
)

// These are fast, synthetic, in-memory tests for the individual worktime.rb
// quirks report.go reproduces. The byte-for-byte comparison against a fresh
// `ruby worktime.rb --report` run over the real 12,802-entry dataset is a
// separate, non-hermetic check (task 271's golden-diff test), not repeated
// here.

func epochAt(year, month, day, hour, minute, second int) int64 {
	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local).Unix()
}

func mkEntry(id int64, action string, epoch int64, value int64, tags ...string) Entry {
	return Entry{ID: id, Action: action, Epoch: epoch, Host: "h1", Value: value, Tags: tags}
}

func buildAndFormat(t *testing.T, cfg config.AccountingConfig, entries []Entry) ([]WeekReport, string) {
	t.Helper()
	weeks, err := BuildReport(entries, cfg)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	return weeks, FormatReport(weeks, false)
}

func TestDayLabelFormat(t *testing.T) {
	// 2024-01-01 is a Monday and ISO week 1 of 2024.
	got := dayLabel(epochAt(2024, 1, 1, 9, 0, 0))
	if want := "Mon 20240101 01"; got != want {
		t.Fatalf("dayLabel = %q, want %q", got, want)
	}
}

func TestWeekKeyIgnoresYear(t *testing.T) {
	// Both are the Monday opening ISO week 1 of their respective years —
	// worktime.rb's get_epoch_weekstr formats only "%V" (no year), so a
	// week boundary is a bug-compatible year-blind comparison.
	a := weekKey(epochAt(2023, 1, 2, 9, 0, 0))
	b := weekKey(epochAt(2024, 1, 1, 9, 0, 0))
	if a != b {
		t.Fatalf("weekKey should ignore year: %q vs %q", a, b)
	}
}

func TestWorkAlwaysPrintsEvenWhenAbsent(t *testing.T) {
	cfg := config.Default().Accounting
	day := epochAt(2024, 1, 3, 12, 0, 0) // Wednesday, no work entries at all
	// Deliberately NOT "lunch" (a minusfor category): print_day's
	// `day['values']['work'] -= day['values']['lunch']` would crash real
	// Ruby (nil -= Integer) on a day with no work key at all, so that
	// combination never occurs in real data and isn't a fair case to
	// assert byte-for-byte behavior against here.
	entries := []Entry{mkEntry(1, actionAdd, day, 1800, "off")}

	_, out := buildAndFormat(t, cfg, entries)
	if !strings.Contains(out, "work:0.00h") {
		t.Fatalf("expected always-printed work:0.00h, got:\n%s", out)
	}
	if !strings.Contains(out, "off:0.50h") {
		t.Fatalf("expected off:0.50h, got:\n%s", out)
	}
}

func TestZeroValueSuppressedExceptWork(t *testing.T) {
	cfg := config.Default().Accounting
	day := epochAt(2024, 1, 3, 12, 0, 0)
	entries := []Entry{
		mkEntry(1, actionAdd, day, 3600, WorkTag),
		mkEntry(2, actionAdd, day, 0, "sick"),
	}

	_, out := buildAndFormat(t, cfg, entries)
	if strings.Contains(out, "sick:") {
		t.Fatalf("zero-value sick must be suppressed, got:\n%s", out)
	}
}

func TestPrintOrderIsFixedRegardlessOfInsertionOrder(t *testing.T) {
	cfg := config.Default().Accounting
	day := epochAt(2024, 1, 3, 12, 0, 0)
	// Insert every category in reverse of the required print order.
	entries := []Entry{
		mkEntry(1, actionAdd, day, 60, "selfdevelopment"),
		mkEntry(2, actionAdd, day, 60, "pet"),
		mkEntry(3, actionAdd, day, 60, "bank"),
		mkEntry(4, actionAdd, day, 60, "sick"),
		mkEntry(5, actionAdd, day, 60, "off"),
		mkEntry(6, actionAdd, day, 60, "lunch"),
		mkEntry(7, actionAdd, day, 60, WorkTag),
	}

	_, out := buildAndFormat(t, cfg, entries)
	line := strings.Split(out, "\n")[0]
	order := []string{"work:", "lunch:", "off:", "sick:", "bank:", "pet:", "selfdevelopment:"}
	last := -1
	for _, key := range order {
		idx := strings.Index(line, key)
		if idx < 0 {
			t.Fatalf("missing %q in line %q", key, line)
		}
		if idx < last {
			t.Fatalf("category %q out of print order in line %q", key, line)
		}
		last = idx
	}
}

func TestDayMarker(t *testing.T) {
	cfg := config.Default().Accounting
	entries := []Entry{
		mkEntry(1, actionAdd, epochAt(2024, 1, 2, 9, 0, 0), int64(2*secondsPerHour), "off"),   // Tue, off<8h
		mkEntry(2, actionAdd, epochAt(2024, 1, 3, 9, 0, 0), eightHours, "off"),                // Wed, off==8h
		mkEntry(3, actionAdd, epochAt(2024, 1, 4, 9, 0, 0), eightHours, "bank"),               // Thu, bank==8h
		mkEntry(4, actionAdd, epochAt(2024, 1, 6, 9, 0, 0), int64(1*secondsPerHour), WorkTag), // Sat, weekend
	}

	weeks, _ := buildAndFormat(t, cfg, entries)
	if len(weeks) != 1 || len(weeks[0].Days) != 4 {
		t.Fatalf("expected 1 week of 4 days, got %+v", weeks)
	}
	want := []string{" ", "*", "*", "*"}
	for i, day := range weeks[0].Days {
		if day.Marker != want[i] {
			t.Fatalf("day %d marker = %q, want %q (label %s)", i, day.Marker, want[i], day.Label)
		}
	}
}

func TestMinusForSubtractionIsNotDoubled(t *testing.T) {
	cfg := config.Default().Accounting
	day1 := epochAt(2024, 1, 2, 9, 0, 0) // Tue
	day2 := epochAt(2024, 1, 3, 9, 0, 0) // Wed

	entries := []Entry{
		mkEntry(1, actionLogin, day1, 0, WorkTag),
		mkEntry(2, actionLogout, day1+8*secondsPerHour, 0, WorkTag), // 8h work
		mkEntry(3, actionAdd, day1, secondsPerHour, "lunch"),        // 1h lunch
		mkEntry(4, actionAdd, day2, 4*secondsPerHour, WorkTag),      // 4h work
		mkEntry(5, actionAdd, day2, secondsPerHour/2, "lunch"),      // 0.5h lunch
	}

	weeks, out := buildAndFormat(t, cfg, entries)
	week := weeks[0]

	// Day-level display: work reduced by that day's own lunch.
	if !strings.Contains(out, "work:7.00h lunch:1.00h") {
		t.Fatalf("day 1 display wrong, got:\n%s", out)
	}
	if !strings.Contains(out, "work:3.50h lunch:0.50h") {
		t.Fatalf("day 2 display wrong, got:\n%s", out)
	}
	// Week-level: sum(work)-sum(lunch) == sum(work-lunch per day), i.e. the
	// day-level and week-level minusfor subtractions are the same single
	// deduction viewed two ways, not applied twice.
	if got, want := week.Values[WorkTag], int64(37800); got != want {
		t.Fatalf("week work = %d, want %d (10.5h)", got, want)
	}
}

func TestPlusForReducesWeeklyTargetAndBalanceAccumulates(t *testing.T) {
	cfg := config.Default().Accounting
	week1Day := epochAt(2024, 1, 2, 9, 0, 0) // ISO week 1
	week2Day := epochAt(2024, 1, 9, 9, 0, 0) // ISO week 2

	entries := []Entry{
		// Week 1: 44h work, 4h off (plusfor) -> target 36h -> balance +8h.
		mkEntry(1, actionAdd, week1Day, 44*secondsPerHour, WorkTag),
		mkEntry(2, actionAdd, week1Day, 4*secondsPerHour, "off"),
		// Week 2: 30h work, no plusfor -> target 40h -> weekly balance -10h.
		mkEntry(3, actionAdd, week2Day, 30*secondsPerHour, WorkTag),
	}

	weeks, out := buildAndFormat(t, cfg, entries)
	if len(weeks) != 2 {
		t.Fatalf("expected 2 weeks, got %d", len(weeks))
	}
	if got, want := weeks[0].Values["balance"], int64(8*secondsPerHour); got != want {
		t.Fatalf("week 1 balance = %d, want %d", got, want)
	}
	// Cumulative: +8h then -10h == -2h, carried across the week boundary.
	if got, want := weeks[1].Values["balance"], int64(-2*secondsPerHour); got != want {
		t.Fatalf("week 2 balance = %d, want %d", got, want)
	}
	if !strings.Contains(out, "balance:8.00h") || !strings.Contains(out, "balance:-2.00h") {
		t.Fatalf("expected both balances rendered, got:\n%s", out)
	}
}

func TestBufferExcludesLoginLogoutDurations(t *testing.T) {
	cfg := config.Default().Accounting // bufferfor includes "pet"
	day := epochAt(2024, 1, 2, 9, 0, 0)

	entries := []Entry{
		mkEntry(1, actionLogin, day, 0, "pet"),
		mkEntry(2, actionLogout, day+secondsPerHour, 0, "pet"), // 1h, must NOT count as buffer
		mkEntry(3, actionAdd, day, secondsPerHour/2, "pet"),    // 0.5h add, MUST count as buffer
	}

	weeks, _ := buildAndFormat(t, cfg, entries)
	week := weeks[0]

	if got, want := week.Buffer, int64(secondsPerHour/2); got != want {
		t.Fatalf("buffer = %d, want %d (login/logout duration must be excluded)", got, want)
	}
	// The "pet" category itself still shows both sources combined.
	if got, want := week.Values["pet"], int64(secondsPerHour+secondsPerHour/2); got != want {
		t.Fatalf("pet total = %d, want %d", got, want)
	}
}

func TestLoginOverwriteSilentlyDiscardsPreviousOpenLogin(t *testing.T) {
	cfg := config.Default().Accounting
	day := epochAt(2024, 1, 2, 9, 0, 0)
	t0, t1, t2 := day, day+2*secondsPerHour, day+3*secondsPerHour

	// A second login for "work" arrives while the first is still open.
	// Ruby's "already logged in" guard checks a dead literal key and is
	// always false, so this must NOT error: it silently drops the first
	// login (no duration ever attributed to it) and the eventual logout
	// closes the SECOND login only.
	entries := []Entry{
		mkEntry(1, actionLogin, t0, 0, WorkTag),
		mkEntry(2, actionLogin, t1, 0, WorkTag),
		mkEntry(3, actionLogout, t2, 0, WorkTag),
	}

	weeks, err := BuildReport(entries, cfg)
	if err != nil {
		t.Fatalf("BuildReport must not error on the double-login bug: %v", err)
	}
	if got, want := weeks[0].Values[WorkTag], t2-t1; got != want {
		t.Fatalf("work = %d, want %d (only the second login's span)", got, want)
	}
}

func TestLogoutWithoutLoginErrors(t *testing.T) {
	cfg := config.Default().Accounting
	entries := []Entry{mkEntry(1, actionLogout, epochAt(2024, 1, 2, 9, 0, 0), 0, WorkTag)}

	if _, err := BuildReport(entries, cfg); err == nil {
		t.Fatal("expected an error for logout without a matching login")
	}
}

func TestThreeTrailingNewlinesPerWeekBlock(t *testing.T) {
	cfg := config.Default().Accounting
	entries := []Entry{mkEntry(1, actionAdd, epochAt(2024, 1, 2, 9, 0, 0), secondsPerHour, WorkTag)}

	_, out := buildAndFormat(t, cfg, entries)
	if !strings.HasSuffix(out, "\n\n\n") {
		t.Fatalf("expected exactly three trailing newlines, got tail %q", out[len(out)-6:])
	}
	if strings.HasSuffix(out, "\n\n\n\n") {
		t.Fatalf("expected exactly three trailing newlines, found a fourth")
	}
}

func TestBufferAlwaysPrintedOnWeekLineEvenWhenZero(t *testing.T) {
	cfg := config.Default().Accounting
	entries := []Entry{mkEntry(1, actionAdd, epochAt(2024, 1, 2, 9, 0, 0), secondsPerHour, WorkTag)}

	_, out := buildAndFormat(t, cfg, entries)
	if !strings.Contains(out, "buffer:0.00h") {
		t.Fatalf("expected buffer:0.00h to print unconditionally, got:\n%s", out)
	}
}

func TestCustomTagIsNotFoldedIntoWork(t *testing.T) {
	cfg := config.Default().Accounting
	day := epochAt(2024, 1, 2, 9, 0, 0)
	// "bulgarian" mirrors a real one-off tag from the live data: not
	// "work", not in any plusfor/minusfor/bufferfor list.
	entries := []Entry{
		mkEntry(1, actionAdd, day, 3600, WorkTag),
		mkEntry(2, actionAdd, day, 15, "bulgarian"),
	}

	weeks, _ := buildAndFormat(t, cfg, entries)
	week := weeks[0]
	if got, want := week.Values[WorkTag], int64(3600); got != want {
		t.Fatalf("work = %d, want %d (custom tag must not be folded in)", got, want)
	}
	if week.Buffer != 0 {
		t.Fatalf("buffer = %d, want 0 (custom tag is not a bufferfor category)", week.Buffer)
	}
}

func TestFormatHoursRoundHalfEvenAndItsOneException(t *testing.T) {
	cases := []struct {
		seconds int64
		want    string
	}{
		{0, "0.00"},
		{3600, "1.00"},
		{1800, "0.50"},
		{-3600, "-1.00"},
		{32706, "9.08"}, // exact .5 tie away from zero: rounds to even (8)
		{90, "0.02"},    // exact .5 tie: rounds to even (2)
		{18, "0.01"},    // the one documented exception: smallest tie rounds up, not to even (0)
		{-18, "-0.01"},  // same exception, sign-symmetric
	}
	for _, c := range cases {
		if got := formatHours(c.seconds); got != c.want {
			t.Errorf("formatHours(%d) = %q, want %q", c.seconds, got, c.want)
		}
	}
}
