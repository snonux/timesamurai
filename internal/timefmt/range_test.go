package timefmt

import (
	"testing"
	"time"
)

func TestParseRangeAtValid(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	// Tuesday 2026-08-25 — ISO week 35 starts Monday 2026-08-24.
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)

	tests := []struct {
		name  string
		input string
		start time.Time
		end   time.Time
	}{
		{
			name:  "today",
			input: "today",
			start: time.Date(2026, 8, 25, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 26, 0, 0, 0, 0, loc),
		},
		{
			name:  "yesterday",
			input: "yesterday",
			start: time.Date(2026, 8, 24, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 25, 0, 0, 0, 0, loc),
		},
		{
			name:  "week",
			input: "week",
			start: time.Date(2026, 8, 24, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 31, 0, 0, 0, 0, loc),
		},
		{
			name:  "lastweek",
			input: "lastweek",
			start: time.Date(2026, 8, 17, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 24, 0, 0, 0, 0, loc),
		},
		{
			name:  "month",
			input: "month",
			start: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		},
		{
			name:  "year-month",
			input: "2026-08",
			start: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 9, 1, 0, 0, 0, 0, loc),
		},
		{
			name:  "date span",
			input: "2026-08-01..2026-08-07",
			start: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 8, 0, 0, 0, 0, loc),
		},
		{
			name:  "date span with spaces",
			input: "2026-08-01 .. 2026-08-07",
			start: time.Date(2026, 8, 1, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 8, 0, 0, 0, 0, loc),
		},
		{
			name:  "keyword case",
			input: "Week",
			start: time.Date(2026, 8, 24, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 31, 0, 0, 0, 0, loc),
		},
		{
			name:  "single day span",
			input: "2026-08-25..2026-08-25",
			start: time.Date(2026, 8, 25, 0, 0, 0, 0, loc),
			end:   time.Date(2026, 8, 26, 0, 0, 0, 0, loc),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRangeAt(test.input, now)
			if err != nil {
				t.Fatalf("ParseRangeAt(%q) error = %v", test.input, err)
			}
			if !got.Start.Equal(test.start) || !got.End.Equal(test.end) {
				t.Fatalf("ParseRangeAt(%q) = [%v, %v), want [%v, %v)",
					test.input, got.Start, got.End, test.start, test.end)
			}
		})
	}
}

func TestParseRangeAtWeekOnSunday(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	// Sunday belongs to the ISO week that started the previous Monday.
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, loc)
	got, err := ParseRangeAt("week", now)
	if err != nil {
		t.Fatalf("ParseRangeAt(week) error = %v", err)
	}
	wantStart := time.Date(2026, 8, 24, 0, 0, 0, 0, loc)
	wantEnd := time.Date(2026, 8, 31, 0, 0, 0, 0, loc)
	if !got.Start.Equal(wantStart) || !got.End.Equal(wantEnd) {
		t.Fatalf("ParseRangeAt(week on Sunday) = [%v, %v), want [%v, %v)",
			got.Start, got.End, wantStart, wantEnd)
	}
}

func TestParseRangeInvalid(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	inputs := []string{
		"",
		"   ",
		"banana",
		"2026-13",
		"2026-00",
		"2026-08-01..",
		"..2026-08-07",
		"2026-08-10..2026-08-01",
		"2026-08-01...2026-08-07",
		"2026-99-99..2026-08-01",
		"not-a-date..2026-08-01",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseRangeAt(input, now); err == nil {
				t.Fatalf("ParseRangeAt(%q) error = nil, want error", input)
			}
		})
	}
}

func TestParseRangeUsesNow(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 30, 0, 0, time.Local)
	got, err := ParseRangeAt("today", now)
	if err != nil {
		t.Fatalf("ParseRangeAt(today) error = %v", err)
	}
	start := startOfDay(now)
	if !got.Start.Equal(start) || !got.End.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("ParseRangeAt(today) = [%v, %v), want today", got.Start, got.End)
	}
}

func TestParseRangeWrapper(t *testing.T) {
	if _, err := ParseRange("today"); err != nil {
		t.Fatalf("ParseRange(today) error = %v", err)
	}
}
