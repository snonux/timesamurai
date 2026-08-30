package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal"
)

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
