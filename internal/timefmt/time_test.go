package timefmt

import (
	"testing"
	"time"
)

func TestParseTimeAtValid(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "today",
			input: "today",
			want:  time.Date(2026, 8, 25, 0, 0, 0, 0, loc),
		},
		{
			name:  "yesterday",
			input: "yesterday",
			want:  time.Date(2026, 8, 24, 0, 0, 0, 0, loc),
		},
		{
			name:  "clock minutes",
			input: "09:00",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, loc),
		},
		{
			name:  "clock with seconds",
			input: "09:00:30",
			want:  time.Date(2026, 8, 25, 9, 0, 30, 0, loc),
		},
		{
			name:  "relative hours ago",
			input: "-2h",
			want:  now.Add(-2 * time.Hour),
		},
		{
			name:  "relative hours ahead",
			input: "+2h",
			want:  now.Add(2 * time.Hour),
		},
		{
			name:  "relative uppercase",
			input: "-2H",
			want:  now.Add(-2 * time.Hour),
		},
		{
			name:  "keyword case",
			input: "Today",
			want:  time.Date(2026, 8, 25, 0, 0, 0, 0, loc),
		},
		{
			name:  "relative mixed",
			input: "-1h30m",
			want:  now.Add(-90 * time.Minute),
		},
		{
			name:  "leading fraction offset",
			input: "-.5h",
			want:  now.Add(-30 * time.Minute),
		},
		{
			name:  "datetime minutes",
			input: "2026-08-25T09:00",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, loc),
		},
		{
			name:  "date only",
			input: "2026-08-20",
			want:  time.Date(2026, 8, 20, 0, 0, 0, 0, loc),
		},
		{
			name:  "datetime with space",
			input: "2026-08-25 09:00:00",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, loc),
		},
		{
			name:  "rfc3339",
			input: "2026-08-25T09:00:00Z",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		},
		{
			name:  "unix epoch",
			input: "1787951450",
			want:  time.Unix(1787951450, 0),
		},
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

func TestParseTimeInvalid(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	inputs := []string{
		"",
		"   ",
		"banana",
		"2024-99-99",
		"25:00",
		"09:60",
		"1h30x",
		"1hxxx",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseTimeAt(input, now); err == nil {
				t.Fatalf("ParseTimeAt(%q) error = nil, want error", input)
			}
		})
	}
}

func TestParseEpoch(t *testing.T) {
	got, err := ParseEpoch("1787951450")
	if err != nil {
		t.Fatalf("ParseEpoch error = %v", err)
	}
	want := time.Unix(1787951450, 0)
	if !got.Equal(want) {
		t.Fatalf("ParseEpoch = %v, want %v", got, want)
	}
}

func TestParseEpochInvalid(t *testing.T) {
	inputs := []string{"", "  ", "abc", "1h", "2026-08-25"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseEpoch(input); err == nil {
				t.Fatalf("ParseEpoch(%q) error = nil, want error", input)
			}
		})
	}
}

func TestParseTimeUsesNow(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 30, 0, 0, time.Local)
	got, err := ParseTimeAt("today", now)
	if err != nil {
		t.Fatalf("ParseTimeAt(today) error = %v", err)
	}
	want := startOfDay(now)
	if !got.Equal(want) {
		t.Fatalf("ParseTimeAt(today) = %v, want %v", got, want)
	}
}

func TestParseTimeWrapper(t *testing.T) {
	if _, err := ParseTime("today"); err != nil {
		t.Fatalf("ParseTime(today) error = %v", err)
	}
}

// TestParseUntilAtDayGranularity is the task 381 regression test: unlike
// ParseTimeAt, ParseUntilAt must resolve today/yesterday/bare-date values to
// the last nanosecond of that day (not its midnight start), so --until stays
// an inclusive upper bound for entries later the same day.
func TestParseUntilAtDayGranularity(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "today",
			input: "today",
			want:  time.Date(2026, 8, 25, 23, 59, 59, 999999999, loc),
		},
		{
			name:  "yesterday",
			input: "yesterday",
			want:  time.Date(2026, 8, 24, 23, 59, 59, 999999999, loc),
		},
		{
			name:  "date only",
			input: "2026-08-20",
			want:  time.Date(2026, 8, 20, 23, 59, 59, 999999999, loc),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseUntilAt(test.input, now)
			if err != nil {
				t.Fatalf("ParseUntilAt(%q) error = %v", test.input, err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("ParseUntilAt(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

// TestParseUntilAtInstantGranularityUnchanged confirms ParseUntilAt leaves
// exact-instant values (clock times, datetimes with a time-of-day, RFC3339,
// relative offsets) untouched -- only day-granularity values shift to
// end-of-day.
func TestParseUntilAtInstantGranularityUnchanged(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 8, 25, 15, 4, 5, 0, loc)

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "clock",
			input: "09:00",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, loc),
		},
		{
			name:  "datetime minutes",
			input: "2026-08-25T09:00",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, loc),
		},
		{
			name:  "rfc3339",
			input: "2026-08-25T09:00:00Z",
			want:  time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
		},
		{
			name:  "relative hours ago",
			input: "-2h",
			want:  now.Add(-2 * time.Hour),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseUntilAt(test.input, now)
			if err != nil {
				t.Fatalf("ParseUntilAt(%q) error = %v", test.input, err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("ParseUntilAt(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestParseUntilWrapper(t *testing.T) {
	if _, err := ParseUntil("today"); err != nil {
		t.Fatalf("ParseUntil(today) error = %v", err)
	}
}
