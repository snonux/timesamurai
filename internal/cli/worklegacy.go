package cli

// TEMPORARY SHIM -- DELETE WITH RUBY.
//
// This file exists only so a caller still using the old worktime.rb flag
// surface -- most notably ~/git/dotfiles/fish/conf.d/worktime.fish, which
// invokes `worktime --login/--logout/--add/... --what ...` -- keeps working
// against this Go binary without first learning the new `work <verb>`
// subcommand syntax (task 071). It adds no behavior of its own: every branch
// below just translates one legacy flag into the same worktime.Start/Stop/
// Add/Sub/UseBuffer calls, or the same runReport/runEdit/runImport helpers,
// that session.go/credit.go/report.go/edit.go/import.go already expose in
// this package. Once worktime.rb is retired this file (and its flag
// registration call in work.go) can be deleted in one piece -- nothing else
// in internal/cli references it.
//
// worktime.rb's flag section (GetoptLong, all OPTIONAL_ARGUMENT) is the
// source of truth this shim mirrors; see worktime.rb lines ~386-432.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// legacyFlags holds every worktime.rb flag this shim recognizes, bound to
// local (non-persistent) flags on the "work" command itself. Local rather
// than persistent is deliberate: it means a real subcommand (`work start
// --login`) never inherits these names, so combining legacy flags with an
// explicit subcommand fails with cobra's own "unknown flag" error instead of
// silently doing something the subcommand never asked for -- see
// registerLegacyFlags's doc comment for the precedence this produces.
type legacyFlags struct {
	login      bool
	logout     bool
	add        string
	sub        string
	usebuffer  string
	what       string
	report     bool
	hours      bool // declared for parity, always ignored (see registerLegacyFlags)
	epoch      string
	edit       bool
	importFile string
	descr      string
	pomodoro   string // presence alone is enough; value is never used (see runLegacy)
	log        bool
}

// registerLegacyFlags adds worktime.rb's flag surface to cmd (the "work"
// command) as local flags, and returns the struct RunE reads them back
// through. --verbose/-v is deliberately NOT redeclared here: work.go already
// registers it as a persistent flag with the same short name, and this
// shim's dispatchers read it via rt.verbose exactly like every other verb.
//
// Precedence with a real subcommand: because these are local (not
// persistent) flags, `work start --login` never reaches this file at all --
// cobra routes straight to the "start" subcommand, which doesn't recognize
// "--login" and errors accordingly. Only a bare `work --login ...` with no
// subcommand argument lands in the parent command's own RunE, which is
// exactly where runLegacy below is wired in from work.go.
func registerLegacyFlags(cmd *cobra.Command) *legacyFlags {
	lf := &legacyFlags{}
	flags := cmd.Flags()
	flags.BoolVarP(&lf.login, "login", "l", false, "legacy: start a session (see 'work start')")
	flags.BoolVarP(&lf.logout, "logout", "o", false, "legacy: stop a session (see 'work stop')")
	flags.StringVarP(&lf.add, "add", "a", "", "legacy: credit a duration in seconds (see 'work add')")
	flags.StringVarP(&lf.sub, "sub", "s", "", "legacy: withdraw a duration in seconds (see 'work sub')")
	flags.StringVarP(&lf.usebuffer, "usebuffer", "u", "", "legacy: use buffer hours (see 'work usebuffer')")
	flags.StringVarP(&lf.what, "what", "w", "", "legacy: category/tag for login/logout/add/sub (default \"work\")")
	flags.BoolVarP(&lf.report, "report", "r", false, "legacy: print the full report (see 'work report')")
	// --hours/-H: worktime.rb declares this but never reads it (grep the
	// script -- opts[:hours] is set, never consumed). Accepted here purely so
	// a caller passing it doesn't get an "unknown flag" error; it does
	// nothing, on purpose, matching the Ruby original.
	flags.BoolVarP(&lf.hours, "hours", "H", false, "legacy: accepted and ignored (unused in worktime.rb too)")
	flags.StringVarP(&lf.epoch, "epoch", "e", "", "legacy: raw unix epoch seconds for the entry (default now)")
	flags.BoolVarP(&lf.edit, "edit", "E", false, "legacy: edit entries in $EDITOR (see 'work edit')")
	flags.StringVarP(&lf.importFile, "import", "i", "", "legacy: import a report.txt file (see 'work import')")
	flags.StringVarP(&lf.descr, "descr", "d", "", "legacy: free-text description for the entry")
	// --pomodoro/-p: worktime.rb's optional-argument flag, so pflag needs
	// NoOptDefVal to accept a bare "-p" with no minutes the same way
	// GetoptLong::OPTIONAL_ARGUMENT does. The value itself is never read --
	// see runLegacy's pomodoroNotSupported -- only whether the flag was used.
	flags.StringVarP(&lf.pomodoro, "pomodoro", "p", "", "legacy: not supported by this port (see runLegacy)")
	flags.Lookup("pomodoro").NoOptDefVal = "0"
	// --log: never a real worktime.rb flag (only --login/--logout exist), but
	// worktime.fish's worktime::log function has always called it, so it must
	// be recognized here rather than left to fail with a generic "unknown
	// flag" -- see legacyLogError for why that ambiguity gets an explicit
	// named error instead.
	flags.BoolVar(&lf.log, "log", false, "always errors: ambiguous, use --login or --logout")
	hideLegacyFlags(flags)
	return lf
}

