package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newExportCmd builds `work export`: a forced rewrite of every host's
// legacy db.<host>.json from the current JSONL store (q61's
// worktime.ExportAll), so worktime.rb keeps a report-only-usable view of
// data that now lives in the store. See export.go's package doc comment
// for why this never refuses and never re-imports.
func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Rewrite every host's legacy db.<host>.json from the JSONL store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd)
		},
	}
	return cmd
}

// runExport opens the runtime (for its store and --db-aware
// cfg.Storage.DBDir), calls ExportAll with the command's stderr as the
// discard-warning sink, and prints a one-line-per-host summary of what
// happened on stdout. Discard warnings themselves already went to stderr
// by the time ExportAll returns, so the summary just points back at them
// rather than repeating their content.
func runExport(cmd *cobra.Command) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	results, err := worktime.ExportAll(cmdContext(cmd), rt.store, rt.cfg.Storage.DBDir, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	printExportSummary(cmd, results)
	return nil
}

// printExportSummary reports each host's export result on stdout: how many
// entries were written, and -- when ExportHost found stale on-disk entries
// with no counterpart in the fresh export -- how many were discarded, so
// stdout always reflects what happened even for a caller not watching
// stderr.
func printExportSummary(cmd *cobra.Command, results []worktime.ExportResult) {
	for _, r := range results {
		if len(r.Discarded) > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "export %s: %d entries written, %d discarded (see warning above)\n",
				r.Host, r.Written, len(r.Discarded))
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "export %s: %d entries written\n", r.Host, r.Written)
	}
}
