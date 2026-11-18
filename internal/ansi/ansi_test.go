package ansi

import (
	"os"
	"testing"
)

const (
	esc     = "\x1b["
	reset   = esc + "0m"
	greenSG = esc + "32m"
	boldSG  = esc + "1m"
)

func TestWrapDisabledThemeIsIdentity(t *testing.T) {
	theme, err := NewTheme(false, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	for _, role := range Roles() {
		if got := theme.Wrap(role, "hello"); got != "hello" {
			t.Errorf("disabled theme Wrap(%s) = %q, want plain", role, got)
		}
	}
}

func TestWrapEnabledThemeEmitsSGR(t *testing.T) {
	theme, err := NewTheme(true, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	if got, want := theme.Wrap(RoleSuccess, "ok"), greenSG+"ok"+reset; got != want {
		t.Errorf("Wrap(success) = %q, want %q", got, want)
	}
	if got, want := theme.Wrap(RoleHeader, "H"), boldSG+"H"+reset; got != want {
		t.Errorf("Wrap(header) = %q, want %q", got, want)
	}
	if got := theme.Wrap(RoleSuccess, ""); got != "" {
		t.Errorf("Wrap(success, empty) = %q, want empty", got)
	}
}

func TestWrapUnknownRoleIsPlain(t *testing.T) {
	theme, err := NewTheme(true, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	if got := theme.Wrap("nonsense", "x"); got != "x" {
		t.Errorf("unknown role Wrap = %q, want plain", got)
	}
}

func TestNewThemePaletteOverrides(t *testing.T) {
	theme, err := NewTheme(true, map[string]string{
		RoleError: "bold brightred",
	})
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	if got, want := theme.Wrap(RoleError, "x"), esc+"1;91m"+"x"+reset; got != want {
		t.Errorf("Wrap(error) = %q, want %q", got, want)
	}
	// Non-overridden roles keep their defaults.
	if got, want := theme.Wrap(RoleSuccess, "x"), greenSG+"x"+reset; got != want {
		t.Errorf("Wrap(success) = %q, want %q", got, want)
	}
}

// TestEveryRoleHasADefaultStyle guards against adding a role constant
// without a palette entry (task m83 added key/value/tag): such a role would
// silently render plain, and Roles() would advertise a role that config
// validation accepts but nothing styles.
func TestEveryRoleHasADefaultStyle(t *testing.T) {
	theme, err := NewTheme(true, nil)
	if err != nil {
		t.Fatalf("NewTheme: %v", err)
	}
	palette := DefaultPalette()
	if len(palette) != len(Roles()) {
		t.Errorf("DefaultPalette has %d roles, Roles() lists %d", len(palette), len(Roles()))
	}
	for _, role := range Roles() {
		if got := theme.Wrap(role, "x"); got == "x" {
			t.Errorf("role %q renders plain with the default palette", role)
		}
	}
	for role, want := range map[string]string{
		RoleKey: esc + "35m", RoleValue: esc + "1m", RoleTag: esc + "33m",
	} {
		if got := theme.Wrap(role, "x"); got != want+"x"+reset {
			t.Errorf("Wrap(%s) = %q, want %q", role, got, want+"x"+reset)
		}
	}
}

func TestNewThemeRejectsBadPalette(t *testing.T) {
	if _, err := NewTheme(true, map[string]string{"nope": "red"}); err == nil {
		t.Error("unknown role accepted")
	}
	if _, err := NewTheme(true, map[string]string{RoleSuccess: "chartreuse"}); err == nil {
		t.Error("unknown style accepted")
	}
}

func TestShouldEnablePrecedence(t *testing.T) {
	env := map[string]string{}
	getenv := func(key string) string { return env[key] }

	tests := []struct {
		name     string
		env      map[string]string
		cfg      bool
		forceTTY bool
		want     bool
	}{
		{name: "default enabled on tty", env: nil, cfg: true, forceTTY: true, want: true},
		{name: "config off", env: nil, cfg: false, forceTTY: true, want: false},
		{name: "no tty", env: nil, cfg: true, forceTTY: false, want: false},
		{name: "NO_COLOR wins over everything", env: map[string]string{"NO_COLOR": "1"}, cfg: true, forceTTY: true, want: false},
		{name: "NO_COLOR wins over CLICOLOR_FORCE", env: map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, cfg: true, forceTTY: false, want: false},
		{name: "CLICOLOR_FORCE forces past missing tty", env: map[string]string{"CLICOLOR_FORCE": "1"}, cfg: true, forceTTY: false, want: true},
		{name: "CLICOLOR_FORCE=0 is not a force", env: map[string]string{"CLICOLOR_FORCE": "0"}, cfg: true, forceTTY: false, want: false},
		{name: "CLICOLOR_FORCE does not override config off", env: map[string]string{"CLICOLOR_FORCE": "1"}, cfg: false, forceTTY: false, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clear(env)
			for k, v := range test.env {
				env[k] = v
			}
			var file *os.File
			if test.forceTTY {
				file = openCharDevice(t)
			}
			if got := ShouldEnable(test.cfg, file, getenv); got != test.want {
				t.Errorf("ShouldEnable = %v, want %v", got, test.want)
			}
		})
	}
}

func clear(env map[string]string) {
	for k := range env {
		delete(env, k)
	}
}

// openCharDevice returns an open character device (os.DevNull qualifies:
// it is a char device, which is all IsTerminal checks) so the tty branch of
// ShouldEnable can be exercised without a real terminal. Writing colors to
// /dev/null is harmless, and the file is closed by the test.
func openCharDevice(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
