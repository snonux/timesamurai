package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newReportCmd builds `work report [range]`. With NO range argument it
// prints the entire history: report.txt, the golden-parity fixture task 271
// diffs against `ruby worktime.rb --report`, is a full dump from the start
// of time, so the no-args path here must feed BuildReport every entry ever
// recorded rather than defaulting to some implicit window like "this week".
// Only when a range IS given does it narrow the entries first, via the same
// buildFilter/worktime.Query machinery list.go/search.go use, before
// building the report.
func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report [range]",
		Short: "Print the accounting report (full history unless [range] is given)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd, args)
		},
	}
	// 571: same [range] completer list.go wires up.
	cmd.ValidArgsFunction = completeRanges
	return cmd
}

// runReport loads the runtime, narrows entries to args[0]'s range when
// given, builds the week-by-week report, and prints it.
func runReport(cmd *cobra.Command, args []string) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	entries, err := reportEntries(rt.store, positionalRange(args))
	if err != nil {
		return err
	}

	weeks, err := worktime.BuildReport(entries, rt.cfg.Accounting, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), worktime.FormatReport(weeks, rt.verbose))
	return nil
}

// reportEntries returns every entry across every host (worktime.CollectEntries)
// when rangeArg is "" -- the no-args case that must reproduce the full
// golden-report history -- or that same set narrowed to rangeArg via
// buildFilter/worktime.Query when a range was given. Filtering (rather than
// re-deriving Since/Until bounds here) reuses t61's already-tested Filter
// logic instead of a second, potentially-diverging implementation.
//
// A ranged query filters purely on each entry's own epoch, so a session that
// logged in before the range and logged out inside it loses its login entry
// but keeps its logout -- BuildReport would then hard-error with "logout
// without login" (task 281). withBoundaryLogins repairs exactly that case
// before the entries are handed to BuildReport.
func reportEntries(store *worktime.Store, rangeArg string) ([]worktime.Entry, error) {
	all := worktime.CollectEntries(store)
	if rangeArg == "" {
		return all, nil
	}

	filter, err := buildFilter(rangeArg, filterFlagValues{})
	if err != nil {
		return nil, err
	}
	rows, err := worktime.Query(all, filter)
	if err != nil {
		return nil, err
	}

	entries := make([]worktime.Entry, len(rows))
	for i, row := range rows {
		entries[i] = row.Entry
	}
	return withBoundaryLogins(entries, all, filter.Since), nil
}

// withBoundaryLogins splices a synthetic login into entries for every
// category that (per the FULL unfiltered history in all) still had a login
// open immediately before since -- a session straddling the range's Since
// boundary. Query already dropped that session's real login (it falls
// before Since) while keeping its logout (it falls inside the range), which
// is what produces the orphan "logout without login" BuildReport otherwise
// errors on.
//
// The synthetic entry is pinned to since itself, not the session's true
// start: this deliberately credits only the in-range portion of the session
// (e.g. today's slice of a login that started yesterday) rather than the
// whole multi-day span, and keeps the entry from landing on an out-of-range
// day that would otherwise leak into the printed report as its own day
// line. A category with no open login at since is untouched; since.IsZero()
// (no lower bound at all, e.g. a range with only an Until) is a no-op since
// nothing can precede an unset boundary.
func withBoundaryLogins(entries, all []worktime.Entry, since time.Time) []worktime.Entry {
	if since.IsZero() {
		return entries
	}

	open := worktime.OpenLoginsBefore(all, since)
	if len(open) == 0 {
		return entries
	}

	out := make([]worktime.Entry, 0, len(entries)+len(open))
	for _, boundary := range open {
		out = append(out, worktime.Entry{
			Action: "login",
			Epoch:  since.Unix(),
			Host:   boundary.Host,
			Tags:   []string{boundary.Category},
		})
	}
	return append(out, entries...)
}
