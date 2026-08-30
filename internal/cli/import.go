package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// importSecondsPerHour converts the fractional hour values report.txt lines
// carry into whole seconds, the unit worktime.Add expects via time.Duration.
const importSecondsPerHour = 3600

// Package boundary note: this file, not internal/worktime, is the home for
// the pre-rewrite report.txt line parser (see
// `git show pre-rewrite:internal/worktime/import.go`). That old file's
// ParseImportLine/ParseImportDate are pure text parsing with no dependency
// on the worktime package's model beyond time.Time, and the worktime
// package's file layout (migrate.go, export.go, legacy.go) has no
// import.go alongside them -- only "work import <file>" ever needs this
// parser. Since the only thing left to port is the parsing and the one
// call that used to write straight to a dbDir now goes through
// worktime.Add against an opened *Store, keeping the parser here avoids
// re-widening the worktime package's API for a single CLI verb, while
// still being a five-minute move back into worktime/import.go later if
// another caller ever needs the parser outside the CLI.

// importLine is one parsed report.txt day: hours by category. Mirrors the
// pre-rewrite tool's ImportLine field-for-field.
type importLine struct {
	when       time.Time
	workHours  float64
	lunchHours float64
	offHours   float64
}

// newImportCmd builds `work import <file>`: apply a report.txt-format file
// (worktime.rb's --report output, or one produced by `work report`'s older
// sibling format) as work/lunch/off entries against the current host.
func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Import report.txt-format lines as work/lunch/off entries",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd, args[0])
		},
	}
	registerHostFlag(cmd)
	return cmd
}

// runImport opens the runtime and the given file, then delegates line
// scanning and entry creation to importReport. The file handle is owned
// entirely here so importReport can stay a pure "given a reader" function,
// easy to unit test against a strings.Reader without touching a real file.
func runImport(cmd *cobra.Command, path string) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open import file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	return importReport(cmdContext(cmd), cmd, rt, f)
}

// importReport scans report line by line -- only lines containing "lunch:"
// are report day lines; headers, week separators, and totals are skipped,
// mirroring the pre-rewrite ImportReport's own filter -- parses each into
// work/lunch/off hours, and applies them via applyImportLine. It stops at
// the first parse or Add error, matching the old function's fail-fast
// behavior (a malformed or partially-applied import is surfaced, not
// swallowed).
func importReport(ctx context.Context, cmd *cobra.Command, rt *runtime, report io.Reader) error {
	scanner := bufio.NewScanner(report)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "lunch:") {
			continue
		}

		parsed, err := parseImportLine(line)
		if err != nil {
			return err
		}
		if err := applyImportLine(ctx, cmd, rt, parsed); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan import file: %w", err)
	}
	return nil
}

// applyImportLine turns one parsed day into up to three worktime.Add calls
// (work, lunch, off), skipping any category whose duration is zero.
//
// Lunch is folded into the work duration before the work Add lands -- the
// old ImportReport's workSeconds += lunchSeconds -- and is then also
// recorded as its own "lunch" entry. This is not double-counting: it
// preserves the accounting worktime.rb always used, where a raw "work"
// entry includes any lunch taken that day and the report layer
// (worktime.BuildReport's flushDay) subtracts lunch back out again for
// day-level display. Changing this here would silently shrink every
// imported day's work total relative to entries recorded any other way.
func applyImportLine(ctx context.Context, cmd *cobra.Command, rt *runtime, parsed importLine) error {
	workSeconds := hoursToSeconds(parsed.workHours)
	lunchSeconds := hoursToSeconds(parsed.lunchHours)
	offSeconds := hoursToSeconds(parsed.offHours)
	if lunchSeconds > 0 {
		workSeconds += lunchSeconds
	}

	credits := []struct {
		tag     string
		seconds int64
	}{
		{worktime.WorkTag, workSeconds},
		{"lunch", lunchSeconds},
		{"off", offSeconds},
	}
	for _, c := range credits {
		if c.seconds <= 0 {
			continue
		}
		entry, err := worktime.Add(ctx, rt.store, rt.cfg.Accounting, rt.host,
			[]string{c.tag}, time.Duration(c.seconds)*time.Second, parsed.when, "")
		if err != nil {
			return fmt.Errorf("import %s entry: %w", c.tag, err)
		}
		printEntryResult(cmd, "import", entry, rt.verbose)
	}
	return nil
}

// parseImportLine parses one report-format day line, e.g.
// "Mon 06.01.2026: +8.00h lunch: +0.50h off: +1.00h". Ported field-for-field
// from the pre-rewrite tool's ParseImportLine: the date sits in fields[1]
// (trailing ":" trimmed), work in fields[2], lunch in fields[4], and off in
// fields[6] -- the fixed shape worktime.rb always emitted for a report day.
func parseImportLine(line string) (importLine, error) {
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return importLine{}, fmt.Errorf("unsupported import line: %q", line)
	}

	when, err := parseImportDate(strings.TrimSuffix(fields[1], ":"))
	if err != nil {
		return importLine{}, err
	}
	workHours, err := parseHourToken(fields[2])
	if err != nil {
		return importLine{}, err
	}
	lunchHours, err := parseHourToken(fields[4])
	if err != nil {
		return importLine{}, err
	}
	offHours, err := parseHourToken(fields[6])
	if err != nil {
		return importLine{}, err
	}

	return importLine{when: when, workHours: workHours, lunchHours: lunchHours, offHours: offHours}, nil
}

// parseImportDate parses one report day's date token. It tries the two
// fixed legacy layouts the pre-rewrite tool's ParseImportDate accepted --
// "02.01.2006" and "20060102" -- first, since real report.txt files
// predate timefmt and use those formats exclusively, then falls back to
// internal/timefmt.ParseTime for anything else. The order matters:
// ParseTime treats any bare digit string as a unix epoch (see its
// bareIntegerPattern handling), so trying it before the "20060102" layout
// would silently misread every compact legacy date as a bogus 1970s epoch
// instead of failing over to the layout that actually matches. The old
// function called a since-removed `timefmt.Parse`; ParseTime is its
// closest surviving equivalent, kept only as the fallback for date shapes
// the legacy format never produced.
func parseImportDate(token string) (time.Time, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return time.Time{}, errors.New("import date is empty")
	}

	layouts := []string{"02.01.2006", "20060102"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}
	if parsed, err := timefmt.ParseTime(trimmed); err == nil {
		return parsed, nil
	}
	return time.Time{}, fmt.Errorf("unsupported import date %q", token)
}

// parseHourToken parses one "+8.00h"/"lunch:+0.50h"-style field into hours:
// it strips any "label:" prefix up to the last colon and a trailing "h"
// unit suffix before handing the remainder to strconv.ParseFloat.
func parseHourToken(token string) (float64, error) {
	clean := strings.TrimSpace(token)
	if idx := strings.LastIndex(clean, ":"); idx >= 0 {
		clean = clean[idx+1:]
	}
	clean = strings.TrimSuffix(clean, "h")

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hour token %q: %w", token, err)
	}
	return value, nil
}

// hoursToSeconds converts a fractional hour count to whole seconds,
// truncating any sub-second remainder the same way the pre-rewrite tool did.
func hoursToSeconds(hours float64) int64 {
	return int64(hours * importSecondsPerHour)
}
