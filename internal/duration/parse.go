package duration

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var bareIntegerPattern = regexp.MustCompile(`^[+-]?\d+$`)

const (
	maxInt64 = int64(^uint64(0) >> 1)
	minInt64 = -maxInt64 - 1
)

// Parse converts duration text into time.Duration.
// Go-style durations (e.g. "1h30m") are supported, and bare integers are seconds.
func Parse(value string) (time.Duration, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, errors.New("duration must not be empty")
	}

	if bareIntegerPattern.MatchString(trimmed) {
		seconds, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse seconds %q: %w", value, err)
		}
		if seconds > maxInt64/int64(time.Second) || seconds < minInt64/int64(time.Second) {
			return 0, fmt.Errorf("duration seconds %q overflows time.Duration", value)
		}
		return time.Duration(seconds) * time.Second, nil
	}

	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", value, err)
	}

	return parsed, nil
}
