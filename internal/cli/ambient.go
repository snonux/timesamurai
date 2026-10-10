// Ambient styling (task m83): output written outside any work subcommand's
// runtime -- the final "Error:" line main prints and cobra's --help text --
// has no *runtime to take a theme from, and may be printed precisely because
// building one failed. This file resolves a theme for those two surfaces on
// a best-effort basis and styles them.
package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/pathutil"
)

// AmbientTheme resolves the color theme for output going to file (os.Stdout
// for help, os.Stderr for errors) from the same inputs a work subcommand
// uses: NO_COLOR, --no-color, report.color, CLICOLOR_FORCE, whether file is
// a terminal, and the [colors] palette.
//
// It never fails. args is scanned for --no-color rather than read from a
// parsed flag set because the error being reported may be a flag-parsing
// error; and a config that does not load (possibly the very error being
// reported) falls back to the built-in defaults, so a broken config.toml
// still gets a readable, default-styled error.
func AmbientTheme(configPath string, args []string, file *os.File) ansi.Theme {
	cfg := loadAmbientConfig(configPath)
	enabled := cfg.Report.Color && !hasNoColorFlag(args)
	theme, err := ansi.NewTheme(ansi.ShouldEnable(enabled, file, os.Getenv), cfg.Colors.Palette)
	if err != nil {
		// Unreachable for a config that loaded (it validates [colors]), but
		// the zero Theme -- disabled -- is the safe answer regardless.
		return ansi.Theme{}
	}
	return theme
}

// loadAmbientConfig loads the config for AmbientTheme, returning the
// built-in defaults when it cannot be loaded. The legacy-config notice is
// discarded: the command's own load already printed it (or will).
func loadAmbientConfig(configPath string) config.Config {
	if configPath != "" {
		expanded, err := pathutil.ExpandHome(configPath)
		if err != nil {
			return config.Default()
		}
		configPath = expanded
	}
	cfg, err := config.Load(context.Background(), config.LoadOptions{
		ConfigPath:   configPath,
		NoticeWriter: io.Discard,
	})
	if err != nil {
		return config.Default()
	}
	return cfg
}

// hasNoColorFlag reports whether args carries --no-color (or
// --no-color=<true-ish>) before a "--" terminator. Mirrors pflag's bool
// parsing closely enough for a best-effort scan: an unparsable value counts
// as set, since the user plainly asked for no colors.
func hasNoColorFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "--no-color" {
			return true
		}
		if value, ok := strings.CutPrefix(arg, "--no-color="); ok {
			switch strings.ToLower(value) {
			case "false", "0", "f":
				return false
			}
			return true
		}
	}
	return false
}

// FormatError renders the final error line main prints: the "Error:" label
// in the error role, the message itself plain so it stays easy to read and
// to copy.
func FormatError(theme ansi.Theme, err error) string {
	return theme.Wrap(ansi.RoleError, "Error:") + " " + err.Error()
}

// InstallColorHelp makes root (and, through cobra's inheritance, every
// subcommand) render its help text through ColorizeHelp. theme is a function
// so the theme is resolved only when help is actually printed, after cobra
// has parsed --config.
func InstallColorHelp(root *cobra.Command, theme func() ansi.Theme) {
	plainHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// Cobra's help func writes straight to cmd's output, so it is
		// pointed at a buffer for the duration of the call. Restoring with
		// the resolved writer (rather than "unset") is equivalent here:
		// help is the last thing a run prints.
		out := cmd.OutOrStdout()
		var plain bytes.Buffer
		cmd.SetOut(&plain)
		plainHelp(cmd, args)
		cmd.SetOut(out)
		_, _ = io.WriteString(out, ColorizeHelp(theme(), plain.String()))
	})
}

// helpHeadingRe matches cobra's section headings ("Usage:", "Available
// Commands:", "Flags:", "Global Flags:", ...): a capitalized phrase alone on
// an unindented line, ending in a colon.
var helpHeadingRe = regexp.MustCompile(`^[A-Z][A-Za-z ]*:$`)

// helpCommandRe matches an "Available Commands" row: two-space indent, the
// command name, then at least two spaces of padding before its description.
var helpCommandRe = regexp.MustCompile(`^(  )([a-z][a-z-]*)(\s{2,}\S.*)$`)

// helpFlagRe matches a flag row's leading flag spec: indent, optional
// "-x, " shorthand, and the long "--name".
var helpFlagRe = regexp.MustCompile(`^(\s+)((?:-\w, )?--[\w-]+)(.*)$`)

// ColorizeHelp styles cobra's help/usage text: section headings in the
// header role, command names and flags in the key role. Everything else
// (descriptions, usage lines) stays plain, and a disabled theme returns help
// unchanged.
func ColorizeHelp(theme ansi.Theme, help string) string {
	if !theme.Enabled() {
		return help
	}
	lines := strings.Split(help, "\n")
	for i, line := range lines {
		lines[i] = colorizeHelpLine(theme, line)
	}
	return strings.Join(lines, "\n")
}

// colorizeHelpLine styles one help line by which of the three shapes it
// has; a line matching none is returned unchanged.
func colorizeHelpLine(theme ansi.Theme, line string) string {
	if helpHeadingRe.MatchString(line) {
		return theme.Wrap(ansi.RoleHeader, line)
	}
	if m := helpFlagRe.FindStringSubmatch(line); m != nil {
		return m[1] + theme.Wrap(ansi.RoleKey, m[2]) + m[3]
	}
	if m := helpCommandRe.FindStringSubmatch(line); m != nil {
		return m[1] + theme.Wrap(ansi.RoleKey, m[2]) + m[3]
	}
	return line
}
