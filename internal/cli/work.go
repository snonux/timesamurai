package cli

import (
	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// NewWorkCmd builds the "timesamurai work" command group: tracking
// (start/stop/status), crediting (add/sub/usebuffer/day-off), and the
// hidden login/logout aliases task w61 asks to keep. Sibling tasks add
// further subcommands to the tree this returns -- x61 (report/list/search),
// y61 (modify/delete/undo/edit), z61 (migrate/export/import), and 071 (the
// worktime.rb flag shim in worklegacy.go) -- so the persistent --db/--store/
// --verbose flags and the newRuntime helper live here rather than per verb,
// for every future subcommand to reuse without re-deriving them.
func NewWorkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Track and adjust worked time",
		Args:  cobra.NoArgs,
	}

	cmd.PersistentFlags().String("db", "", "override the legacy worktime.rb JSON directory (storage.db_dir)")
	cmd.PersistentFlags().String("store", "", "override the JSONL store directory (storage.store_dir)")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "print full entry details instead of a one-line confirmation")

	// 071: worktime.rb's legacy flags (--login, -a TIME, --what, ...) live as
	// local flags on this command itself, so a bare `work --login ...` with
	// no subcommand argument dispatches into worklegacy.go's shim instead of
	// requiring the caller to already know the new `work login` syntax. This
	// only fires when cobra resolves the command line to "work" itself --
	// any real subcommand (`work start`) is routed to before this RunE ever
	// runs, so legacy flags never combine with one (see registerLegacyFlags's
	// doc comment for what happens if a caller tries).
	legacy := registerLegacyFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if legacyActionRequested(legacy) {
			return runLegacy(cmd, legacy)
		}
		return cmd.Help()
	}

	cmd.AddCommand(
		newSessionCmd("start", "Open a work session", false, "start", worktime.Start),
		newSessionCmd("login", "", true, "start", worktime.Start),
		newSessionCmd("stop", "Close a work session", false, "stop", worktime.Stop),
		newSessionCmd("logout", "", true, "stop", worktime.Stop),
		newStatusCmd(),
		newAddCmd(),
		newSubCmd(),
		newUseBufferCmd(),
		newDayOffCmd(),
		// x61: reporting/querying verbs, all read-only and all built on
		// worktime/query.go + report.go rather than reimplementing
		// filtering or rendering here.
		newReportCmd(),
		newListCmd(),
		newSearchCmd(),
		// y61: correction verbs built on worktime.Modify/Delete/UndoLast plus
		// x61's query machinery (list's address format, its buildFilter),
		// so a mistaken entry can be fixed without hand-editing JSONL.
		newModifyCmd(),
		newDeleteCmd(),
		newUndoCmd(),
		newEditCmd(),
		// z61: maintenance verbs built on worktime.Migrate/ExportAll plus a
		// CLI-local port of the pre-rewrite report.txt line parser (see
		// import.go's package-boundary note) for one-shot legacy JSON
		// import, the worktime.rb-parity JSON export, and re-applying an
		// old report.txt as entries.
		newMigrateCmd(),
		newExportCmd(),
		newImportCmd(),
	)
	return cmd
}
