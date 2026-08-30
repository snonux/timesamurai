package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newStatusCmd builds `work status`, showing every category with an open
// login and no matching logout yet. This intentionally stops at "what is
// running" -- the fuller "plus today and this week" view the plan's CLI
// table mentions belongs to `work report` (task x61's concern), which reuses
// the same accounting math this command does not need to duplicate.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show open sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := newRuntime(cmd)
			if err != nil {
				return err
			}
			printStatus(cmd, worktime.OpenSessions(rt.store, rt.cfg.Accounting))
			return nil
		},
	}
}

// printStatus writes open to cmd's stdout. This is the command's whole job:
// formatting worktime.OpenSessions' result, with no login/logout state
// machine of its own -- that single implementation now lives in
// worktime.OpenSessions, shared with Start/Stop's open-login guard, so
// status can never drift out of sync with what a login/logout actually
// does. Write errors are discarded explicitly (same rationale as
// output.go's printEntryResult) rather than left unchecked, so errcheck can
// confirm the sink is trusted rather than just assume it.
func printStatus(cmd *cobra.Command, open []worktime.OpenSession) {
	w := cmd.OutOrStdout()
	if len(open) == 0 {
		_, _ = fmt.Fprintln(w, "no open sessions")
		return
	}
	for _, s := range open {
		_, _ = fmt.Fprintf(w, "%s: open on %s since %s\n", s.Category, s.Host, s.Since.Format("2006-01-02 15:04:05"))
	}
}
