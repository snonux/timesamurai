package viinput

import "testing"

func TestWordForward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		cursor int
		want   int
	}{
		{name: "empty", input: "", cursor: 0, want: 0},
		{name: "within word", input: "alpha beta", cursor: 0, want: 6},
		{name: "from whitespace", input: "alpha beta", cursor: 5, want: 6},
		{name: "from punctuation", input: "alpha, beta", cursor: 5, want: 7},
		{name: "at end", input: "alpha beta", cursor: 10, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := wordForward([]rune(tt.input), tt.cursor); got != tt.want {
				t.Fatalf("wordForward(%q, %d) = %d, want %d", tt.input, tt.cursor, got, tt.want)
			}
		})
	}
}

func TestWordBackward(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		cursor int
		want   int
	}{
		{name: "empty", input: "", cursor: 0, want: 0},
		{name: "from middle of next word", input: "alpha beta", cursor: 7, want: 6},
		{name: "from whitespace", input: "alpha beta", cursor: 6, want: 0},
		{name: "from end", input: "alpha beta", cursor: 10, want: 6},
		{name: "single word", input: "alpha", cursor: 5, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := wordBackward([]rune(tt.input), tt.cursor); got != tt.want {
				t.Fatalf("wordBackward(%q, %d) = %d, want %d", tt.input, tt.cursor, got, tt.want)
			}
		})
	}
}

func TestWordEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		cursor int
		want   int
	}{
		{name: "empty", input: "", cursor: 0, want: 0},
		{name: "within word", input: "alpha beta", cursor: 0, want: 4},
		{name: "from whitespace", input: "alpha beta", cursor: 5, want: 9},
		{name: "from punctuation", input: "alpha, beta", cursor: 5, want: 10},
		{name: "at end", input: "alpha beta", cursor: 10, want: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := wordEnd([]rune(tt.input), tt.cursor); got != tt.want {
				t.Fatalf("wordEnd(%q, %d) = %d, want %d", tt.input, tt.cursor, got, tt.want)
			}
		})
	}
}
