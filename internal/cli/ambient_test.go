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
		// Documented imprecision: a raw scan cannot tell a flag from another
		// flag's value, and deliberately errs toward plain output.
		{"flag value that is literally the flag", []string{"-d", "--no-color"}, true},
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

// TestColorizeHelpLeavesProseAlone: only rows inside a Commands or Flags
// section are styled. Usage lines, descriptions, and row-shaped lines in a
// Long description or an Examples block must stay plain.
func TestColorizeHelpLeavesProseAlone(t *testing.T) {
	theme := enabledTheme(t)
	help := strings.Join([]string{
		"Long description mentioning",
		"  indented   word that looks like a command row",
		"  --looks-like a flag row",
		"lowercase heading:",
		"",
		"Usage:",
		"  timesamurai work [command]",
		"",
		"Examples:",
		"  report   this-week",
		"  --at yesterday",
		"",
		"Flags:",
		"      --real string   a flag",
		"",
		`Use "timesamurai work [command] --help" for more information about a command.`,
		"  trailing   row after prose ended the section",
		"",
	}, "\n")
	want := strings.Join([]string{
		"Long description mentioning",
		"  indented   word that looks like a command row",
		"  --looks-like a flag row",
		"lowercase heading:",
		"",
		"\x1b[1mUsage:\x1b[0m",
		"  timesamurai work [command]",
		"",
		"\x1b[1mExamples:\x1b[0m",
		"  report   this-week",
		"  --at yesterday",
		"",
		"\x1b[1mFlags:\x1b[0m",
		"      \x1b[35m--real\x1b[0m string   a flag",
		"",
		`Use "timesamurai work [command] --help" for more information about a command.`,
		"  trailing   row after prose ended the section",
		"",
	}, "\n")
	if got := ColorizeHelp(theme, help); got != want {
		t.Errorf("ColorizeHelp:\n got %q\nwant %q", got, want)
	}
}

// helpTree returns a fresh root with the work tree attached, like main's.
func helpTree() *cobra.Command {
	root := &cobra.Command{Use: "timesamurai", Short: "Worktime tracking tool"}
	root.AddCommand(NewWorkCmd())
	return root
}

// runHelp executes args against root and returns what it printed.
func runHelp(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

// TestPlainHelpMatchesCobra pins plainHelp (our rebuild of cobra's default
// help layout) against cobra's own default help func for every command in
// the tree, so a cobra upgrade that changes the layout fails here.
func TestPlainHelpMatchesCobra(t *testing.T) {
	newScratchStore(t)
	paths := [][]string{{}, {"work"}}
	for _, sub := range NewWorkCmd().Commands() {
		paths = append(paths, []string{"work", sub.Name()})
	}
	for _, path := range paths {
		args := append(path, "--help")
		want := runHelp(t, helpTree(), args...)

		ours := helpTree()
		InstallColorHelp(ours, func() ansi.Theme { return ansi.Theme{} })
		if got := runHelp(t, ours, args...); got != want {
			t.Errorf("%v: help differs from cobra's default:\n got %q\nwant %q", args, got, want)
		}
	}
}

// TestInstallColorHelpStylesSubcommandHelp checks the wiring: installed on a
// root, the help func is inherited by subcommands, writes the styled text to
// the command's own output, resolves the theme only when help runs, and
// does not pin the subcommand's output writer (a later SetOut on the root
// must still reach it).
func TestInstallColorHelpStylesSubcommandHelp(t *testing.T) {
	newScratchStore(t)
	root := helpTree()
	resolved := 0
	InstallColorHelp(root, func() ansi.Theme { resolved++; return enabledTheme(t) })
	if resolved != 0 {
		t.Fatalf("theme resolved before help ran")
	}

	styled := runHelp(t, root, "work", "report", "--help")
	if resolved != 1 {
		t.Errorf("theme resolved %d times, want 1", resolved)
	}
	if !strings.Contains(styled, "\x1b[1mUsage:\x1b[0m") || !strings.Contains(styled, "\x1b[35m--no-color\x1b[0m") {
		t.Errorf("subcommand help not styled: %q", styled)
	}
	plain := runHelp(t, helpTree(), "work", "report", "--help")
	if got := stripSGR.ReplaceAllString(styled, ""); got != plain {
		t.Errorf("visible help differs:\n got %q\nwant %q", got, plain)
	}

	// Second run on the same tree, into a new writer set on the root.
	if again := runHelp(t, root, "work", "report", "--help"); again != styled {
		t.Errorf("second help run did not reach the root's new writer: %q", again)
	}
}

// TestAmbientThemeNeverWritesConfig is the regression test for a review
// finding: with only a legacy config.json present, a normal config load
// migrates it to config.toml. Printing help or an error must not.
func TestAmbientThemeNeverWritesConfig(t *testing.T) {
	newScratchStore(t)
	dir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "timesamurai")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "config.json"), `{"weekworkhours": 40}`)

	AmbientTheme("", []string{"--help"}, nil)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.json" {
		t.Errorf("ambient theme changed the config directory: %v", entries)
	}
}

// TestAmbientThemeFindsConfigInRawArgs: when cobra never parsed --config
// (a flag error came first), the raw args still select the config file, in
// both spellings, and not past a "--" terminator.
func TestAmbientThemeFindsConfigInRawArgs(t *testing.T) {
	newScratchStore(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	off := filepath.Join(t.TempDir(), "off.toml")
	writeFile(t, off, "[report]\ncolor = false\n")

	for name, args := range map[string][]string{
		"separate value": {"work", "--bogus", "--config", off},
		"equals value":   {"work", "--bogus", "--config=" + off},
	} {
		if AmbientTheme("", args, nil).Enabled() {
			t.Errorf("%s: --config in raw args was ignored", name)
		}
	}
	for name, args := range map[string][]string{
		"after terminator": {"work", "--", "--config", off},
		"missing value":    {"work", "--config"},
		"absent":           {"work", "report"},
	} {
		if !AmbientTheme("", args, nil).Enabled() {
			t.Errorf("%s: theme should use the default config", name)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
