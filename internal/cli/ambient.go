// Ambient styling (task m83): output written outside any work subcommand's
// runtime -- the final "Error:" line main prints and cobra's --help text --
// has no *runtime to take a theme from, and may be printed precisely because
// building one failed. This file resolves a theme for those two surfaces on
// a best-effort basis and styles them.
package cli

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/pathutil"
)

// AmbientInputs is what AmbientTheme resolves a theme from.
type AmbientInputs struct {
	// ConfigPath is --config as cobra parsed it ("" when absent or when
	// parsing never got that far).
	ConfigPath string
	// NoColor is --no-color as cobra parsed it on the command being
	// reported on (false when absent or when parsing never got that far).
	NoColor bool
	// Args are the raw command-line arguments, scanned for --config and
	// --no-color as a fallback for the two parsed values above.
	Args []string
	// File is the stream being written to: os.Stdout for help, os.Stderr
	// for errors. nil counts as "not a terminal".
	File *os.File
}

// AmbientTheme resolves the color theme for help or error output from the
// same inputs a work subcommand uses: NO_COLOR, --no-color, report.color,
// CLICOLOR_FORCE, whether the stream is a terminal, and the [colors]
// palette.
//
// It never fails and never writes anything. The parsed flag values are used
// when cobra produced them, and the raw args are scanned as well because
// the error being reported may be a flag-parsing error, which leaves the
// flags after the offending one unparsed. A config that does not load
// (possibly the very error being reported) falls back to the built-in
// defaults, so a broken config.toml still gets a readable, default-styled
// error.
func AmbientTheme(in AmbientInputs) ansi.Theme {
	configPath := in.ConfigPath
	if configPath == "" {
		configPath = configPathFromArgs(in.Args)
	}
	cfg := loadAmbientConfig(configPath)
	enabled := cfg.Report.Color && !in.NoColor && !hasNoColorFlag(in.Args)
	theme, err := ansi.NewTheme(ansi.ShouldEnable(enabled, in.File, os.Getenv), cfg.Colors.Palette)
	if err != nil {
		// Unreachable for a config that loaded (it validates [colors]), but
		// the zero Theme -- disabled -- is the safe answer regardless.
		return ansi.Theme{}
	}
	return theme
}

// loadAmbientConfig loads the config for AmbientTheme, returning the
// built-in defaults when it cannot be loaded. The load is strictly
// read-only (SkipLegacyMigration): printing help or an error must not
// create config.toml from a legacy config.json as a side effect, nor repeat
// the legacy notice the command's own load already printed.
func loadAmbientConfig(configPath string) config.Config {
	if configPath != "" {
		expanded, err := pathutil.ExpandHome(configPath)
		if err != nil {
			return config.Default()
		}
		configPath = expanded
	}
	cfg, err := config.Load(context.Background(), config.LoadOptions{
		ConfigPath:          configPath,
		NoticeWriter:        io.Discard,
		SkipLegacyMigration: true,
	})
	if err != nil {
		return config.Default()
	}
	return cfg
}

// configPathFromArgs returns the --config value from raw args ("--config
// <path>" or "--config=<path>", before a "--" terminator), or "" when there
// is none. As in pflag, the last occurrence wins. Unlike hasNoColorFlag
// there is no safe direction to err in here, so "--" is simply taken as the
// end of the flags; in the rare case it was a flag's value, a --config
// after it is missed and the default config's colors are used.
func configPathFromArgs(args []string) string {
	path := ""
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--config" && i+1 < len(args) {
			path = args[i+1]
		}
		if value, ok := strings.CutPrefix(arg, "--config="); ok {
			path = value
		}
	}
	return path
}

// hasNoColorFlag reports whether args carries --no-color (or
// --no-color=<true-ish>). Mirrors pflag's bool parsing closely enough for a
// best-effort scan: the last occurrence wins, and an unparsable value
// counts as set, since the user plainly asked for no colors.
//
// A raw scan cannot know what pflag will make of each token: `-d
// --no-color` is a description, and "--" is either the end of the flags or
// just some flag's value. Every such doubt is resolved toward plain output
// -- an uncolored help or error line, never a stray escape sequence: a
// flag's value that reads --no-color counts, and after a "--" the flag can
// still be switched on but no longer off.
func hasNoColorFlag(args []string) bool {
	set, terminated := false, false
	for _, arg := range args {
		if arg == "--" {
			terminated = true
		}
		if arg == "--no-color" {
			set = true
		}
		if value, ok := strings.CutPrefix(arg, "--no-color="); ok {
			switch strings.ToLower(value) {
			case "false", "0", "f":
				set = set && terminated
			default:
				set = true
			}
		}
	}
	return set
}

