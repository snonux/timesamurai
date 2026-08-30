package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

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
// back as an error wrapping legacy.ErrExportWouldDiscard, which is
// annotated here with the --strict-specific remediation hint before being
// returned to cobra for printing.
func runExport(cmd *cobra.Command, strict bool) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}

	opts := legacy.ExportOptions{Strict: strict, WarnOut: cmd.ErrOrStderr()}
	results, err := legacy.ExportAll(cmdContext(cmd), rt.store, rt.cfg.Storage.DBDir, opts)
	if err != nil {
		if errors.Is(err, legacy.ErrExportWouldDiscard) {
			return fmt.Errorf("%w; drop --strict to overwrite as before", err)
		}
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
func printExportSummary(cmd *cobra.Command, results []legacy.ExportResult) {
	for _, r := range results {
		if len(r.Discarded) > 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "export %s: %d entries written, %d discarded (see warning above)\n",
				r.Host, r.Written, len(r.Discarded))
			continue
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "export %s: %d entries written\n", r.Host, r.Written)
	}
}
