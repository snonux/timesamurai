package timefmt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var unixTimestampPattern = regexp.MustCompile(`^[+-]?\d+$`)

var localLayouts = []string{
	"2006-01-02",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// Parse converts timestamp/date text into a time value.
func Parse(value string) (time.Time, error) {
	return ParseAt(value, time.Now())
}

// ParseAt behaves like Parse but uses a provided "now" reference for relative values.
func ParseAt(value string, now time.Time) (time.Time, error) {
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

	if unixTimestampPattern.MatchString(trimmed) {
		seconds, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse unix timestamp %q: %w", value, err)
		}
		return time.Unix(seconds, 0), nil
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

	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

func startOfDay(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}