// FormatError renders the final error line main prints: the "Error:" label
// in the error role, the message itself plain so it stays easy to read and
// to copy.
func FormatError(theme ansi.Theme, err error) string {
	return theme.Wrap(ansi.RoleError, "Error:") + " " + err.Error()
}

// InstallColorHelp makes root (and, through cobra's inheritance, every
// subcommand) render its help text through ColorizeHelp. theme is called
// with the command whose help is being printed, and only when help is
// actually printed, so it can consult flags cobra has parsed by then. Note
// that those are not always cmd's own: `<cmd> --help` parses cmd's flags,
// but `help <cmd>` parses the help command's.
func InstallColorHelp(root *cobra.Command, theme func(cmd *cobra.Command) ansi.Theme) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		_, _ = io.WriteString(cmd.OutOrStdout(), ColorizeHelp(theme(cmd), plainHelp(cmd)))
	})
}

// plainHelp renders cmd's help text exactly as cobra's default help func
// does (cobra's defaultHelpFunc: the long or short description, a blank
// line, then the usage block). It is rebuilt here rather than captured from
// the default help func because that one only writes to cmd's own output:
// capturing it would mean swapping cmd's writer, and cobra offers no way to
// swap it back to "inherit from parent". UsageString is cobra's own
// buffer-returning API and does restore the writer. The equivalence is
// pinned for every command of the real tree by main's
// TestHelpMatchesCobraDefaultForEveryCommand (and for the work tree by
// TestPlainHelpMatchesCobra here), so a cobra upgrade that changes the
// default layout fails loudly instead of drifting.
func plainHelp(cmd *cobra.Command) string {
	var out strings.Builder
	description := cmd.Long
	if description == "" {
		description = cmd.Short
	}
	if description = strings.TrimRightFunc(description, unicode.IsSpace); description != "" {
		out.WriteString(description + "\n\n")
	}
	if cmd.Runnable() || cmd.HasSubCommands() {
		out.WriteString(cmd.UsageString())
	}
	return out.String()
}

// helpHeadingRe matches cobra's section headings ("Usage:", "Available
// Commands:", "Flags:", "Global Flags:", ...): a capitalized phrase alone on
// an unindented line, ending in a colon.
var helpHeadingRe = regexp.MustCompile(`^[A-Z][A-Za-z ]*:$`)

// helpCommandRe matches a command row: two-space indent, the command name,
// then at least two spaces of padding before its description.
var helpCommandRe = regexp.MustCompile(`^(  )([a-z][\w-]*)(\s{2,}\S.*)$`)

// helpFlagRe matches a flag row's leading flag spec: indent, optional
// "-x, " shorthand, and the long "--name".
var helpFlagRe = regexp.MustCompile(`^(\s+)((?:-\w, )?--[\w-]+)(.*)$`)

// ColorizeHelp styles cobra's help/usage text: section headings in the
// header role, and command names and flags in the key role -- but only
// inside the sections that list them ("... Commands:" and "... Flags:"), so
// a Long description or Examples block that happens to contain an indented
// word or a "--flag" is never mistaken for a row. Everything else stays
// plain, and a disabled theme returns help unchanged.
func ColorizeHelp(theme ansi.Theme, help string) string {
	if !theme.Enabled() {
		return help
	}
	lines := strings.Split(help, "\n")
	var row *regexp.Regexp // row shape of the section being read, if any
	for i, line := range lines {
		switch {
		case helpHeadingRe.MatchString(line):
			row = helpSectionRow(line)
			lines[i] = theme.Wrap(ansi.RoleHeader, line)
		case !strings.HasPrefix(line, " ") && line != "":
			// Unindented prose ends the section.
			row = nil
		case row != nil:
			if m := row.FindStringSubmatch(line); m != nil {
				lines[i] = m[1] + theme.Wrap(ansi.RoleKey, m[2]) + m[3]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// helpSectionRow returns the row regex for the rows under heading, or nil
// for a section whose lines are not styled (Usage, Aliases, Examples).
func helpSectionRow(heading string) *regexp.Regexp {
	switch {
	case strings.HasSuffix(heading, "Commands:"):
		return helpCommandRe
	case strings.HasSuffix(heading, "Flags:"):
		return helpFlagRe
	default:
		return nil
	}
}
