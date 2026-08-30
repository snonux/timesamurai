package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// filterFlagValues holds the raw text of the query filter flags shared by
// `work list` and `work search`, bound directly to Cobra flag storage by
// addFilterFlags. Keeping these as strings (rather than parsing eagerly at
// bind time) lets buildFilter report parse errors after cobra has already
// validated argument counts, and lets search populate descr itself from its
// positional <text> argument instead of a flag.
type filterFlagValues struct {
	host, tag, action, descr string
	since, until             string
	min, max                 string
	limit                    int
	format                   string
}

// newListCmd builds `work list [range]`, printing every entry matching the
// filter flags as "<host>:<id>"-addressed rows via worktime.Query/FormatTable
// (or FormatJSON with --format json) -- reusing t61's query/output machinery
// rather than re-filtering or re-rendering here.
func newListCmd() *cobra.Command {
	var f filterFlagValues
	cmd := &cobra.Command{
		Use:   "list [range]",
		Short: "List entries matching filters, addressed for modify/delete",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, args, f)
		},
	}
	addFilterFlags(cmd, &f, true)
	// 571: the optional [range] positional completes against
	// internal/timefmt's fixed range keywords (complete.go); --host/--tag
	// completion is wired once, for both list and search, inside
	// addFilterFlags below.
	cmd.ValidArgsFunction = completeRanges
	return cmd
}

// addFilterFlags registers the query filter surface --host --tag --action
// --descr --since --until --min --max --limit --format. includeDescr is
// false for `work search`, whose required <text> positional fills the same
// role as --descr -- offering both would just invite "which one wins"
// confusion for no benefit, so search's flag set omits it entirely.
func addFilterFlags(cmd *cobra.Command, f *filterFlagValues, includeDescr bool) {
	cmd.Flags().StringVar(&f.host, "host", "", "match entries from exactly this host")
	cmd.Flags().StringVar(&f.tag, "tag", "", "match entries carrying this tag")
	cmd.Flags().StringVar(&f.action, "action", "", "match entries with this action (login/logout/add)")
	// 571: --host/--tag both enumerate against the store's actual values
	// (complete.go) rather than offering nothing or falling back to file
	// completion, which would never be the right suggestion for either flag.
	registerFlagCompletion(cmd, "host", completeHosts)
	registerFlagCompletion(cmd, "tag", completeTags)
	if includeDescr {
		cmd.Flags().StringVarP(&f.descr, "descr", "d", "", "match entries whose description contains this text (case-insensitive)")
	}
	cmd.Flags().StringVar(&f.since, "since", "", "inclusive lower time bound (same formats as --at)")
	cmd.Flags().StringVar(&f.until, "until", "", "inclusive upper time bound (same formats as --at)")
	cmd.Flags().StringVar(&f.min, "min", "", "inclusive lower value bound, as a duration (e.g. 1h)")
	cmd.Flags().StringVar(&f.max, "max", "", "inclusive upper value bound, as a duration (e.g. 8h)")
	cmd.Flags().IntVar(&f.limit, "limit", 0, "cap the number of rows returned (0 = unlimited)")
	cmd.Flags().StringVar(&f.format, "format", "table", "output format: table or json")
}

// runList resolves the optional positional range and flag values into a
// worktime.Filter, queries every host's entries, and prints the result.
func runList(cmd *cobra.Command, args []string, f filterFlagValues) error {
	rows, err := queryRows(cmd, positionalRange(args), f)
	if err != nil {
		return err
	}
	return printRows(cmd, rows, f.format)
}

