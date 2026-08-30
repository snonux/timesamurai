package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// sessionAction is the shape shared by worktime.Start and worktime.Stop, so
// newSessionCmd builds "start"/"login" and "stop"/"logout" from one template
// instead of duplicating flag wiring and result handling four times.
type sessionAction func(ctx context.Context, store *worktime.Store, cfg worktime.AccountingConfig, host string, tags []string, at time.Time, descr string) (worktime.Entry, error)

// newSessionCmd builds a start/stop-shaped subcommand: bare positional words
// are tags (default WorkTag when none given), --at/--descr set the entry's
// time and description. hidden drives the login/logout aliases: task w61
// asks for them to keep working without showing up in help, so they are
// separate Hidden cobra.Commands -- Cobra's own Aliases field would instead
// print them under "Aliases:" on the visible command's help text, which is
// not what "hidden" means here.
func newSessionCmd(use, short string, hidden bool, verb string, action sessionAction) *cobra.Command {
	var at, descr string

	cmd := &cobra.Command{
		Use:    use + " [tags...]",
		Short:  short,
		Hidden: hidden,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSession(cmd, verb, action, at, descr, args)
		},
	}
	addAtDescrFlags(cmd, &at, &descr)
	registerHostFlag(cmd)
	return cmd
}

// runSession parses --at, opens the runtime, and invokes action -- split out
// of newSessionCmd's RunE closure so that closure stays a one-liner and this
// function reads top-to-bottom as "validate cheap inputs, then touch disk".
func runSession(cmd *cobra.Command, verb string, action sessionAction, at, descr string, tags []string) error {
	atTime, err := parseAtFlag(at)
	if err != nil {
		return err
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	entry, err := action(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, tags, atTime, descr)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	printEntryResult(cmd, verb, entry, rt.verbose)
	return nil
}
