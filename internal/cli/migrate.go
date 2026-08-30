package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// newMigrateCmd builds `work migrate [--force]`, the one-shot import of
// legacy db.*.json into the JSONL store (p61's worktime.Migrate). Unlike
// every other work subcommand this does not go through newRuntime: Migrate
// takes dbDir/storeDir directly and opens/writes the store itself, so
// pre-opening it here (as newRuntime would) would just be redundant I/O and
// would also resolve a hostname migrate never uses.
func newMigrateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "One-shot import of legacy db.*.json files into the JSONL store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cmd, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"re-run migration even though the store already has data for a host (development/testing only)")
	return cmd
}

// runMigrate loads --db/--store-aware config and delegates to
// worktime.Migrate, which writes its own human-readable report straight to
// cmd's stdout via MigrateOptions.Report -- so there is nothing left to
// format here. The one thing this wraps is ErrAlreadyMigrated, adding the
// --force hint so a refused migrate tells the operator exactly how to
// override it instead of just repeating the bare "already migrated" text.
func runMigrate(cmd *cobra.Command, force bool) error {
	ctx := cmdContext(cmd)
	cfg, err := loadConfigWithOverrides(ctx, cmd)
	if err != nil {
		return err
	}

	_, err = worktime.Migrate(ctx, cfg.Storage.DBDir, cfg.Storage.StoreDir, worktime.MigrateOptions{
		Force:  force,
		Report: cmd.OutOrStdout(),
	})
	if err != nil {
		if errors.Is(err, worktime.ErrAlreadyMigrated) {
			return fmt.Errorf("%w; pass --force to re-run against a scratch copy", err)
		}
		return err
	}
	return nil
}
