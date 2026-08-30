package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// newModifyCmd builds `work modify <host:id>`, applying only the fields the
// caller actually passed as a worktime.EntryPatch via worktime.Modify.
// Cobra's Flags().Changed (not "is the string non-empty") is what
// distinguishes "the user didn't mention this flag" from "the user
// explicitly set it to a value that happens to be the zero value" -- e.g. an
// unset --tags must not blank out an entry's tags, but an explicit
// `--tags ""` should.
func newModifyCmd() *cobra.Command {
	var at, value, descr, action string
	var tags []string

	cmd := &cobra.Command{
		Use:   "modify <host:id>",
		Short: "Modify one entry's fields in place",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModify(cmd, args[0], at, value, descr, action, tags)
		},
	}
	cmd.Flags().StringVar(&at, "at", "", "new time for the entry (same formats as start/stop's --at)")
	cmd.Flags().StringVar(&value, "value", "", "new signed value, as a duration (e.g. 1h, -30m) or bare seconds")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "new free-text description")
	cmd.Flags().StringVar(&action, "action", "", "new action (login/logout/add)")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "new comma-separated tag list, replacing every existing tag")
	// 571: complete the <host:id> positional against known entry addresses,
	// each shown with its date and description (complete.go) so a caller
	// can tell entries apart without a separate `work list` first.
	cmd.ValidArgsFunction = completeHostIDAddress
	return cmd
}

// runModify builds the patch, resolves the runtime, and applies it -- split
// out of the RunE closure so that closure stays a one-liner.
func runModify(cmd *cobra.Command, addr, at, value, descr, action string, tags []string) error {
	patch, err := buildModifyPatch(cmd, at, value, descr, action, tags)
	if err != nil {
		return err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	entry, err := worktime.Modify(cmdContext(cmd), rt.store, rt.cfg.Accounting, addr, rt.host, patch)
	if err != nil {
		return err
	}
	printEntryResult(cmd, "modify", entry, rt.verbose)
	return nil
}

// buildModifyPatch turns whichever of --at/--value/--descr/--action/--tags
// were actually passed into a worktime.EntryPatch, leaving every other field
// nil so worktime.Modify's apply() leaves it untouched.
func buildModifyPatch(cmd *cobra.Command, at, value, descr, action string, tags []string) (worktime.EntryPatch, error) {
	var patch worktime.EntryPatch
	flags := cmd.Flags()

	if flags.Changed("at") {
		epoch, err := parseModifyAt(at)
		if err != nil {
			return worktime.EntryPatch{}, err
		}
		patch.Epoch = &epoch
	}
	if flags.Changed("value") {
		seconds, err := parseModifyValue(value)
		if err != nil {
			return worktime.EntryPatch{}, err
		}
		patch.Value = &seconds
	}
	if flags.Changed("descr") {
		patch.Descr = &descr
	}
	if flags.Changed("action") {
		patch.Action = &action
	}
	if flags.Changed("tags") {
		patch.Tags = &tags
	}
	return patch, nil
}

// parseModifyAt parses --at via internal/timefmt, the same parser start/stop
// and the edit block use, so a modify's --at accepts the same clock times,
// ISO dates, and relative offsets everywhere else in this CLI does.
func parseModifyAt(at string) (int64, error) {
	t, err := timefmt.ParseTime(at)
	if err != nil {
		return 0, fmt.Errorf("--at: %w", err)
	}
	return t.Unix(), nil
}

// parseModifyValue parses --value as a signed duration (internal/timefmt's
// ParseDuration: bare integer = seconds, else Go duration syntax including a
// leading "-"), converting it to the signed-seconds shape Entry.Value uses --
// consistent with list.go's --min/--max, which parse the same field the same
// way.
func parseModifyValue(value string) (int64, error) {
	d, err := timefmt.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("--value: %w", err)
	}
	return int64(d.Seconds()), nil
}
