package timefmt

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var yearMonthPattern = regexp.MustCompile(`^(\d{4})-(\d{2})$`)

// Range is a half-open time interval [Start, End).
type Range struct {
	// Start is the inclusive lower bound of the range.
	Start time.Time
	// End is the exclusive upper bound of the range.
	End time.Time
}

// ParseRange converts range text into a half-open interval using time.Now().
func ParseRange(value string) (Range, error) {
	return ParseRangeAt(value, time.Now())
}

// ParseRangeAt behaves like ParseRange but uses now for relative named ranges.
// Supported forms: today, yesterday, week, lastweek, month, YYYY-MM, and
// inclusive date..date spans (start-of-day through end-of-end-date).
func ParseRangeAt(value string, now time.Time) (Range, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return Range{}, errors.New("range must not be empty")
	}

	lower := strings.ToLower(trimmed)
	switch lower {
	case "today":
		start := startOfDay(now)
		return Range{Start: start, End: start.AddDate(0, 0, 1)}, nil
	case "yesterday":
		start := startOfDay(now.AddDate(0, 0, -1))
		return Range{Start: start, End: start.AddDate(0, 0, 1)}, nil
	case "week":
		start := startOfISOWeek(now)
		return Range{Start: start, End: start.AddDate(0, 0, 7)}, nil
	case "lastweek":
		start := startOfISOWeek(now).AddDate(0, 0, -7)
		return Range{Start: start, End: start.AddDate(0, 0, 7)}, nil
	case "month":
		start := startOfMonth(now)
		return Range{Start: start, End: start.AddDate(0, 1, 0)}, nil
	}

	if start, end, ok, err := parseYearMonth(trimmed, now.Location()); ok {
		return Range{Start: start, End: end}, nil
	} else if err != nil {
		return Range{}, err
	}

	if start, end, ok, err := parseDateSpan(trimmed, now.Location()); ok {
		return Range{Start: start, End: end}, nil
	} else if err != nil {
		return Range{}, err
	}

	return Range{}, fmt.Errorf("unsupported range %q", value)
}

func parseYearMonth(value string, loc *time.Location) (start, end time.Time, ok bool, err error) {
	parts := yearMonthPattern.FindStringSubmatch(value)
	if parts == nil {
		return time.Time{}, time.Time{}, false, nil
	}
	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse year-month %q: %w", value, err)
	}
	month, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse year-month %q: %w", value, err)
	}
	if month < 1 || month > 12 {
		return time.Time{}, time.Time{}, false, fmt.Errorf("invalid month in range %q", value)
	}
	start = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0), true, nil
}

func parseDateSpan(value string, loc *time.Location) (start, end time.Time, ok bool, err error) {
	left, right, found := strings.Cut(value, "..")
	if !found {
		return time.Time{}, time.Time{}, false, nil
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return time.Time{}, time.Time{}, false, fmt.Errorf("invalid date range %q", value)
	}
	if strings.Contains(right, "..") {
		return time.Time{}, time.Time{}, false, fmt.Errorf("invalid date range %q", value)
	}

	start, err = parseDateOnly(left, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse range start %q: %w", left, err)
	}
	endDay, err := parseDateOnly(right, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse range end %q: %w", right, err)
	}
	end = endDay.AddDate(0, 0, 1)
	if !end.After(start) {
		return time.Time{}, time.Time{}, false, fmt.Errorf("range end before start in %q", value)
	}
	return start, end, true, nil
}

func parseDateOnly(value string, loc *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func startOfISOWeek(value time.Time) time.Time {
	day := startOfDay(value)
	// ISO weeks start on Monday. Convert Sunday=0 to 7 so offset is days since Monday.
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return day.AddDate(0, 0, -(weekday - 1))
}

func startOfMonth(value time.Time) time.Time {
	year, month, _ := value.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, value.Location())
}
