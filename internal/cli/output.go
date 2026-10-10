package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// addAtDescrFlags registers the --at and --descr/-d flags shared by every
// mutating verb (start, stop, add, sub, usebuffer, day-off), binding them
// into the caller's local variables so each verb file keeps its own flag
// storage rather than reaching into package state.
func addAtDescrFlags(cmd *cobra.Command, at, descr *string) {
	cmd.Flags().StringVar(at, "at", "",
		"time for the entry: clock time, today/yesterday/tomorrow/now, next/this/last <weekday>, bare weekday names, '<n> days/weeks/months/years ago', ISO-8601, or relative like -2h (default now)")
	cmd.Flags().StringVarP(descr, "descr", "d", "", "free-text description for the entry")
}

// parseAtFlag parses the --at flag text via internal/timefmt, returning the
// zero time.Time when unset. The zero value is intentional, not a sentinel
// bug: every worktime mutation (Start/Stop/Add/Sub/UseBuffer) already treats
// a zero time.Time as "use time.Now()" (see entries.go's epochOf), so an
// unset --at flag needs no special-casing here.
func parseAtFlag(at string) (time.Time, error) {
	if strings.TrimSpace(at) == "" {
		return time.Time{}, nil
	}
	t, err := timefmt.ParseTime(at)
	if err != nil {
		return time.Time{}, fmt.Errorf("--at: %w", err)
	}
	return t, nil
}

// printEntryResult writes one mutation's result: a full field dump under
// --verbose, or a terse "<verb> <host>:<id>" confirmation otherwise. Kept
// separate from printEntriesResult (rather than the latter always calling
// this in a loop with an extra branch) only to keep both callers' intent
// obvious at the call site -- one entry vs. several.
func printEntryResult(cmd *cobra.Command, theme ansi.Theme, verb string, entry worktime.Entry, verbose bool) {
	// The underlying sink is cobra's configured stdout writer, which in
	// normal operation is os.Stdout and in tests is an in-memory buffer;
	// neither fails in practice, but the errors are discarded explicitly
	// (matching worktime/query.go's FormatTable) so errcheck can confirm
	// that rather than just assume it.
	if verbose {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n",
			theme.Wrap(ansi.RoleSuccess, verb), formatEntry(entry, theme))
		return
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n",
		theme.Wrap(ansi.RoleSuccess, verb),
		theme.Wrap(ansi.RoleAddress, fmt.Sprintf("%s:%d", entry.Host, entry.ID)))
}

// printEntriesResult reports every entry a multi-entry mutation produced
// (currently only UseBuffer's withdraw+credit pair) via printEntryResult, so
// --verbose/terse formatting stays defined in exactly one place.
func printEntriesResult(cmd *cobra.Command, theme ansi.Theme, verb string, entries []worktime.Entry, verbose bool) {
	for _, e := range entries {
		printEntryResult(cmd, theme, verb, e, verbose)
	}
}

// formatEntry renders entry's fields for --verbose output. Tags are
// comma-joined and descr is quoted only when present, so a plain entry
// doesn't end with a trailing empty descr="". With an enabled theme every
// field value is styled by the same role its `work list` column uses (tasks
// n9 and m83), and the field names are faint so the values stand out.
func formatEntry(e worktime.Entry, theme ansi.Theme) string {
	when := time.Unix(e.Epoch, 0).Format("2006-01-02 15:04:05")
	field := func(name, role, value string) string {
		return theme.Wrap(ansi.RoleDim, name+"=") + theme.Wrap(role, value)
	}
	base := strings.Join([]string{
		theme.Wrap(ansi.RoleAddress, fmt.Sprintf("%s:%d", e.Host, e.ID)),
		field("action", ansi.RoleKey, e.Action),
		field("at", ansi.RoleTime, when),
		field("value", ansi.RoleValue, fmt.Sprintf("%d", e.Value)),
		field("tags", ansi.RoleTag, strings.Join(e.Tags, ",")),
	}, " ")
	if e.Descr == "" {
		return base
	}
	return base + " " + field("descr", "", fmt.Sprintf("%q", e.Descr))
}
