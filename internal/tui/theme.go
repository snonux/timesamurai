package tui

import (
	"math/rand/v2"
	"strconv"
)

// Theme defines the shared color palette used across the TUI.
type Theme struct {
	HeaderFG   string
	SelectedFG string
	SelectedBG string
	RowFG      string
	RowBG      string
	StatusFG   string
	StatusBG   string
	SearchFG   string
	SearchBG   string
}

// DefaultTheme returns the baseline theme inspired by Task Samurai.
func DefaultTheme() Theme {
	return Theme{
		HeaderFG:   "205",
		SelectedFG: "229",
		SelectedBG: "57",
		RowFG:      "15",
		RowBG:      "236",
		StatusFG:   "229",
		StatusBG:   "57",
		SearchFG:   "21",
		SearchBG:   "226",
	}
}

// RandomTheme returns a randomized high-contrast palette for disco mode.
func RandomTheme() Theme {
	theme := Theme{
		HeaderFG:   randomColor(),
		SelectedBG: randomColor(),
		RowBG:      randomColor(),
		StatusBG:   randomColor(),
		SearchBG:   randomColor(),
	}

	theme.SelectedFG = contrastColor(theme.SelectedBG)
	theme.RowFG = contrastColor(theme.RowBG)
	theme.StatusFG = contrastColor(theme.StatusBG)
	theme.SearchFG = contrastColor(theme.SearchBG)
	return theme
}

func randomColor() string {
	return strconv.Itoa(rand.IntN(256))
}

func contrastColor(background string) string {
	value, err := strconv.Atoi(background)
	if err != nil {
		return "15"
	}

	if xtermBrightness(value) > 128 {
		return "0"
	}
	return "15"
}

func xtermBrightness(index int) float64 {
	red, green, blue := xtermRGB(index)
	return 0.299*float64(red) + 0.587*float64(green) + 0.114*float64(blue)
}

func xtermRGB(index int) (int, int, int) {
	if index < 0 {
		index = 0
	}
	if index > 255 {
		index = 255
	}

	if index < 16 {
		table := [16][3]int{
			{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
			{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
			{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		rgb := table[index]
		return rgb[0], rgb[1], rgb[2]
	}

	if index <= 231 {
		index -= 16
		red := (index / 36) * 51
		green := (index % 36 / 6) * 51
		blue := (index % 6) * 51
		return red, green, blue
	}

	gray := (index-232)*10 + 8
	return gray, gray, gray
}
