package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
)

func TestHasNoColorFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", []string{"work", "report"}, false},
		{"empty", nil, false},
		{"bare flag", []string{"work", "report", "--no-color"}, true},
		{"explicit true", []string{"work", "--no-color=true", "report"}, true},
		{"explicit false", []string{"work", "--no-color=false", "report"}, false},
		{"explicit zero", []string{"--no-color=0"}, false},
		{"garbage value counts as set", []string{"--no-color=maybe"}, true},
		{"after terminator is a positional", []string{"work", "--", "--no-color"}, false},
		{"similar flag is not it", []string{"--no-colors", "--no-colour"}, false},
		{"flag value lookalike", []string{"-d", "no-color"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNoColorFlag(tc.args); got != tc.want {
				t.Errorf("hasNoColorFlag(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestAmbientThemeFollowsTheStandardSwitches drives AmbientTheme through
// every off switch. file is nil (never a terminal), so CLICOLOR_FORCE is
// what turns colors on, exactly as in the command tests.
func TestAmbientThemeFollowsTheStandardSwitches(t *testing.T) {
	newScratchStore(t) // empty XDG_CONFIG_HOME, neutral color env

	if AmbientTheme("", nil, nil).Enabled() {
		t.Error("enabled without a terminal or CLICOLOR_FORCE")
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	if !AmbientTheme("", []string{"work", "report"}, nil).Enabled() {
		t.Error("CLICOLOR_FORCE did not enable the ambient theme")
	}
	if AmbientTheme("", []string{"work", "report", "--no-color"}, nil).Enabled() {
		t.Error("--no-color did not disable the ambient theme")
	}
	t.Setenv("NO_COLOR", "1")
	if AmbientTheme("", nil, nil).Enabled() {
		t.Error("NO_COLOR did not disable the ambient theme")
	}
}

// TestAmbientThemeReadsConfig: report.color=false in the --config file turns
// the ambient theme off, and a [colors] override reaches the error label.
func TestAmbientThemeReadsConfig(t *testing.T) {
	newScratchStore(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	dir := t.TempDir()

	off := filepath.Join(dir, "off.toml")
	writeFile(t, off, "[report]\ncolor = false\n")
	if AmbientTheme(off, nil, nil).Enabled() {
		t.Error("report.color=false did not disable the ambient theme")
	}

	themed := filepath.Join(dir, "themed.toml")
	writeFile(t, themed, "[colors]\nerror = \"bold brightred\"\n")
	got := FormatError(AmbientTheme(themed, nil, nil), errors.New("boom"))
	if want := "\x1b[1;91mError:\x1b[0m boom"; got != want {
		t.Errorf("FormatError with palette override = %q, want %q", got, want)
	}
}

// TestAmbientThemeSurvivesBrokenConfig is the negative case that motivates
// the best-effort design: the config failing to load is likely the very
// error being printed, so the theme must fall back to defaults, not fail.
func TestAmbientThemeSurvivesBrokenConfig(t *testing.T) {
	newScratchStore(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	dir := t.TempDir()

	for name, content := range map[string]string{
		"syntax.toml":  "this is = = not toml [",
		"badrole.toml": "[colors]\nnosuchrole = \"red\"\n",
	} {
		path := filepath.Join(dir, name)
		writeFile(t, path, content)
		got := FormatError(AmbientTheme(path, nil, nil), errors.New("boom"))
		if want := "\x1b[31mError:\x1b[0m boom"; got != want {
			t.Errorf("%s: FormatError = %q, want default-styled %q", name, got, want)
		}
	}
	missing := filepath.Join(dir, "does-not-exist.toml")
	if !AmbientTheme(missing, nil, nil).Enabled() {
		t.Error("missing --config file disabled the ambient theme")
	}
}

func TestFormatErrorPlainWhenDisabled(t *testing.T) {
	if got, want := FormatError(ansi.Theme{}, errors.New("boom")), "Error: boom"; got != want {
		t.Errorf("FormatError = %q, want %q", got, want)
	}
}

// TestColorizeHelpOnRealHelp runs ColorizeHelp over the real `work` help
// text: headings, command names and flags are styled, descriptions are not,
// and the visible text is unchanged.
func TestColorizeHelpOnRealHelp(t *testing.T) {
	store := newScratchStore(t)
	plain, err := runWork(t, store, "--help")
	if err != nil {
		t.Fatalf("work --help: %v", err)
	}
	styled := ColorizeHelp(enabledTheme(t), plain)

	for _, want := range []string{
		"\x1b[1mUsage:\x1b[0m\n",
		"\x1b[1mAvailable Commands:\x1b[0m\n",
		"\x1b[1mFlags:\x1b[0m\n",
		"  \x1b[35mday-off\x1b[0m     Credit a full day off",
		"      \x1b[35m--no-color\x1b[0m ",
		"  \x1b[35m-v, --verbose\x1b[0m ",
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled help missing %q:\n%s", want, styled)
		}
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible help text differs:\n got %q\nwant %q", got, plain)
	}
	if got := ColorizeHelp(disabledTheme(t), plain); got != plain {
		t.Error("disabled theme changed the help text")
	}
}

// TestColorizeHelpLeavesProseAlone: usage lines, descriptions and sentences
// that merely contain a colon or a flag name must stay plain.
func TestColorizeHelpLeavesProseAlone(t *testing.T) {
	theme := enabledTheme(t)
	for _, line := range []string{
		"Track and adjust worked time",
		"  timesamurai work [command]",
		`Use "timesamurai work [command] --help" for more information about a command.`,
		"Note: see --no-color",
		"lowercase heading:",
		"",
	} {
		if got := colorizeHelpLine(theme, line); got != line {
			t.Errorf("colorizeHelpLine(%q) = %q, want unchanged", line, got)
		}
	}
}

// TestInstallColorHelpStylesSubcommandHelp checks the wiring: installed on a
// root, the help func is inherited by subcommands, writes the styled text to
// the command's own output, and resolves the theme only when help runs.
func TestInstallColorHelpStylesSubcommandHelp(t *testing.T) {
	newScratchStore(t)
	run := func(theme ansi.Theme) (string, int) {
		root := &cobra.Command{Use: "timesamurai"}
		root.AddCommand(NewWorkCmd())
		resolved := 0
		InstallColorHelp(root, func() ansi.Theme { resolved++; return theme })
		var out strings.Builder
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"work", "report", "--help"})
		if resolved != 0 {
			t.Fatalf("theme resolved before help ran")
		}
		if err := root.Execute(); err != nil {
			t.Fatalf("help: %v", err)
		}
		return out.String(), resolved
	}

	plain, _ := run(disabledTheme(t))
	if strings.ContainsRune(plain, '\x1b') || !strings.Contains(plain, "Usage:") {
		t.Fatalf("plain help wrong: %q", plain)
	}
	styled, resolved := run(enabledTheme(t))
	if resolved != 1 {
		t.Errorf("theme resolved %d times, want 1", resolved)
	}
	if !strings.Contains(styled, "\x1b[1mUsage:\x1b[0m") {
		t.Errorf("subcommand help not styled: %q", styled)
	}
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible help differs:\n got %q\nwant %q", got, plain)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
