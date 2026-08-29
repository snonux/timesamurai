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
			name:  "relative mixed",
			input: "-1h30m",
			want:  now.Add(-90 * time.Minute),
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
	got, err := ParseTime("today")
	if err != nil {
		t.Fatalf("ParseTime(today) error = %v", err)
	}
	want := startOfDay(time.Now())
	if !got.Equal(want) {
		t.Fatalf("ParseTime(today) = %v, want %v", got, want)
	}
}
