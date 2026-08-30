package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// openSession describes one still-open login: which accounting category,
// which host holds it, and since when.
type openSession struct {
	category string
	host     string
	since    time.Time
}

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
			printStatus(cmd, openSessions(rt.store, rt.cfg.Accounting))
			return nil
		},
	}
}

// openSessions replays every entry in epoch order (worktime.CollectEntries
// is already globally sorted for exactly this kind of replay), tracking the
// most recent login/logout per accounting category. This mirrors
// worktime.Start/Stop's internal openSessionHost/sessionKey state machine,
// reimplemented here against the package's exported surface
// (AccountingTag, WorkTag, CollectEntries) since those two helpers are
// unexported and status is not itself a mutation that belongs in
// entries.go.
func openSessions(store *worktime.Store, cfg config.AccountingConfig) []openSession {
	open := make(map[string]openSession)
	for _, e := range worktime.CollectEntries(store) {
		category, err := worktime.AccountingTag(cfg, e.Tags)
		if err != nil {
			continue // malformed historical tags: skip rather than abort the whole replay
		}
		if category == "" {
			category = worktime.WorkTag
		}
		applySessionEvent(open, category, e)
	}
	return sortedSessions(open)
}

// applySessionEvent updates open in place for one entry: a login opens (or
// re-opens, e.g. on a superseded prior login) the category, a logout closes
// it, and any other action leaves session state untouched.
func applySessionEvent(open map[string]openSession, category string, e worktime.Entry) {
	switch strings.ToLower(strings.TrimSpace(e.Action)) {
	case "login":
		open[category] = openSession{category: category, host: e.Host, since: time.Unix(e.Epoch, 0)}
	case "logout":
		delete(open, category)
	}
}

// sortedSessions returns open's values sorted by category, so `work status`
// output is stable across runs instead of following Go's randomized map
// iteration order.
func sortedSessions(open map[string]openSession) []openSession {
	result := make([]openSession, 0, len(open))
	for _, s := range open {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].category < result[j].category })
	return result
}

// printStatus writes open to cmd's stdout. Write errors are discarded
// explicitly (same rationale as output.go's printEntryResult) rather than
// left unchecked, so errcheck can confirm the sink is trusted rather than
// just assume it.
func printStatus(cmd *cobra.Command, open []openSession) {
	w := cmd.OutOrStdout()
	if len(open) == 0 {
		_, _ = fmt.Fprintln(w, "no open sessions")
		return
	}
	for _, s := range open {
		_, _ = fmt.Fprintf(w, "%s: open on %s since %s\n", s.category, s.host, s.since.Format("2006-01-02 15:04:05"))
	}
}
