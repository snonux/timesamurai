package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal"
	"github.com/snonux/timesamurai/internal/ansi"
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
	root := newRoot()
	if err := root.Execute(); err != nil {
		reportError(os.Stderr, root, os.Args[1:], err)
		os.Exit(1)
	}
}

// reportError prints the final "Error: ..." line to stderr, its label
// styled by the ambient theme resolved against stderr (see
// internal/cli/ambient.go for why errors cannot use a command's own theme).
// args are the raw arguments root was executed with: after a flag-parsing
// error they are the only place a later --no-color or --config can be seen.
func reportError(stderr *os.File, root *cobra.Command, args []string, err error) {
	theme := cli.AmbientTheme(cli.AmbientInputs{
		ConfigPath: parsedConfigPath(root),
		Args:       args,
		File:       stderr,
	})
	_, _ = fmt.Fprintln(stderr, cli.FormatError(theme, err))
}

// parsedConfigPath returns --config as cobra parsed it, or "" when it was
// not given or parsing never got that far.
func parsedConfigPath(root *cobra.Command) string {
	configPath, _ := root.PersistentFlags().GetString("config")
	return configPath
}

// helpStream is the stream whose terminal-ness decides whether --help is
// colored. A variable only so tests, which capture help in a buffer, are
// independent of whether `go test` itself is attached to a terminal.
var helpStream = os.Stdout

// newRoot builds the command tree main executes: buildRoot's tree with
// colored --help installed.
func newRoot() *cobra.Command {
	root := buildRoot()
	// Help is printed outside any work subcommand's runtime, so it resolves
	// its own best-effort theme (internal/cli/ambient.go) from the flags
	// cobra parsed. --no-color exists as two flag objects, the root's and
	// `work`'s (which shadows it for the work tree), and which one a run
	// sets depends on the invocation: `work list --help --no-color` sets
	// work's, `help work list --no-color` the root's (cobra parses the help
	// command's flags there, not those of the command being helped). Either
	// one turns colors off.
	cli.InstallColorHelp(root, func(cmd *cobra.Command) ansi.Theme {
		onCommand, _ := cmd.Flags().GetBool("no-color")
		onRoot, _ := root.PersistentFlags().GetBool("no-color")
		return cli.AmbientTheme(cli.AmbientInputs{
			ConfigPath: parsedConfigPath(root),
			NoColor:    onCommand || onRoot,
			File:       helpStream,
		})
	})
	return root
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

// buildRoot builds the command tree with cobra's default (plain) help; it
// exists separately from newRoot so tests can compare the two.
func buildRoot() *cobra.Command {
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

	// --no-color is registered here as well as on `work`: now that --help
	// is colored at every level, the off switch has to be accepted at every
	// level too, including `timesamurai --help` and `timesamurai completion
	// --help`.
	cli.RegisterNoColorFlag(root)

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
