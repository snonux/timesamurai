package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// This file implements `work edit [range]`: render entries in range (or the
// full history when no range is given, matching `work list`'s own default)
// as a plain-text block, open it in $EDITOR, and turn whatever comes back
// into individual worktime.Modify/Delete/insert calls -- one mutation per
// changed line, so every change lands in the undo log separately instead of
// as one opaque batch operation.
//
// The file is kept as a thin coordinator (newEditCmd/runEdit plus the small
// glue functions runEdit calls directly): the text-block format itself
// (render/parse/diff) lives in edit_format.go, applying the resulting ops
// via worktime mutations lives in edit_apply.go, and the $EDITOR/scratch-file
// plumbing lives in edit_editor.go. See edit_format.go's header comment for
// the field-by-field format documentation.

// newEditCmd builds `work edit [range]`.
func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit [range]",
		Short: "Edit entries in $EDITOR as a text block",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(cmd, positionalRange(args))
		},
	}
	// 571: same [range] completer list.go/report.go wire up.
	cmd.ValidArgsFunction = completeRanges
	return cmd
}

// runEdit loads the rows in range, opens them in $EDITOR, diffs the result
// against the original rendering, and applies each difference as its own
// mutation.
func runEdit(cmd *cobra.Command, rangeArg string) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	rows, err := editRows(rt, rangeArg)
	if err != nil {
		return err
	}

	edited, err := launchEditor(cmd, renderEditBlock(rows))
	if err != nil {
		return err
	}

	ops, err := parseAndDiffEdit(rows, edited)
	if err != nil {
		return err
	}

	applied, applyErr := applyEditOps(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, ops)
	printEditSummary(cmd, ops, applied, rt.verbose)
	return applyErr
}

// editRows resolves rangeArg (list.go's shared buildFilter: "" means the
// full history) against rt's already-open store, without opening a second
// runtime/store the way calling list.go's queryRows here would.
func editRows(rt *runtime, rangeArg string) ([]worktime.Row, error) {
	filter, err := buildFilter(rangeArg, filterFlagValues{})
	if err != nil {
		return nil, err
	}
	return worktime.Query(worktime.CollectEntries(rt.store), filter)
}

// parseAndDiffEdit turns the raw edited text into editOps, wrapping a parse
// failure so it reads distinctly from a diff failure (an edited line
// referencing an address that no longer exists).
func parseAndDiffEdit(rows []worktime.Row, edited string) ([]editOp, error) {
	lines, err := parseEditBlock(edited)
	if err != nil {
		return nil, fmt.Errorf("parse edited block: %w", err)
	}
	return diffEditBlock(rows, lines)
}

// printEditSummary reports each applied op via printEntryResult, the same
// "<verb> <host>:<id>"/verbose shape every other mutating verb uses.
func printEditSummary(cmd *cobra.Command, ops []editOp, applied []worktime.Entry, verbose bool) {
	if len(ops) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no changes")
		return
	}
	for i, entry := range applied {
		printEntryResult(cmd, editOpVerb(ops[i].Kind), entry, verbose)
	}
}

// editOpVerb names the op kind the way printEntryResult's other callers name
// their own verb ("delete"/"insert"/"modify").
func editOpVerb(kind editOpKind) string {
	switch kind {
	case editOpDelete:
		return "delete"
	case editOpInsert:
		return "insert"
	default:
		return "modify"
	}
}