// hideLegacyFlags marks every flag this shim registers as Hidden, the same
// way work.go's login/logout subcommand aliases are Hidden cobra.Commands:
// `work --help` should show the current, non-legacy surface, not fourteen
// backward-compat flags a new user was never meant to discover this way.
// The flags still parse and work identically when actually typed -- Hidden
// only affects whether they're listed.
func hideLegacyFlags(flags *pflag.FlagSet) {
	for _, name := range []string{
		"login", "logout", "add", "sub", "usebuffer", "what", "report",
		"hours", "epoch", "edit", "import", "descr", "pomodoro", "log",
	} {
		_ = flags.MarkHidden(name)
	}
}

// legacyActionRequested reports whether any flag that names an action
// (as opposed to a modifier like --what/--descr/--epoch/--hours) was
// explicitly passed. work.go's RunE uses this to decide whether a bare
// `work` invocation should dispatch into this shim or fall back to printing
// help -- the same "no action requested" case worktime.rb itself leaves a
// silent no-op for.
func legacyActionRequested(lf *legacyFlags) bool {
	return lf.login || lf.logout || lf.add != "" || lf.sub != "" ||
		lf.usebuffer != "" || lf.report || lf.edit || lf.importFile != "" ||
		lf.pomodoro != "" || lf.log
}

// runLegacy is work.go's dispatch entry point once legacyActionRequested has
// confirmed a legacy invocation. --log and --pomodoro are validated first,
// before anything touches the store, deliberately departing from
// worktime.rb's own control flow (which runs actions in flag order and would
// only ever hit an unimplemented/ambiguous flag's effect at that flag's own
// position, potentially after other actions already mutated the store): a
// clearly invalid or unsupported invocation should fail before it does
// anything, not after a partial sequence of side effects.
func runLegacy(cmd *cobra.Command, lf *legacyFlags) error {
	if lf.log {
		return legacyLogError()
	}
	if lf.pomodoro != "" {
		return legacyPomodoroError()
	}
	return runLegacyActions(cmd, lf)
}

// legacyLogError is the explicit, named error task 071 asks for in place of
// --log's historical silent misbehavior: worktime.rb never declared --log as
// an option at all, so GetoptLong fell through to an ambiguous partial match
// in some undefined way, which is why worktime.fish's worktime::log function
// has always been broken. Rather than reproduce that ambiguity (or leave
// cobra's generic "unknown flag" to speak for it), name both real flags so
// the fix is obvious from the error alone.
func legacyLogError() error {
	return fmt.Errorf("--log is ambiguous: use --login or --logout")
}

// legacyPomodoroError explains why --pomodoro is rejected rather than
// silently ignored: worktime.rb's pomodoro timer drives macOS `osascript`
// dialogs, which have no equivalent on this Linux/CLI port, and this CLI
// has no timer verb to reuse instead. A loud, explicit rejection is safer
// than a flag that parses fine and then does nothing a caller would notice.
func legacyPomodoroError() error {
	return fmt.Errorf("--pomodoro is not supported by this port: worktime.rb's pomodoro timer used macOS osascript dialogs, which have no CLI equivalent here")
}

// runLegacyActions runs each requested action in worktime.rb's own order
// (login, logout, add, sub, usebuffer, edit, report, import), stopping at
// the first error -- matching the Ruby script's behavior of crashing on the
// first uncaught exception rather than continuing past a failed step.
func runLegacyActions(cmd *cobra.Command, lf *legacyFlags) error {
	tags, at, err := legacyTagsAndTime(lf)
	if err != nil {
		return err
	}
	if err := runLegacySessionActions(cmd, lf, tags, at); err != nil {
		return err
	}
	if err := runLegacyCreditActions(cmd, lf, tags, at); err != nil {
		return err
	}
	if lf.edit {
		if err := runEdit(cmd, ""); err != nil {
			return err
		}
	}
	if lf.report {
		if err := runReport(cmd, nil); err != nil {
			return err
		}
	}
	if lf.importFile != "" {
		if err := runImport(cmd, lf.importFile); err != nil {
			return err
		}
	}
	return nil
}

// legacyTagsAndTime resolves --what into the single-tag slice Start/Stop/
// Add/Sub take (defaulting to worktime.WorkTag, matching worktime.rb's
// `opts[:what] = opts[:what] || 'work'`) and --epoch into a time.Time.
func legacyTagsAndTime(lf *legacyFlags) ([]string, time.Time, error) {
	what := lf.what
	if what == "" {
		what = worktime.WorkTag
	}
	at, err := legacyEpochTime(lf.epoch)
	if err != nil {
		return nil, time.Time{}, err
	}
	return []string{what}, at, nil
}

