package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// dayOffHours is the fixed day-off credit, matching the pre-rewrite tool's
// AddDayOff (see `git show pre-rewrite:internal/worktime/entries.go`). Task
// s61 deliberately did not carry AddDayOff into internal/worktime/entries.go
// -- its disposition notes say only the positional EditEntry was excluded on
// purpose, but AddDayOff itself never reappeared there either. Rather than
// treat that as blocking, this CLI task recreates its semantics (an 8-hour
// Add against the "off" tag, dated to the start of the day) as a composition
// of the existing worktime.Add primitive: day-off was never anything more
// than sugar over Add in the pre-rewrite tool, so no new worktime package
// API is needed for it.
const dayOffHours = 8

// creditAction is the shape shared by worktime.Add and worktime.Sub.
type creditAction func(ctx context.Context, store *worktime.Store, cfg worktime.AccountingConfig, host string, tags []string, duration time.Duration, at time.Time, descr string) (worktime.Entry, error)

func newAddCmd() *cobra.Command {
	var at, descr string
	cmd := &cobra.Command{
		Use:   "add <duration> [tags...]",
		Short: "Credit a duration (default tag: work)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredit(cmd, at, descr, args, worktime.Add)
		},
	}
	addAtDescrFlags(cmd, &at, &descr)
	return cmd
}

func newSubCmd() *cobra.Command {
	var at, descr string
	cmd := &cobra.Command{
		Use:   "sub <duration> [tags...]",
		Short: "Withdraw a duration (default tag: work)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCredit(cmd, at, descr, args, worktime.Sub)
		},
	}
	addAtDescrFlags(cmd, &at, &descr)
	return cmd
}

// runCredit parses args[0] as a duration (internal/timefmt.ParseDuration:
// bare integer = seconds, else 30m/1h/1h30m/2.5h/45s) and the rest as tags,
// then calls action -- shared so add/sub don't duplicate flag parsing and
// result printing.
func runCredit(cmd *cobra.Command, at, descr string, args []string, action creditAction) error {
	duration, err := timefmt.ParseDuration(args[0])
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	atTime, err := parseAtFlag(at)
	if err != nil {
		return err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	entry, err := action(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, args[1:], duration, atTime, descr)
	if err != nil {
		return err
	}
	printEntryResult(cmd, cmd.Name(), entry, rt.verbose)
	return nil
}

// newUseBufferCmd builds `work usebuffer <duration>`. Unlike add/sub it
// takes no tag arguments: worktime.UseBuffer hardcodes the
// selfdevelopment-to-work transfer (see entries.go's bufferSourceTag doc
// comment), matching the pre-rewrite tool and the CLI plan, which likewise
// documents usebuffer as taking only a duration.
func newUseBufferCmd() *cobra.Command {
	var at, descr string
	cmd := &cobra.Command{
		Use:   "usebuffer <duration>",
		Short: "Move buffer hours (selfdevelopment) into work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUseBuffer(cmd, at, descr, args[0])
		},
	}
	addAtDescrFlags(cmd, &at, &descr)
	return cmd
}

func runUseBuffer(cmd *cobra.Command, at, descr, durationArg string) error {
	duration, err := timefmt.ParseDuration(durationArg)
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	atTime, err := parseAtFlag(at)
	if err != nil {
		return err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	entries, err := worktime.UseBuffer(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, duration, atTime, descr)
	if err != nil {
		return err
	}
	printEntriesResult(cmd, "usebuffer", entries, rt.verbose)
	return nil
}

// newDayOffCmd implements the day-off convenience verb the plan's CLI table
// keeps ("timesamurai work day-off [--at ...]"). Extra positional words are
// accepted as additional label tags appended after "off" -- consistent with
// "bare words after a verb are tags" everywhere else in this CLI group, and
// a deliberate, low-risk extension since worktime.rb's day-off took no such
// option and this task's guidance says to treat ambiguity here as low-risk
// best-effort.
func newDayOffCmd() *cobra.Command {
	var at, descr string
	cmd := &cobra.Command{
		Use:   `day-off [tags...]`,
		Short: `Credit a full day off (8h against the "off" tag)`,
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDayOff(cmd, at, descr, args)
		},
	}
	addAtDescrFlags(cmd, &at, &descr)
	return cmd
}

func runDayOff(cmd *cobra.Command, at, descr string, extraTags []string) error {
	atTime, err := parseAtFlag(at)
	if err != nil {
		return err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	day := startOfDayOrNow(atTime)
	tags := append([]string{"off"}, extraTags...)
	entry, err := worktime.Add(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, tags, dayOffHours*time.Hour, day, descr)
	if err != nil {
		return err
	}
	printEntryResult(cmd, "day-off", entry, rt.verbose)
	return nil
}

// startOfDayOrNow returns the start (00:00:00) of at's calendar day, or of
// today when at is the zero time.Time (parseAtFlag's "--at unset" value).
// internal/timefmt has an equivalent helper but it is unexported, and this
// case is subtly different from parseAtFlag's usual "zero means now": a
// day-off dated to "right now" would carry a random time-of-day, whereas
// pre-rewrite's AddDayOff always normalized to midnight.
func startOfDayOrNow(at time.Time) time.Time {
	if at.IsZero() {
		at = time.Now()
	}
	year, month, day := at.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, at.Location())
}
