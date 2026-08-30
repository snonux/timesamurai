package cli

import (
	"fmt"

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
	return entries, nil
}
