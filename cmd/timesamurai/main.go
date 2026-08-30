package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal"
	"github.com/snonux/timesamurai/internal/cli"
)

func main() {
	// pflag/cobra treat a single-dash "-version" as a shorthand flag
	// cluster (-v -e -r -s -i -o -n) rather than an alias for "--version",
	// so it fails with "unknown shorthand flag e in -ersion" before cobra
	// ever gets a chance to recognize it. Intercept that exact invocation
	// ourselves before handing args to cobra, so both "-version" and
	// "--version" print the same thing.
	if handleLegacyVersionFlag(os.Args[1:], os.Stdout) {
		return
	}
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

// handleLegacyVersionFlag prints the version and reports true when args is
// exactly the single-dash "-version" flag. It only matches that one bare
// token (not combined with other flags or arguments) so it stays a narrow
// compatibility shim rather than a second flag-parsing path competing with
// cobra's own "--version" handling.
func handleLegacyVersionFlag(args []string, out io.Writer) bool {
	if len(args) != 1 || args[0] != "-version" {
		return false
	}
	// Match repo convention: a write error to the version output stream
	// isn't actionable here, so it's discarded rather than propagated.
	_, _ = fmt.Fprintln(out, internal.Version)
	return true
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "timesamurai",
		Short:         "Worktime tracking tool",
		Version:       internal.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	// Define the version flag ourselves so cobra's InitDefaultVersionFlag
	// skips it and does not claim the "-v" shorthand. worktime.rb uses -v
	// for --verbose, and the work subcommands keep that meaning, so leaving
	// -v as an alias for --version here would make the same letter mean two
	// different things one level apart.
	root.Flags().Bool("version", false, "version for timesamurai")

	// --config selects an alternative config.toml. config.LoadOptions has
	// carried ConfigPath since the config rewrite; this is the flag that
	// finally reaches it, matching the precedence documented in
	// docs/configuration.md (flags beat TIMESAMURAI_* beat the file).
	root.PersistentFlags().String("config", "", "path to config.toml (default: $XDG_CONFIG_HOME/timesamurai/config.toml)")

	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(newCompletionCmd())
	root.AddCommand(cli.NewWorkCmd())
	return root
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Args:  cobra.ExactArgs(1),
		ValidArgs: []string{
			"bash", "zsh", "fish", "powershell",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell %q", args[0])
			}
		},
	}
}
