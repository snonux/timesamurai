package timefmt

import (
	"testing"
	"time"
)

func TestParseAtRelativeValues(t *testing.T) {
	now := time.Date(2026, 3, 3, 15, 4, 5, 0, time.FixedZone("EET", 2*3600))

	today, err := ParseAt("today", now)
	if err != nil {
		t.Fatalf("ParseAt(today) error = %v", err)
	}
	wantToday := time.Date(2026, 3, 3, 0, 0, 0, 0, now.Location())
	if !today.Equal(wantToday) {
		t.Fatalf("ParseAt(today) = %v, want %v", today, wantToday)
	}

	yesterday, err := ParseAt("yesterday", now)
	if err != nil {
		t.Fatalf("ParseAt(yesterday) error = %v", err)
	}
	wantYesterday := time.Date(2026, 3, 2, 0, 0, 0, 0, now.Location())
	if !yesterday.Equal(wantYesterday) {
		t.Fatalf("ParseAt(yesterday) = %v, want %v", yesterday, wantYesterday)
	}
}

func TestParseUnixTimestamp(t *testing.T) {
	got, err := ParseAt("1714424400", time.Now())
	if err != nil {
		t.Fatalf("ParseAt(unix) error = %v", err)
	}

	want := time.Unix(1714424400, 0)
	if !got.Equal(want) {
		t.Fatalf("ParseAt(unix) = %v, want %v", got, want)
	}
}

func TestParseISOValues(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	now := time.Date(2026, 3, 3, 12, 0, 0, 0, loc)

	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "date only",
			input: "2024-01-15",
			want:  time.Date(2024, 1, 15, 0, 0, 0, 0, loc),
		},
		{
			name:  "datetime minutes",
			input: "2024-01-15T09:30",
			want:  time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
		},
		{
			name:  "datetime with seconds and space",
			input: "2024-01-15 09:30:45",
			want:  time.Date(2024, 1, 15, 9, 30, 45, 0, loc),
		},
		{
			name:  "rfc3339",
			input: "2024-01-15T09:30:00Z",
			want:  time.Date(2024, 1, 15, 9, 30, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseAt(test.input, now)
			if err != nil {
				t.Fatalf("ParseAt(%q) error = %v", test.input, err)
			}
			if !got.Equal(test.want) {
				t.Fatalf("ParseAt(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestParseInvalidValues(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"banana",
		"2024-99-99",
	}

	for _, input := range inputs {
		input := input
		t.Run(input, func(t *testing.T) {
			if _, err := ParseAt(input, time.Now()); err == nil {
				t.Fatalf("ParseAt(%q) error = nil, want error", input)
			}
		})
	}
}
