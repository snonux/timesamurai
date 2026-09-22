package timefmt

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// This file adds GNU-date-style human phrases to --at / ParseTime (task o9):
// "next monday", "last friday", "this saturday", bare weekday names,
// "tomorrow", "now", and "<n> <unit> ago" forms such as "two days ago".
//
// Scope is deliberately narrow: exactly the phrases the task names plus the
// obvious siblings, all matching one of three small patterns below. Anything
// else (GNU date's full grammar — "next month at noon", timezone suffixes,
// "in 3 days", …) stays unsupported so a typo'd phrase fails loudly with
// "unsupported time format" instead of silently landing on the wrong day.
//
// Two deliberate semantics choices, both documented here rather than
// silently surprising: (1) "this <weekday>" means that weekday of the
// current Monday-based (ISO) week — which can be today or the recent past —
// whereas GNU date treats "this" as "next, today included"; (2) month/year
// "ago" arithmetic uses time.Time.AddDate, which normalizes overflow the
// way Go's stdlib does: e.g. "one month ago" from 2025-03-30 lands on
// 2025-03-02 (Feb 30 rolls forward) rather than GNU date's clamped
// 2025-02-28. It never lands in the future, so downstream day-off/entry
// timestamps stay sane.

var (
	// weekdayPattern matches an optional next/last/this qualifier followed
	// by an English weekday name, e.g. "next monday", "last friday",
	// "this saturday", or a bare "friday". The qualifier requires real
	// whitespace so "nextmonday" fails loudly instead of silently reading
	// as "next monday". Case-insensitive: callers pass an already-
	// lowercased string.
	weekdayPattern = regexp.MustCompile(
		`^(?:(next|last|this)\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday)$`)

	// agoPattern matches "<n> <unit> ago" with a digit or spelled-out
	// number 1..12 and unit day/week/month/year in singular or plural,
	// e.g. "two days ago", "1 week ago", "three months ago".
	agoPattern = regexp.MustCompile(
		`^(\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)\s+` +
			`(day|days|week|weeks|month|months|year|years)\s+ago$`)

	numberWords = map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
		"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
		"eleven": 11, "twelve": 12,
	}

	weekdayNames = map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
	}
)

// parseRelativeDayPhrase recognizes the human phrases task o9 asks for and
// returns their time in now's location. ok=false means the value is not a
// relative day phrase and parsing should continue with the remaining
// formats; a non-nil err with ok=true is not possible. All phrase results
// are day-granularity (midnight of the named day) except "now", which is an
// instant, so --until's inclusive end-of-day bump keeps working for phrases
// exactly as it already does for "today" and "yesterday".
func parseRelativeDayPhrase(lower string, now time.Time) (t time.Time, granularity timeGranularity, ok bool) {
	switch lower {
	case "now":
		return now, instantGranularity, true
	case "tomorrow":
		return startOfDay(now.AddDate(0, 0, 1)), dayGranularity, true
	}

	if m := weekdayPattern.FindStringSubmatch(lower); m != nil {
		target := weekdayNames[m[2]]
		today := now.Weekday()
		var deltaDays int
		switch m[1] {
		case "last":
			// Most recent occurrence strictly before today; if today is
			// the named weekday, "last" still means the previous week's.
			back := (int(today-target) + 7) % 7
			if back == 0 {
				back = 7
			}
			deltaDays = -back
		case "this":
			// The named weekday of the current Monday-based (ISO) week.
			// May land on today itself ("this tuesday" on a Tuesday) or in
			// the recent past ("this monday" on a Tuesday = yesterday).
			daysSinceMonday := (int(today) + 6) % 7
			deltaDays = (int(target)+6)%7 - daysSinceMonday
		default:
			// "next <weekday>" and a bare weekday both mean the coming
			// occurrence strictly in the future: today never qualifies,
			// so "next monday" on a Monday is a week out (documented, and
			// close enough to GNU date's behaviour for tracker entries).
			deltaDays = (int(target-today) + 7) % 7
			if deltaDays == 0 {
				deltaDays = 7
			}
		}
		return startOfDay(now.AddDate(0, 0, deltaDays)), dayGranularity, true
	}

	if m := agoPattern.FindStringSubmatch(lower); m != nil {
		count := numberWords[m[1]]
		if count == 0 {
			var err error
			if count, err = strconv.Atoi(m[1]); err != nil {
				return time.Time{}, instantGranularity, false
			}
		}
		if count == 0 {
			// "0 days ago" reads as a mistake; fail loudly the same way
			// the spelled-out "zero days ago" already does.
			return time.Time{}, instantGranularity, false
		}
		unit := strings.TrimSuffix(m[2], "s")
		var t time.Time
		switch unit {
		case "day":
			t = now.AddDate(0, 0, -count)
		case "week":
			t = now.AddDate(0, 0, -7*count)
		case "month":
			t = now.AddDate(0, -count, 0)
		case "year":
			t = now.AddDate(-count, 0, 0)
		}
		return startOfDay(t), dayGranularity, true
	}

	return time.Time{}, instantGranularity, false
}
