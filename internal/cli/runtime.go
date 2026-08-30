// Package cli builds the Cobra command tree for timesamurai: the "work"
// group (this file's runtime plumbing plus one file per verb family:
// report/list/search, modify/delete/undo/edit, migrate/export/import, the
// worktime.rb flag shim, shell completions).
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/pathutil"
	"github.com/snonux/timesamurai/internal/worktime"
)

// runtime bundles the config, opened store, resolved hostname, and verbose
// flag every work subcommand needs, so each RunE makes one newRuntime(cmd)
// call instead of repeating config.Load/worktime.Open/hostname-resolution
// wiring in every verb file.
type runtime struct {
	cfg     config.Config
	store   *worktime.Store
	host    string
	verbose bool
}

// newRuntime loads configuration (honoring the --db/--store overrides
// registered as persistent flags on the "work" command), opens the JSONL
// store at the resolved directory, and determines which host new entries are
// written under. Called last in each RunE, after cheap argument/flag parsing
// has already had a chance to fail, since opening the store is the one step
// here that touches disk.
func newRuntime(cmd *cobra.Command) (*runtime, error) {
	ctx := cmdContext(cmd)

	cfg, err := loadConfigWithOverrides(ctx, cmd)
	if err != nil {
		return nil, err
	}

	store, err := worktime.Open(ctx, cfg.Storage.StoreDir)
	if err != nil {
		return nil, fmt.Errorf("open store %q: %w", cfg.Storage.StoreDir, err)
	}

	host, err := resolveHost(cfg)
	if err != nil {
		return nil, err
	}
	if override, err := hostIdentityOverride(cmd); err != nil {
		return nil, err
	} else if override != "" {
		host = override
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	return &runtime{cfg: cfg, store: store, host: host, verbose: verbose}, nil
}

// loadConfigWithOverrides loads the sectioned TOML config and then applies
// --db/--store as the highest-precedence override, ahead of config.toml and
// TIMESAMURAI_* env vars (the plan's documented precedence order). These two
// flags exist specifically so commands -- and tests -- can point at a
// scratch directory without touching the real config or ~/git/worktime.
func loadConfigWithOverrides(ctx context.Context, cmd *cobra.Command) (config.Config, error) {
	// --config is a persistent flag on the root command, so it is only
	// present here once cobra has merged inherited flags; a work command
	// built standalone in a test has no such parent and simply gets "".
	configPath, _ := cmd.Flags().GetString("config")
	if configPath != "" {
		expanded, err := pathutil.ExpandHome(configPath)
		if err != nil {
			return config.Config{}, err
		}
		configPath = expanded
	}

	cfg, err := config.Load(ctx, config.LoadOptions{
		ConfigPath:   configPath,
		NoticeWriter: cmd.ErrOrStderr(),
	})
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}

	if db, _ := cmd.Flags().GetString("db"); db != "" {
		expanded, err := pathutil.ExpandHome(db)
		if err != nil {
			return config.Config{}, err
		}
		cfg.Storage.DBDir = expanded
	}
	if store, _ := cmd.Flags().GetString("store"); store != "" {
		expanded, err := pathutil.ExpandHome(store)
		if err != nil {
			return config.Config{}, err
		}
		cfg.Storage.StoreDir = expanded
	}
	return cfg, nil
}

// resolveHost returns cfg.General.Hostname when set (the config's documented
// override), else the OS-reported hostname -- the same fallback worktime.rb
// and the pre-rewrite tool used.
func resolveHost(cfg config.Config) (string, error) {
	if h := strings.TrimSpace(cfg.General.Hostname); h != "" {
		return h, nil
	}
	h, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve hostname: %w", err)
	}
	return h, nil
}

// cmdContext returns cmd.Context(), falling back to context.Background()
// since cobra leaves Context() nil unless the caller used ExecuteContext.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// hostIdentityAnnotation marks a --host flag as redirecting where a mutation
// is RECORDED, as opposed to list/search's --host, which only filters what is
// displayed. Both flags are called --host because that reads correctly in
// either sentence ("add it under the mac", "list the mac's entries"), so the
// annotation, not the name, is what tells newRuntime which one it is looking
// at. Without it, `work list --host mac` would silently re-identify the
// machine as well as filter.
const hostIdentityAnnotation = "timesamurai_host_identity"

// registerHostFlag adds the identity --host flag to a mutating command.
//
// Entries are addressed <host>:<id> and every read verb already spans hosts,
// but every write was pinned to the current machine -- so "I forgot to log
// two hours on the laptop" had no answer short of going to the laptop. This
// closes that asymmetry.
func registerHostFlag(cmd *cobra.Command) {
	cmd.Flags().String("host", "", "record under this host's database instead of the current machine's")
	_ = cmd.Flags().SetAnnotation("host", hostIdentityAnnotation, []string{"true"})
	registerFlagCompletion(cmd, "host", completeHosts)
}

// hostIdentityOverride returns the normalized --host value when the command
// registered one as an identity override, or "" when it did not.
func hostIdentityOverride(cmd *cobra.Command) (string, error) {
	flag := cmd.Flags().Lookup("host")
	if flag == nil || flag.Annotations[hostIdentityAnnotation] == nil {
		return "", nil
	}
	value := strings.TrimSpace(flag.Value.String())
	if value == "" {
		return "", nil
	}
	host, err := worktime.NormalizeHost(value)
	if err != nil {
		return "", fmt.Errorf("--host: %w", err)
	}
	return host, nil
}
