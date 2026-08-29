package timefmt

import (
	"testing"
	"time"
)

func TestParseDurationValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "bare seconds", input: "3600", want: time.Hour},
		{name: "trimmed bare", input: "  120  ", want: 120 * time.Second},
		{name: "negative bare", input: "-60", want: -60 * time.Second},
		{name: "zero", input: "0", want: 0},
		{name: "minutes", input: "30m", want: 30 * time.Minute},
		{name: "hours", input: "1h", want: time.Hour},
		{name: "mixed", input: "1h30m", want: 90 * time.Minute},
		{name: "fractional hours", input: "2.5h", want: 150 * time.Minute},
		{name: "seconds suffix", input: "45s", want: 45 * time.Second},
		{name: "negative suffixed", input: "-1h", want: -time.Hour},
		{name: "uppercase hours", input: "1H", want: time.Hour},
		{name: "uppercase fractional", input: "2.5H", want: 150 * time.Minute},
		{name: "uppercase minutes", input: "30M", want: 30 * time.Minute},
		{name: "leading fraction", input: ".5h", want: 30 * time.Minute},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDuration(test.input)
			if err != nil {
				t.Fatalf("ParseDuration(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("ParseDuration(%q) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestParseDurationInvalid(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"abc",
		"1h30x",
		"1hxxx",
		"h30m",
		"9223372036854775807",
		"-9223372036854775808",
		"9223372036854775808",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseDuration(input); err == nil {
				t.Fatalf("ParseDuration(%q) error = nil, want error", input)
			}
		})
	}
}