// positionalRange returns args[0] when list/search was given an optional
// leading range argument, or "" when it wasn't -- centralizing the
// "0-or-1 positional" shape both commands share.
func positionalRange(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// queryRows loads the runtime's store and runs worktime.Query against every
// host's entries (CollectEntries), using the filter buildFilter derives from
// rangeArg and f. Shared by list and search (and, for its own no-range-arg
// full-history path, indirectly documents the same filter-building rules
// report.go relies on).
func queryRows(cmd *cobra.Command, rangeArg string, f filterFlagValues) ([]worktime.Row, error) {
	filter, err := buildFilter(rangeArg, f)
	if err != nil {
		return nil, err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return nil, err
	}
	rows, err := worktime.Query(worktime.CollectEntries(rt.store), filter)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// buildFilter turns rangeArg (the optional positional range; "" means none)
// and f's flag values into a worktime.Filter. Precedence: a range sets the
// initial Since/Until bounds, then an explicit --since or --until flag
// overrides its own half independently -- so e.g. "list week --until
// 2026-08-15" keeps the range's Monday start but tightens only the end.
// Explicit flags win because they are the more specific request, but only
// for the bound they actually set; the other bound still comes from the
// range.
func buildFilter(rangeArg string, f filterFlagValues) (worktime.Filter, error) {
	var filter worktime.Filter
	if rangeArg != "" {
		bounds, err := timefmt.ParseRange(rangeArg)
		if err != nil {
			return worktime.Filter{}, fmt.Errorf("range: %w", err)
		}
		filter.Since = bounds.Start
		// Range is a half-open [Start, End) interval but Filter's Until is
		// an inclusive bound; entries carry integer-second epochs, so
		// stepping back one nanosecond excludes End itself without being
		// able to exclude any entry that legitimately belongs in the range.
		filter.Until = bounds.End.Add(-time.Nanosecond)
	}

	if err := applyTimeBoundFlags(&filter, f); err != nil {
		return worktime.Filter{}, err
	}
	if err := applyValueBoundFlags(&filter, f); err != nil {
		return worktime.Filter{}, err
	}

	filter.Host = f.host
	filter.Tag = f.tag
	filter.Action = f.action
	filter.Descr = f.descr
	filter.Limit = f.limit
	return filter, nil
}

// applyTimeBoundFlags overrides filter's Since/Until with --since/--until
// when set, accepting the same formats as --at (clock time, ISO date,
// "today"/"yesterday", or a relative offset). --since is parsed with
// timefmt.ParseTime, which resolves a bare date/today/yesterday to that
// day's midnight -- correct for a lower bound. --until is parsed with
// timefmt.ParseUntil instead: since --until is documented as an *inclusive*
// upper bound, a bare date/today/yesterday there must resolve to the end of
// that day (not its midnight start), or entries later that same day would be
// wrongly excluded (task 381). Clock times, RFC3339, and other instant
// values are returned unchanged by ParseUntil, so their exact-instant
// semantics are unaffected.
func applyTimeBoundFlags(filter *worktime.Filter, f filterFlagValues) error {
	if f.since != "" {
		t, err := timefmt.ParseTime(f.since)
		if err != nil {
			return fmt.Errorf("--since: %w", err)
		}
		filter.Since = t
	}
	if f.until != "" {
		t, err := timefmt.ParseUntil(f.until)
		if err != nil {
			return fmt.Errorf("--until: %w", err)
		}
		filter.Until = t
	}
	return nil
}

// applyValueBoundFlags sets filter's Min/Max from --min/--max, parsed as
// durations (internal/timefmt.ParseDuration) since Entry.Value is always a
// count of seconds -- "1h"/"30m" reads far better at the CLI than requiring
// callers to compute raw seconds themselves.
func applyValueBoundFlags(filter *worktime.Filter, f filterFlagValues) error {
	if f.min != "" {
		v, err := parseValueBound("--min", f.min)
		if err != nil {
			return err
		}
		filter.Min = &v
	}
	if f.max != "" {
		v, err := parseValueBound("--max", f.max)
		if err != nil {
			return err
		}
		filter.Max = &v
	}
	return nil
}

// parseValueBound parses one --min/--max duration string into seconds,
// wrapping any error with the flag name so a bad --max doesn't get reported
// as if it were --min or vice versa.
func parseValueBound(flag, value string) (int64, error) {
	d, err := timefmt.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", flag, err)
	}
	return int64(d.Seconds()), nil
}

// printRows renders rows via worktime.FormatTable/FormatJSON depending on
// format (case-insensitive; empty defaults to table) and writes the result
// to cmd's stdout with exactly one trailing newline -- FormatTable already
// ends in "\n" (or is empty for zero rows plus its header), but FormatJSON's
// indented output does not, so JSON gets one appended here rather than
// leaving output format-dependent on whether the last line is terminated.
func printRows(cmd *cobra.Command, rows []worktime.Row, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "table":
		_, _ = fmt.Fprint(cmd.OutOrStdout(), worktime.FormatTable(rows))
		return nil
	case "json":
		out, err := worktime.FormatJSON(rows)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
		return nil
	default:
		return fmt.Errorf("unsupported --format %q (want table or json)", format)
	}
}
