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

// localLayout pairs a supported local timestamp layout with whether it names
// a whole day (no time-of-day component) or a specific instant. ParseUntilAt
// uses granularity to decide whether a value needs bumping to end-of-day.
type localLayout struct {
	layout      string
	granularity timeGranularity
}

var localLayouts = []localLayout{
	{"2006-01-02", dayGranularity},
	{"2006-01-02T15:04", instantGranularity},
	{"2006-01-02 15:04", instantGranularity},
	{"2006-01-02T15:04:05", instantGranularity},
	{"2006-01-02 15:04:05", instantGranularity},
}

// timeGranularity distinguishes values that name a whole day (today,
// yesterday, bare YYYY-MM-DD) from those that name a specific instant (clock
// times, full datetimes, RFC3339, epoch seconds, relative offsets).
// --until uses this to turn a day-granularity result into that day's last
// instant instead of its first midnight, so the flag's documented "inclusive
// upper bound" promise holds the same way it already does for --at/--since
// and for positional ranges (ParseRange's End.Add(-1ns)).
type timeGranularity int

const (
	instantGranularity timeGranularity = iota
	dayGranularity
)

// ParseTime converts --at timestamp text into a time value using time.Now().
func ParseTime(value string) (time.Time, error) {
	return ParseTimeAt(value, time.Now())
}

// ParseTimeAt behaves like ParseTime but uses now for relative values
// (today, yesterday, clock times, and signed duration offsets such as -2h).
// It always returns the start of a day-granularity value (midnight); callers
// that need an inclusive upper bound instead (--until) should use
// ParseUntilAt.
func ParseTimeAt(value string, now time.Time) (time.Time, error) {
	t, _, err := parseTimeAtGranular(value, now)
	return t, err
}

// ParseUntil converts --until text into an inclusive upper-bound time value
// using time.Now().
func ParseUntil(value string) (time.Time, error) {
	return ParseUntilAt(value, time.Now())
}

// ParseUntilAt behaves like ParseTimeAt but treats a day-granularity result
// (today, yesterday, or a bare YYYY-MM-DD date) as spanning that whole day,
// returning its last nanosecond instead of its midnight start. Instant
// values (clock times, datetimes with a time-of-day, RFC3339, epoch seconds,
// relative offsets) are returned unchanged, since they already name a
// specific moment and must keep meaning exactly that moment.
func ParseUntilAt(value string, now time.Time) (time.Time, error) {
	t, granularity, err := parseTimeAtGranular(value, now)
	if err != nil {
		return time.Time{}, err
	}
	if granularity == dayGranularity {
		return endOfDay(t), nil
	}
	return t, nil
}

// parseTimeAtGranular is the shared implementation behind ParseTimeAt and
// ParseUntilAt: it parses value the same way for both, but also reports
// whether the match was day-granularity (so ParseUntilAt can bump it to
// end-of-day) or already names a specific instant (so it must not be
// touched).
func parseTimeAtGranular(value string, now time.Time) (time.Time, timeGranularity, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, instantGranularity, errors.New("time value must not be empty")
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "today":
		return startOfDay(now), dayGranularity, nil
	case "yesterday":
		return startOfDay(now.AddDate(0, 0, -1)), dayGranularity, nil
	}

	if hour, min, sec, ok := parseClock(trimmed); ok {
		y, m, d := now.Date()
		return time.Date(y, m, d, hour, min, sec, 0, now.Location()), instantGranularity, nil
	}

	if bareIntegerPattern.MatchString(trimmed) {
		t, err := ParseEpoch(trimmed)
		return t, instantGranularity, err
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, instantGranularity, nil
	}

	for _, ll := range localLayouts {
		parsed, err := time.ParseInLocation(ll.layout, trimmed, now.Location())
		if err == nil {
			return parsed, ll.granularity, nil
		}
	}

	lowered := strings.ToLower(trimmed)
	if durationLikePattern.MatchString(lowered) {
		offset, err := time.ParseDuration(lowered)
		if err != nil {
			return time.Time{}, instantGranularity, fmt.Errorf("parse relative time %q: %w", value, err)
		}
		return now.Add(offset), instantGranularity, nil
	}

	return time.Time{}, instantGranularity, fmt.Errorf("unsupported time format %q", value)
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

// endOfDay returns the last nanosecond of value's day: the next day's
// midnight minus one nanosecond. Entries carry integer-second epochs, so
// stepping back one nanosecond from the next midnight includes every instant
// that legitimately belongs to value's day without reaching into the next
// one -- the same technique buildFilter already uses to turn ParseRange's
// exclusive End into an inclusive upper bound.
func endOfDay(value time.Time) time.Time {
	return startOfDay(value).AddDate(0, 0, 1).Add(-time.Nanosecond)
}
