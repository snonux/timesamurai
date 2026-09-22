package timefmt

import (
	"testing"
	"time"
)

// TestParseTimeAtRelativeDayPhrases covers the human phrases task o9 asks
// for. now is 2026-08-25 15:04 EET, a Tuesday (verified via now.Weekday()
// assertions below rather than trust in a calendar).
func TestParseTimeAtRelativeDayPhrases(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)
	if got := now.Weekday(); got != time.Tuesday {
		t.Fatalf("test assumption broken: 2026-08-25 is %v, want Tuesday", got)
	}

	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{name: "now", input: "now", want: now},
		{name: "now case-insensitive", input: "NOW", want: now},
		{name: "tomorrow", input: "tomorrow", want: day(2026, time.August, 26)},
		{name: "tomorrow case", input: "Tomorrow", want: day(2026, time.August, 26)},
		{name: "bare weekday forward", input: "friday", want: day(2026, time.August, 28)},
		{name: "bare weekday wraps week", input: "monday", want: day(2026, time.August, 31)},
		{name: "next wednesday", input: "next wednesday", want: day(2026, time.August, 26)},
		{name: "next monday", input: "next monday", want: day(2026, time.August, 31)},
		{name: "next tuesday skips today", input: "next tuesday", want: day(2026, time.September, 1)},
		{name: "next weekday case", input: "NEXT FRIDAY", want: day(2026, time.August, 28)},
		{name: "last monday", input: "last monday", want: day(2026, time.August, 24)},
		{name: "last sunday", input: "last sunday", want: day(2026, time.August, 23)},
		{ // Tuesday is today: strictly-in-the-past rule means last week.
			name: "last tuesday skips today", input: "last tuesday", want: day(2026, time.August, 18),
		},
		{name: "this wednesday", input: "this wednesday", want: day(2026, time.August, 26)},
		{name: "one day ago", input: "1 day ago", want: day(2026, time.August, 24)},
		{name: "two days ago", input: "two days ago", want: day(2026, time.August, 23)},
		{name: "three days ago", input: "three days ago", want: day(2026, time.August, 22)},
		{name: "a week ago", input: "1 week ago", want: day(2026, time.August, 18)},
		{name: "two weeks ago", input: "two weeks ago", want: day(2026, time.August, 11)},
		{name: "one month ago", input: "one month ago", want: day(2026, time.July, 25)},
		{name: "one year ago", input: "one year ago", want: day(2025, time.August, 25)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseTimeAt(test.input, now)
			if err != nil {
				t.Fatalf("ParseTimeAt(%q) error = %v", test.input, err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("ParseTimeAt(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

// TestParseUntilAtRelativeDayPhrases pins the granularity contract: phrase
// results are day-granularity (bumped to end-of-day by --until) except
// "now", which is an instant and must pass through unchanged.
func TestParseUntilAtRelativeDayPhrases(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)

	got, err := ParseUntilAt("next monday", now)
	if err != nil {
		t.Fatalf("ParseUntilAt(next monday): %v", err)
	}
	want := time.Date(2026, 8, 31, 23, 59, 59, 999999999, loc)
	if !got.Equal(want) {
		t.Fatalf("ParseUntilAt(next monday) = %v, want %v", got, want)
	}

	got, err = ParseUntilAt("now", now)
	if err != nil {
		t.Fatalf("ParseUntilAt(now): %v", err)
	}
	if !got.Equal(now) {
		t.Fatalf("ParseUntilAt(now) = %v, want %v (instant must not be bumped)", got, now)
	}
}

// TestParseTimeAtPhraseInvalid keeps the boundary honest: near-misses and
// out-of-scope grammar must fail loudly, never silently parse to today.
func TestParseTimeAtPhraseInvalid(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	inputs := []string{
		"nextmonth",
		"next",
		"last",
		"mondayish",
		"nextmonday",
		"funday",
		"days ago",
		"2 days",
		"in 2 days",
		"two days from now",
		"zero days ago",
		"thirteen days ago",
		"next monday at noon",
		"2 fortnights ago",
	}
	for _, input := range inputs {
		if got, err := ParseTimeAt(input, now); err == nil {
			t.Errorf("ParseTimeAt(%q) = %v, want unsupported-format error", input, got)
		}
	}
}
// TestParseTimeAtPhraseZeroAgoRejected pins the symmetry fix: "0 days ago"
// must fail the same way the spelled-out "zero days ago" does.
func TestParseTimeAtPhraseZeroAgoRejected(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for _, input := range []string{"0 days ago", "0 day ago", "0 weeks ago"} {
		if got, err := ParseTimeAt(input, now); err == nil {
			t.Errorf("ParseTimeAt(%q) = %v, want unsupported-format error", input, got)
		}
	}
}