// legacyEpochTime parses --epoch as raw unix epoch seconds, per task 071's
// instruction to treat it as an epoch integer rather than routing it through
// internal/timefmt's duration/time-string grammar. An empty value (--epoch
// not given) returns the zero time.Time, which every worktime mutation
// already treats as "use time.Now()" (see output.go's parseAtFlag).
func legacyEpochTime(epoch string) (time.Time, error) {
	if epoch == "" {
		return time.Time{}, nil
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(epoch), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("--epoch: %q is not a unix epoch integer: %w", epoch, err)
	}
	return time.Unix(seconds, 0), nil
}

// runLegacySessionActions handles --login/--logout. It reuses session.go's
// own sessionAction type (the shape shared by worktime.Start/Stop) rather
// than declaring a new one, and dispatches both through runLegacyMutation so
// the login/logout blocks below don't duplicate each other's error-wrap/
// print steps.
func runLegacySessionActions(cmd *cobra.Command, lf *legacyFlags, tags []string, at time.Time) error {
	if !lf.login && !lf.logout {
		return nil
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	ctx := cmdContext(cmd)
	if lf.login {
		if err := runLegacyMutation(ctx, cmd, rt, "login", tags, at, lf.descr, worktime.Start); err != nil {
			return err
		}
	}
	if lf.logout {
		if err := runLegacyMutation(ctx, cmd, rt, "logout", tags, at, lf.descr, worktime.Stop); err != nil {
			return err
		}
	}
	return nil
}

// runLegacyMutation calls a sessionAction (worktime.Start or worktime.Stop)
// and reports the result the same way every other mutating verb in this
// package does, via printEntryResult.
func runLegacyMutation(ctx context.Context, cmd *cobra.Command, rt *runtime, verb string, tags []string, at time.Time, descr string, action sessionAction) error {
	entry, err := action(ctx, rt.store, rt.cfg.Accounting, rt.host, tags, at, descr)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	printEntryResult(cmd, verb, entry, rt.verbose)
	return nil
}

// runLegacyCreditActions handles --add/--sub/--usebuffer. --add/--sub reuse
// credit.go's own creditAction type (the shape shared by worktime.Add/Sub)
// through runLegacyCredit; --usebuffer returns a slice of entries instead of
// one, so it gets its own small helper rather than being forced into that
// shape.
func runLegacyCreditActions(cmd *cobra.Command, lf *legacyFlags, tags []string, at time.Time) error {
	if lf.add == "" && lf.sub == "" && lf.usebuffer == "" {
		return nil
	}
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	ctx := cmdContext(cmd)
	if lf.add != "" {
		if err := runLegacyCredit(ctx, cmd, rt, "add", lf.add, tags, at, lf.descr, worktime.Add); err != nil {
			return err
		}
	}
	if lf.sub != "" {
		if err := runLegacyCredit(ctx, cmd, rt, "sub", lf.sub, tags, at, lf.descr, worktime.Sub); err != nil {
			return err
		}
	}
	if lf.usebuffer != "" {
		if err := runLegacyUseBuffer(ctx, cmd, rt, lf.usebuffer, at, lf.descr); err != nil {
			return err
		}
	}
	return nil
}

// runLegacyCredit parses raw as a duration (internal/timefmt.ParseDuration,
// the same parser credit.go's runCredit uses -- a bare integer like
// worktime.rb's "3600" is already its bare-seconds case), calls action, and
// reports the result.
func runLegacyCredit(ctx context.Context, cmd *cobra.Command, rt *runtime, verb, raw string, tags []string, at time.Time, descr string, action creditAction) error {
	duration, err := timefmt.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("--%s: %w", verb, err)
	}
	entry, err := action(ctx, rt.store, rt.cfg.Accounting, rt.host, tags, duration, at, descr)
	if err != nil {
		return fmt.Errorf("%s: %w", verb, err)
	}
	printEntryResult(cmd, verb, entry, rt.verbose)
	return nil
}

// runLegacyUseBuffer parses raw as a duration and calls worktime.UseBuffer,
// the same primitive credit.go's runUseBuffer calls. Kept separate from
// runLegacyCredit because UseBuffer returns multiple entries (a withdraw and
// a credit) rather than one.
func runLegacyUseBuffer(ctx context.Context, cmd *cobra.Command, rt *runtime, raw string, at time.Time, descr string) error {
	duration, err := timefmt.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("--usebuffer: %w", err)
	}
	entries, err := worktime.UseBuffer(ctx, rt.store, rt.cfg.Accounting, rt.host, duration, at, descr)
	if err != nil {
		return fmt.Errorf("usebuffer: %w", err)
	}
	printEntriesResult(cmd, "usebuffer", entries, rt.verbose)
	return nil
}
