package duration

import (
	"testing"
	"time"
)

func TestParseValidDurations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{
			name:  "go-style mixed",
			input: "1h30m",
			want:  90 * time.Minute,
		},
		{
			name:  "go-style minutes",
			input: "30m",
			want:  30 * time.Minute,
		},
		{
			name:  "go-style hours",
			input: "8h",
			want:  8 * time.Hour,
		},
		{
			name:  "bare integer seconds",
			input: "3600",
			want:  time.Hour,
		},
		{
			name:  "trimmed bare integer",
			input: "  120  ",
			want:  120 * time.Second,
		},
		{
			name:  "negative bare integer",
			input: "-60",
			want:  -60 * time.Second,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(test.input)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("Parse(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestParseInvalidDurations(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"abc",
		"1h30x",
		"9223372036854775807",
		"-9223372036854775808",
	}

	for _, input := range tests {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(input); err == nil {
				t.Fatalf("Parse(%q) error = nil, want error", input)
			}
		})
	}
}
