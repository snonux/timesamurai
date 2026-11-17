package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/ansi"
	"github.com/snonux/timesamurai/internal/worktime"
)

// syncLoginMarker keeps the login marker file (general.login_marker, default
// ~/.wtloggedin) in step with a successful start/stop. The marker used to be
// touched and removed by the fish wrappers around `work start`/`work stop`
// (task a94); owning it here means every entry point -- the hidden
// login/logout aliases and the legacy `--login`/`--logout` shim included --
// behaves the same, whatever shell (or none) invoked timesamurai.
//
// The marker answers "is THIS machine logged in", so it is left alone when
// --host redirects the entry to another machine's database. Failing to
// update the marker only warns: the session entry is already persisted and
// failing the command now would invite a retry that double-books it.
//
// actionErr is the start/stop outcome. Besides success, the two "state
// already as requested" refusals also sync the marker -- ErrAlreadyLoggedIn
// creates it, ErrNotLoggedIn removes it -- so a marker that drifted from the
// store (crash, manual edit, the old wrapper) heals on the next start/stop
// instead of sticking; the old fish wrapper likewise removed it
// unconditionally. Any other error leaves the marker alone.
func syncLoginMarker(cmd *cobra.Command, rt *runtime, verb string, actionErr error) {
	path := rt.cfg.General.LoginMarker
	if path == "" {
		return
	}
	login := verb == "start" || verb == "login"
	switch {
	case actionErr == nil:
	case login && errors.Is(actionErr, worktime.ErrAlreadyLoggedIn):
	case !login && errors.Is(actionErr, worktime.ErrNotLoggedIn):
	default:
		return
	}
	if own, err := resolveHost(rt.cfg); err == nil && rt.host != own {
		return // --host recorded the entry for another machine
	}
	var err error
	if login {
		err = touchMarker(path)
	} else {
		err = removeMarker(path)
	}
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), rt.errTheme.Wrap(ansi.RoleWarn, fmt.Sprintf("warning: login marker %s: %v", path, err)))
	}
}

// touchMarker creates the marker file if missing; an existing one is left
// untouched (its mtime is not meaningful to any reader).
func touchMarker(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

// removeMarker deletes the marker; an already-absent file is the desired
// state, not an error (e.g. `work stop` twice).
func removeMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
