// Package ascii provides ASCII art rendering for timer display.
// This code is adapted from https://github.com/Bahaaio/pomo
// Copyright (c) 2025 Bahaa El Deen Mohamed
// Licensed under the MIT License
package ascii

import (
	"charm.land/lipgloss/v2"
)

// RenderNumber renders a string of digits as ASCII art using the specified font.
// The number parameter should contain only digits (0-9) and colons (:).
// Returns the ASCII art representation as a single string with properly aligned characters.
func RenderNumber(number string, font Font) string {
	digits := make([]string, 0, len(number))

	for _, digit := range number {
		digits = append(digits, renderDigit(digit, font))
	}

	asciiDigits := lipgloss.JoinHorizontal(lipgloss.Top, digits...)
	return asciiDigits
}

// GetFont returns the font with the given name.
// If the font doesn't exist, it returns the default font.
func GetFont(fontName string) Font {
	if font, exists := fonts[fontName]; exists {
		return font
	}

	return fonts[DefaultFont]
}

// renderDigit converts a single digit character to its ASCII art representation.
// Supports digits 0-9 and colon (:). Returns empty string for unsupported characters.
func renderDigit(digit rune, font Font) string {
	if digit == ':' {
		return font[len(font)-1]
	}

	if digit < '0' || digit > '9' {
		return ""
	}

	return font[digit-'0']
}
