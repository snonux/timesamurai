package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/worktime/legacy"
)

// newExportCmd builds `work export [--strict]`: a rewrite of every host's
// legacy db.<host>.json from the current JSONL store (q61's
// legacy.ExportAll, moved from internal/worktime to internal/worktime/legacy
// by task e81's core/legacy package split), so worktime.rb keeps a
// report-only-usable view of data that now lives in the store. By default
// this is warn-and-overwrite; see export.go's package doc comment (in
// internal/worktime/legacy) for why. --strict opts into fail-closed
// behavior instead, for operators who'd rather the command refuse than
// silently discard a worktime.rb or hand edit -- see
// legacy.ExportOptions.Strict.
func newExportCmd() *cobra.Command {
	var strict bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Rewrite every host's legacy db.<host>.json from the JSONL store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false,
		"refuse to overwrite db.<host>.json when doing so would discard on-disk entries "+
			"absent from the fresh export, instead of warning and overwriting (default: warn and overwrite)")
	return cmd
}

// runExport opens the runtime (for its store and --db-aware
// cfg.Storage.DBDir), calls ExportAll with the command's stderr as the
// discard-warning sink, and prints a one-line-per-host summary of what
// happened on stdout. Discard warnings themselves already went to stderr
// by the time ExportAll returns, so the summary just points back at them
// rather than repeating their content. In --strict mode a refusal comes
// back as an error wrapping legacy.ErrExportWouldDiscard whose message
// (built by strictDiscardError) already includes the --strict-specific
// remediation hint, so it's returned to cobra as-is.
func runExport(cmd *cobra.Command, strict bool) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	// The discard warning is written by the legacy package, which knows
	// nothing about colors, so it is styled on its way to stderr (task m83).
	warnOut := newColorizeWriter(cmd.ErrOrStderr(), rt.errTheme, ansi.RoleWarn)
	opts := legacy.ExportOptions{Strict: strict, WarnOut: warnOut}
	results, err := legacy.ExportAll(cmdContext(cmd), rt.store, rt.cfg.Storage.DBDir, opts)
	if err != nil {
		return err
	}

	printExportSummary(cmd, rt.theme, results)
	return nil
}

// printExportSummary reports each host's export result on stdout: how many
// entries were written, and -- when ExportHost found stale on-disk entries
// with no counterpart in the fresh export -- how many were discarded, so
// stdout always reflects what happened even for a caller not watching
// stderr. The verb is styled like every other confirmation and the discard
// note as a warning (task m83).
func printExportSummary(cmd *cobra.Command, theme ansi.Theme, results []legacy.ExportResult) {
	for _, r := range results {
		discarded := ""
		if len(r.Discarded) > 0 {
			discarded = theme.Wrap(ansi.RoleWarn,
				fmt.Sprintf(", %d discarded (see warning above)", len(r.Discarded)))
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s entries written%s\n",
			theme.Wrap(ansi.RoleSuccess, "export"), r.Host,
			theme.Wrap(ansi.RoleValue, fmt.Sprintf("%d", r.Written)), discarded)
	}
}
