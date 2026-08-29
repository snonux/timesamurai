package timefmt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	clockPattern = regexp.MustCompile(`^(\d{1,2}):(\d{2})(?::(\d{2}))?$`)
	// Leading optional fraction (.5h) or digits; units case-folded before match.
	durationLikePattern = regexp.MustCompile(`^[+-]?((\d+(\.\d*)?|\.\d+)[hms])+$`)
)

var localLayouts = []string{
	"2006-01-02",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// ParseTime converts --at timestamp text into a time value using time.Now().
func ParseTime(value string) (time.Time, error) {
	return ParseTimeAt(value, time.Now())
}

// ParseTimeAt behaves like ParseTime but uses now for relative values
// (today, yesterday, clock times, and signed duration offsets such as -2h).
func ParseTimeAt(value string, now time.Time) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, errors.New("time value must not be empty")
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "today":
		return startOfDay(now), nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1)), nil
	}

	if hour, min, sec, ok := parseClock(trimmed); ok {
		y, m, d := now.Date()
		return time.Date(y, m, d, hour, min, sec, 0, now.Location()), nil
	}

	if bareIntegerPattern.MatchString(trimmed) {
		return ParseEpoch(trimmed)
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, nil
	}

	for _, layout := range localLayouts {
		parsed, err := time.ParseInLocation(layout, trimmed, now.Location())
		if err == nil {
			return parsed, nil
		}
	}

	lowered := strings.ToLower(trimmed)
	if durationLikePattern.MatchString(lowered) {
		offset, err := time.ParseDuration(lowered)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse relative time %q: %w", value, err)
		}
		return now.Add(offset), nil
	}

	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

// ParseEpoch converts a legacy --epoch unix-seconds string into a time value.
func ParseEpoch(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, errors.New("epoch value must not be empty")
	}
	if !bareIntegerPattern.MatchString(trimmed) {
		return time.Time{}, fmt.Errorf("invalid epoch %q", value)
	}
	seconds, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse epoch %q: %w", value, err)
	}
	return time.Unix(seconds, 0), nil
}

func parseClock(value string) (hour, min, sec int, ok bool) {
	parts := clockPattern.FindStringSubmatch(value)
	if parts == nil {
		return 0, 0, 0, false
	}
	hour, err := strconv.Atoi(parts[1])
	if err != nil || hour > 23 {
		return 0, 0, 0, false
	}
	min, err = strconv.Atoi(parts[2])
	if err != nil || min > 59 {
		return 0, 0, 0, false
	}
	if parts[3] != "" {
		sec, err = strconv.Atoi(parts[3])
		if err != nil || sec > 59 {
			return 0, 0, 0, false
		}
	}
	return hour, min, sec, true
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}
