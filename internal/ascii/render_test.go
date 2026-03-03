package ascii

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestGetFont(t *testing.T) {
	tests := []struct {
		name     string
		fontName string
		want     Font
	}{
		{
			name:     "known font",
			fontName: Rebel,
			want:     fonts[Rebel],
		},
		{
			name:     "unknown font falls back to default",
			fontName: "missing-font",
			want:     fonts[DefaultFont],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFont(tt.fontName)
			if got != tt.want {
				t.Fatalf("GetFont(%q) returned unexpected font", tt.fontName)
			}
		})
	}
}

func TestRenderNumber(t *testing.T) {
	font := fonts[Mono12]

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "digits and colon",
			input:  "10:2",
			expect: lipgloss.JoinHorizontal(lipgloss.Top, font[1], font[0], font[10], font[2]),
		},
		{
			name:   "unsupported runes are empty",
			input:  "1x2",
			expect: lipgloss.JoinHorizontal(lipgloss.Top, font[1], "", font[2]),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderNumber(tt.input, font)
			if got != tt.expect {
				t.Fatalf("RenderNumber(%q) returned unexpected output", tt.input)
			}
		})
	}
}

func TestRenderDigit(t *testing.T) {
	font := fonts[Ansi]

	tests := []struct {
		name  string
		input rune
		want  string
	}{
		{name: "digit", input: '3', want: font[3]},
		{name: "colon", input: ':', want: font[10]},
		{name: "unsupported", input: 'x', want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderDigit(tt.input, font); got != tt.want {
				t.Fatalf("renderDigit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
