package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal"
)

// TestMain makes the help tests independent of whether `go test` is attached
// to a terminal: help is captured in buffers, but its color decision looks
// at helpStream. With no stream, help is colored only when a test forces it
// (CLICOLOR_FORCE).
func TestMain(m *testing.M) {
	helpStream = nil
	os.Exit(m.Run())
}

func TestRootHelp(t *testing.T) {
	root := newRoot()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--help): %v", err)
	}
	got := out.String()
	for _, want := range []string{"Usage:", "timesamurai", "--version"} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q; got:\n%s", want, got)
		}
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr should be empty on help, got %q", errOut.String())
	}
}

func TestRootVersion(t *testing.T) {
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--version): %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != internal.Version {
		t.Errorf("version = %q, want %q", got, internal.Version)
	}
}

func TestLegacyDashVersion(t *testing.T) {
	var out bytes.Buffer
	if handled := handleLegacyVersionFlag([]string{"-version"}, &out); !handled {
		t.Fatal("expected \"-version\" to be handled")
	}
	got := strings.TrimSpace(out.String())
	if got != internal.Version {
		t.Errorf("version = %q, want %q", got, internal.Version)
	}
}

func TestLegacyDashVersionIgnoresOtherArgs(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"--version"},
		{"-version", "extra"},
		{"work", "-version"},
		{"-versionx"},
	} {
		var out bytes.Buffer
		if handled := handleLegacyVersionFlag(args, &out); handled {
			t.Errorf("args %v: expected not handled, got output %q", args, out.String())
		}
	}
}

