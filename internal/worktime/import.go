package worktime

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/timesamurai/internal/timefmt"
)

const importSecondsPerHour = 3600

// ImportLine is a parsed report line with hourly values by category.
type ImportLine struct {
	When       time.Time
	WorkHours  float64
	LunchHours float64
	OffHours   float64
}

// ImportReport imports report-format lines into the worktime database.
func ImportReport(dbDir, hostname string, report io.Reader) (imported int, err error) {
	scanner := bufio.NewScanner(report)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "lunch:") {
			continue
		}

		parsed, parseErr := ParseImportLine(line)
		if parseErr != nil {
			return imported, parseErr
		}

		workSeconds := hoursToSeconds(parsed.WorkHours)
		lunchSeconds := hoursToSeconds(parsed.LunchHours)
		offSeconds := hoursToSeconds(parsed.OffHours)

		if lunchSeconds > 0 {
			workSeconds += lunchSeconds
		}

		if workSeconds > 0 {
			if _, addErr := Add(dbDir, hostname, "work", time.Duration(workSeconds)*time.Second, parsed.When, ""); addErr != nil {
				return imported, addErr
			}
			imported++
		}
		if lunchSeconds > 0 {
			if _, addErr := Add(dbDir, hostname, "lunch", time.Duration(lunchSeconds)*time.Second, parsed.When, ""); addErr != nil {
				return imported, addErr
			}
			imported++
		}
		if offSeconds > 0 {
			if _, addErr := Add(dbDir, hostname, "off", time.Duration(offSeconds)*time.Second, parsed.When, ""); addErr != nil {
				return imported, addErr
			}
			imported++
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return imported, scanErr
	}

	return imported, nil
}

// ParseImportLine parses one report-format day line.
func ParseImportLine(line string) (ImportLine, error) {
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return ImportLine{}, fmt.Errorf("unsupported import line: %q", line)
	}

	dateToken := strings.TrimSuffix(fields[1], ":")
	workToken := fields[2]
	lunchToken := fields[4]
	offToken := fields[6]

	when, err := ParseImportDate(dateToken)
	if err != nil {
		return ImportLine{}, err
	}

	workHours, err := parseHourToken(workToken)
	if err != nil {
		return ImportLine{}, err
	}
	lunchHours, err := parseHourToken(lunchToken)
	if err != nil {
		return ImportLine{}, err
	}
	offHours, err := parseHourToken(offToken)
	if err != nil {
		return ImportLine{}, err
	}

	return ImportLine{
		When:       when,
		WorkHours:  workHours,
		LunchHours: lunchHours,
		OffHours:   offHours,
	}, nil
}

// ParseImportDate parses date values used by work report imports.
func ParseImportDate(token string) (time.Time, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return time.Time{}, errors.New("import date is empty")
	}

	parsed, parseErr := timefmt.Parse(trimmed)
	if parseErr == nil {
		return parsed, nil
	}

	layouts := []string{
		"02.01.2006",
		"20060102",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported import date %q: %w", token, parseErr)
}

func parseHourToken(token string) (float64, error) {
	clean := strings.TrimSpace(token)
	if idx := strings.Index(clean, ":"); idx >= 0 {
		clean = clean[idx+1:]
	}
	clean = strings.TrimSuffix(clean, "h")

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hour token %q: %w", token, err)
	}
	return value, nil
}

func hoursToSeconds(hours float64) int64 {
	return int64(hours * importSecondsPerHour)
}
