package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newUndoCmd builds `work undo`, reverting the current host's most recent
// insert/modify/delete via Store.UndoLast (task r61) and reporting what was
// undone. There is no argument: undo always targets the current host's own
// undo log, matching how every other mutating verb here writes under the
// current host by default.
func newUndoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undo",
		Short: "Revert the last insert/modify/delete on this host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndo(cmd)
		},
	}
}

func runUndo(cmd *cobra.Command) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	rec, err := rt.store.UndoLast(cmdContext(cmd), rt.host)
	if err != nil {
		return err
	}
	printUndoResult(cmd, rec, rt.verbose)
	return nil
}

// printUndoResult reports which op was undone and which entry it affected.
// --verbose additionally prints the before/after (or restored) entry so a
// caller can confirm exactly what came back, the same level of detail
// printEntryResult gives every other mutation.
func printUndoResult(cmd *cobra.Command, rec worktime.UndoRecord, verbose bool) {
	w := cmd.OutOrStdout()
	host := undoRecordHost(rec)
	_, _ = fmt.Fprintf(w, "undo %s: %s:%d\n", rec.Op, host, rec.ID)
	if !verbose {
		return
	}
	printUndoDetail(w, rec)
}

// printUndoDetail writes the verbose detail line for one undo record, split
// out of printUndoResult so that function's terse/verbose branching stays a
// flat read. Wording matches what UndoLast actually did to the entry:
// undoing an insert removes it, undoing a delete restores it, and undoing a
// modify reverts it from its post-modify state back to its pre-modify one.
func printUndoDetail(w io.Writer, rec worktime.UndoRecord) {
	switch rec.Op {
	case worktime.OpInsert:
		if rec.After != nil {
			_, _ = fmt.Fprintf(w, "  removed: %s\n", formatEntry(*rec.After))
		}
	case worktime.OpDelete:
		if rec.Before != nil {
			_, _ = fmt.Fprintf(w, "  restored: %s\n", formatEntry(*rec.Before))
		}
	case worktime.OpModify:
		if rec.Before != nil && rec.After != nil {
			_, _ = fmt.Fprintf(w, "  reverted from: %s\n  back to:       %s\n", formatEntry(*rec.After), formatEntry(*rec.Before))
		}
	}
}

// undoRecordHost reads the host off whichever of Before/After is present,
// since UndoRecord itself carries no separate Host field.
func undoRecordHost(rec worktime.UndoRecord) string {
	if rec.After != nil {
		return rec.After.Host
	}
	if rec.Before != nil {
		return rec.Before.Host
	}
	return ""
}