func TestRootUnknownFlag(t *testing.T) {
	root := newRoot()
	var errOut bytes.Buffer
	root.SetErr(&errOut)
	root.SetArgs([]string{"--nosuch"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %v, want unknown flag", err)
	}
	// SilenceErrors: cobra must not also write the error to its err writer.
	if errOut.Len() != 0 {
		t.Errorf("err writer should be empty with SilenceErrors; got %q", errOut.String())
	}
}

func TestRootExtraArgs(t *testing.T) {
	root := newRoot()
	var errOut bytes.Buffer
	root.SetErr(&errOut)
	root.SetArgs([]string{"bogus-subcommand"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %v, want unknown command", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("err writer should be empty with SilenceErrors; got %q", errOut.String())
	}
}

func TestCompletionFish(t *testing.T) {
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"completion", "fish"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion fish: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "# fish completion for timesamurai") {
		t.Fatalf("missing fish completion header; got %q", got[:min(120, len(got))])
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatal("fish completion should end with a newline")
	}
}

func TestCompletionUnsupported(t *testing.T) {
	root := newRoot()
	root.SetArgs([]string{"completion", "tcsh"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

// TestRootConfigFlag guards the flag that docs/configuration.md names as the
// highest-precedence config source. config.LoadOptions has always carried
// ConfigPath; the flag reaching it is what makes the documented precedence
// true, so its absence was a documentation/behaviour mismatch rather than a
// missing feature.
func TestRootConfigFlag(t *testing.T) {
	root := newRoot()
	flag := root.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("--config not registered on the root command")
	}
	if flag.Shorthand != "" {
		t.Errorf("--config shorthand = %q, want none", flag.Shorthand)
	}

	// Inherited by subcommands, which is where it is actually read.
	work, _, err := root.Find([]string{"work"})
	if err != nil {
		t.Fatalf("find work command: %v", err)
	}
	if work.InheritedFlags().Lookup("config") == nil {
		t.Error("work does not inherit --config from root")
	}
}

// TestVersionFlagDoesNotClaimShorthandV keeps "-v" meaning --verbose across
// the tool. worktime.rb uses -v for verbose and the work subcommands keep
// that, so cobra's default of binding -v to --version would make the same
// letter mean two different things one level apart.
func TestVersionFlagDoesNotClaimShorthandV(t *testing.T) {
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--version): %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != internal.Version {
		t.Fatalf("--version printed %q, want %q", got, internal.Version)
	}
	if sh := root.Flags().ShorthandLookup("v"); sh != nil {
		t.Errorf("-v is bound to %q on root; it must stay free for --verbose", sh.Name)
	}
}

// hermeticColorEnv points config at an empty directory and neutralizes the
// ambient color variables, so the color tests below see only what they set.
func hermeticColorEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("TIMESAMURAI_COLOR", "")
}

// runRoot executes a fresh root with args and returns its combined output.
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// TestRootHelpColoredWhenForcedAndOffSwitchesWork covers newRoot's help
// wiring (task m83): forced colors style the help at every level, the
// visible text is unchanged, and NO_COLOR / report.color=false turn it off.
func TestRootHelpColoredWhenForcedAndOffSwitchesWork(t *testing.T) {
	hermeticColorEnv(t)
	for _, args := range [][]string{{"--help"}, {"completion", "--help"}, {"work", "list", "--help"}} {
		plain, err := runRoot(t, args...)
		if err != nil || strings.ContainsRune(plain, '\x1b') {
			t.Fatalf("%v plain: err=%v out=%q", args, err, plain)
		}
		t.Setenv("CLICOLOR_FORCE", "1")
		styled, err := runRoot(t, args...)
		if err != nil || !strings.Contains(styled, "\x1b[1mUsage:\x1b[0m") {
			t.Errorf("%v forced: err=%v, help not styled: %q", args, err, styled)
		}
		if got := stripEscapes(styled); got != plain {
			t.Errorf("%v: visible help differs:\n got %q\nwant %q", args, got, plain)
		}
		t.Setenv("NO_COLOR", "1")
		if out, _ := runRoot(t, args...); out != plain {
			t.Errorf("%v: NO_COLOR help differs from plain: %q", args, out)
		}
		t.Setenv("NO_COLOR", "")
		t.Setenv("TIMESAMURAI_COLOR", "false")
		if out, _ := runRoot(t, args...); out != plain {
			t.Errorf("%v: report.color=false help differs from plain: %q", args, out)
		}
		t.Setenv("TIMESAMURAI_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
	}
}

// TestNoColorFlagUncolorsHelpAtEveryLevel: --no-color must parse on the
// root and on non-work commands too, since their help is colored as well
// (it used to exist only under `work`, making `timesamurai --help
// --no-color` an unknown-flag error) -- and, through newRoot's own wiring,
// it must actually win over CLICOLOR_FORCE wherever it is placed.
func TestNoColorFlagUncolorsHelpAtEveryLevel(t *testing.T) {
	hermeticColorEnv(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, args := range [][]string{
		{"--no-color", "--help"},
		{"--help", "--no-color"},
		{"completion", "--no-color", "--help"},
		{"work", "--no-color", "list", "--help"},
		{"--no-color", "work", "list", "--help"},
		{"work", "list", "--help", "--no-color=true"},
		{"--no-color"},
	} {
		out, err := runRoot(t, args...)
		if err != nil || !strings.Contains(out, "Usage:") {
			t.Errorf("%v: err=%v out=%q", args, err, out)
		}
		if strings.ContainsRune(out, '\x1b') {
			t.Errorf("%v: help still colored: %q", args, out)
		}
	}
	// Negatives: an explicit false keeps the forced colors, and a
	// misspelled variant is still rejected.
	if out, err := runRoot(t, "--no-color=false", "--help"); err != nil || !strings.ContainsRune(out, '\x1b') {
		t.Errorf("--no-color=false: err=%v, help not colored: %q", err, out)
	}
	if _, err := runRoot(t, "--no-colour", "--help"); err == nil {
		t.Error("--no-colour was accepted")
	}
}

// commandPaths returns the argument path of every command in cmd's tree,
// hidden ones and cobra's generated help/completion commands included.
func commandPaths(cmd *cobra.Command, prefix []string) [][]string {
	paths := [][]string{prefix}
	for _, sub := range cmd.Commands() {
		paths = append(paths, commandPaths(sub, append(append([]string{}, prefix...), sub.Name()))...)
	}
	return paths
}

// TestHelpMatchesCobraDefaultForEveryCommand compares the real tree's help
// (newRoot, colors off) with the same tree's help under cobra's default
// help func (buildRoot) for every command, both as `<cmd> --help` and as
// `help <cmd>`. cli.plainHelp rebuilds cobra's layout by hand; this is what
// makes a cobra upgrade that changes it fail loudly.
func TestHelpMatchesCobraDefaultForEveryCommand(t *testing.T) {
	hermeticColorEnv(t)
	run := func(root *cobra.Command, args []string) string {
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return out.String()
	}
	// Execute once so cobra adds its generated help command to the tree.
	probe := buildRoot()
	run(probe, []string{"--help"})
	paths := commandPaths(probe, nil)
	if len(paths) < 20 {
		t.Fatalf("only %d commands found; the tree walk is broken", len(paths))
	}
	for _, path := range paths {
		for _, args := range [][]string{
			append(append([]string{}, path...), "--help"),
			append([]string{"help"}, path...),
		} {
			want := run(buildRoot(), args)
			if got := run(newRoot(), args); got != want {
				t.Errorf("%v: help differs from cobra's default:\n got %q\nwant %q", args, got, want)
			}
			if !strings.Contains(want, "Usage:") {
				t.Errorf("%v: not a help text: %q", args, want)
			}
		}
	}
}

// TestMachineReadableOutputNeverColored: version, completion scripts and
// the shell-completion protocol must stay escape-free with colors forced.
func TestMachineReadableOutputNeverColored(t *testing.T) {
	hermeticColorEnv(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, args := range [][]string{
		{"--version"},
		{"completion", "fish"},
		{"completion", "bash"},
		{"__complete", "work", ""},
		{"__completeNoDesc", "work", "re"},
	} {
		out, err := runRoot(t, args...)
		if err != nil {
			t.Errorf("%v: %v", args, err)
		}
		if out == "" || strings.ContainsRune(out, '\x1b') {
			t.Errorf("%v: output empty or colored: %q", args, out)
		}
	}
}

// TestReportError covers the final error line: plain by default (the file
// is not a terminal), red label when forced, and plain again with
// --no-color in the raw args even though parsing failed before reaching it.
func TestReportError(t *testing.T) {
	hermeticColorEnv(t)
	report := func(args ...string) string {
		t.Helper()
		file, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		root := newRoot()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetArgs(args)
		runErr := root.Execute()
		if runErr == nil {
			t.Fatalf("%v: expected an error", args)
		}
		reportError(file, root, args, runErr)
		written, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(written)
	}
	const message = `unknown command "bogus" for "timesamurai work"` + "\n"

	if got := report("work", "bogus"); got != "Error: "+message {
		t.Errorf("plain error = %q", got)
	}
	t.Setenv("CLICOLOR_FORCE", "1")
	if got := report("work", "bogus"); got != "\x1b[31mError:\x1b[0m "+message {
		t.Errorf("forced error = %q", got)
	}
	if got := report("work", "--nosuchflag", "--no-color"); !strings.HasPrefix(got, "Error: unknown flag") {
		t.Errorf("--no-color after a flag error still colored: %q", got)
	}
	// Last occurrence wins, as in pflag: an earlier =false does not undo it.
	if got := report("--no-color=false", "work", "--nosuchflag", "--no-color"); !strings.HasPrefix(got, "Error: unknown flag") {
		t.Errorf("later --no-color lost to an earlier --no-color=false: %q", got)
	}
}

// stripEscapes removes SGR escape sequences.
func stripEscapes(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}
