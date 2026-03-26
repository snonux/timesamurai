package viinput

import "unicode"

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func wordForward(runes []rune, cursor int) int {
	if len(runes) == 0 {
		return 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(runes) {
		return len(runes)
	}

	pos := cursor
	if isWordChar(runes[pos]) {
		for pos < len(runes) && isWordChar(runes[pos]) {
			pos++
		}
	} else {
		for pos < len(runes) && !isWordChar(runes[pos]) {
			pos++
		}
	}
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	return pos
}

func wordBackward(runes []rune, cursor int) int {
	if len(runes) == 0 || cursor <= 0 {
		return 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	pos := cursor - 1
	for pos >= 0 && !isWordChar(runes[pos]) {
		pos--
	}
	for pos >= 0 && isWordChar(runes[pos]) {
		pos--
	}
	return pos + 1
}

func wordEnd(runes []rune, cursor int) int {
	if len(runes) == 0 {
		return 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(runes) {
		return len(runes)
	}

	pos := cursor
	for pos < len(runes) && !isWordChar(runes[pos]) {
		pos++
	}
	if pos >= len(runes) {
		return len(runes)
	}
	for pos+1 < len(runes) && isWordChar(runes[pos+1]) {
		pos++
	}
	return pos
}
