package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLegacyLoginLogoutDefaultTag confirms `work --login`/`--logout` with no
// --what default to worktime.WorkTag, matching worktime.rb's
// `opts[:what] = opts[:what] || 'work'`.
func TestLegacyLoginLogoutDefaultTag(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "--login"); err != nil {
		t.Fatalf("work --login: %v", err)
	}
	if _, err := runWork(t, store, "--logout"); err != nil {
		t.Fatalf("work --logout: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 2 || entries[0].Action != "login" || entries[1].Action != "logout" {
		t.Fatalf("want [login logout], got %+v", entries)
	}
	if got := strings.Join(entries[0].Tags, ","); got != "work" {
		t.Errorf("tags = %q, want default \"work\"", got)
	}
}

// TestLegacyWhatSetsTag confirms --what/-w supplies the category for
// login/logout/add/sub/usebuffer, e.g. the exact `--login --what pet`
// invocation task 071 names as the motivating case.
func TestLegacyWhatSetsTag(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "--login", "--what", "pet"); err != nil {
		t.Fatalf("work --login --what pet: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if got := strings.Join(entries[0].Tags, ","); got != "pet" {
		t.Errorf("tags = %q, want \"pet\"", got)
	}
}

// TestLegacyAddSubUsebuffer covers the three credit-shaped legacy flags in
// one pass, each against its own scratch store.
func TestLegacyAddSubUsebuffer(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "--add", "3600", "--what", "work"); err != nil {
		t.Fatalf("work --add 3600: %v", err)
	}
	entries := readEntries(t, store, host)
	if len(entries) != 1 || entries[0].Action != "add" || entries[0].Value != 3600 {
		t.Fatalf("--add: want one add entry of 3600s, got %+v", entries)
	}

	if _, err := runWork(t, store, "--sub", "1800", "--what", "work"); err != nil {
		t.Fatalf("work --sub 1800: %v", err)
	}
	entries = readEntries(t, store, host)
	if len(entries) != 2 || entries[1].Action != "add" || entries[1].Value != -1800 {
		t.Fatalf("--sub: want a second add entry of -1800s, got %+v", entries)
	}

	// usebuffer withdraws from selfdevelopment and credits work, so it needs
	// a selfdevelopment balance to draw down first.
	if _, err := runWork(t, store, "add", "2h", "selfdevelopment"); err != nil {
		t.Fatalf("seed selfdevelopment: %v", err)
	}
	if _, err := runWork(t, store, "--usebuffer", "1800"); err != nil {
		t.Fatalf("work --usebuffer 1800: %v", err)
	}
	entries = readEntries(t, store, host)
	// 2 (add/sub above) + 1 (seed selfdevelopment) + 2 (usebuffer's own
	// withdraw-from-selfdevelopment/credit-to-work pair) = 5.
	if len(entries) != 5 {
		t.Fatalf("--usebuffer: want 5 entries total, got %d: %+v", len(entries), entries)
	}
}

// TestLegacyReportMatchesNoRangeReport confirms `--report` reproduces x61's
// no-range `work report` behavior (full history, not an implicit window).
func TestLegacyReportMatchesNoRangeReport(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "2h", "work", "--at", "2026-01-05T09:00"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "--report")
	if err != nil {
		t.Fatalf("work --report: %v", err)
	}
	if !strings.Contains(out, "20260105") {
		t.Errorf("--report should include the full history, got:\n%s", out)
	}
}

// TestLegacyEditDispatchesToEditFlow confirms --edit reaches y61's edit
// flow: a no-op stub $EDITOR should report "no changes" (the same output
// `work edit` produces on an untouched block), proving the dispatch reused
// edit.go's runEdit rather than a separate copy of its logic.
func TestLegacyEditDispatchesToEditFlow(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	script := filepath.Join(t.TempDir(), "noop-editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o755); err != nil {
		t.Fatalf("write stub editor: %v", err)
	}
	t.Setenv("EDITOR", script)

	out, err := runWork(t, store, "--edit")
	if err != nil {
		t.Fatalf("work --edit: %v", err)
	}
	if !strings.Contains(out, "no changes") {
		t.Errorf("--edit with an untouched block should report \"no changes\", got %q", out)
	}
}

// TestLegacyImportDispatchesToImportFlow confirms --import reaches z61's
// import flow (report.txt-format line parsing and worktime.Add calls),
// using the same day-line shape import_test.go's fixtures use.
func TestLegacyImportDispatchesToImportFlow(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	file := filepath.Join(t.TempDir(), "report.txt")
	content := "Mon 06.01.2026: +8.00h lunch: +0.50h off: +1.00h\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write import fixture: %v", err)
	}

	if _, err := runWork(t, store, "--import", file); err != nil {
		t.Fatalf("work --import: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 3 {
		t.Fatalf("--import: want 3 entries (work/lunch/off), got %d: %+v", len(entries), entries)
	}
}

// TestLegacyHoursFlagAcceptedAndIgnored confirms --hours/-H is accepted
// without error and has no effect on the resulting entry, matching
// worktime.rb declaring but never reading it.
func TestLegacyHoursFlagAcceptedAndIgnored(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "--login", "-H"); err != nil {
		t.Fatalf("work --login -H: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 || entries[0].Action != "login" {
		t.Fatalf("-H should not change login's effect, got %+v", entries)
	}
}

// TestLegacyEpochSetsEntryTime confirms --epoch/-e is read as raw unix
// epoch seconds and converted to the entry's timestamp directly, not routed
// through internal/timefmt's duration/time-string grammar.
func TestLegacyEpochSetsEntryTime(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local)
	epoch := strconv.FormatInt(when.Unix(), 10)

	if _, err := runWork(t, store, "--login", "--epoch", epoch); err != nil {
		t.Fatalf("work --login --epoch %s: %v", epoch, err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Epoch != when.Unix() {
		t.Errorf("epoch = %d, want %d", entries[0].Epoch, when.Unix())
	}
}

// TestLegacyEpochInvalidFails confirms a non-numeric --epoch fails clearly
// rather than being silently misparsed.
func TestLegacyEpochInvalidFails(t *testing.T) {
	store := newScratchStore(t)

	_, err := runWork(t, store, "--login", "--epoch", "not-a-number")
	if err == nil {
		t.Fatal("work --login --epoch not-a-number: want error, got nil")
	}
	if !strings.Contains(err.Error(), "--epoch") {
		t.Errorf("error should name --epoch, got %q", err.Error())
	}
}

// TestLegacyDescrAndVerbosePassThrough confirms --descr/-d attaches to the
// created entry and --verbose/-v (the flag work.go already registers)
// produces the same full-detail output every other mutating verb uses.
func TestLegacyDescrAndVerbosePassThrough(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	out, err := runWork(t, store, "--login", "--descr", "legacy note", "--verbose")
	if err != nil {
		t.Fatalf("work --login --descr --verbose: %v", err)
	}
	if !strings.Contains(out, `descr="legacy note"`) {
		t.Errorf("--verbose output should include the description, got %q", out)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 || entries[0].Descr != "legacy note" {
		t.Fatalf("want descr \"legacy note\", got %+v", entries)
	}
}

// TestLegacyLogFailsWithExplicitError is task 071's core regression guard:
// --log was never a real worktime.rb flag (only --login/--logout exist),
// which is why worktime.fish's worktime::log function has always been
// broken. This must fail with an explicit error naming both real flags
// rather than cobra's generic "unknown flag" or a silent no-op.
func TestLegacyLogFailsWithExplicitError(t *testing.T) {
	store := newScratchStore(t)

	_, err := runWork(t, store, "--log", "--epoch", "1700000000", "--what", "work")
	if err == nil {
		t.Fatal("work --log: want error, got nil")
	}
	if !strings.Contains(err.Error(), "--login") || !strings.Contains(err.Error(), "--logout") {
		t.Errorf("error should name both --login and --logout, got %q", err.Error())
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 0 {
		t.Errorf("--log must not write any entry, got %+v", entries)
	}
}

// TestLegacyPomodoroFailsWithNotSupportedError confirms --pomodoro (Ruby's
// macOS osascript timer, out of scope for this Linux/CLI port) fails
// clearly rather than silently no-op-ing, both bare and with a minutes
// argument. worktime.rb declares --pomodoro as GetoptLong's
// OPTIONAL_ARGUMENT, whose long-option form only attaches a value written
// as "--pomodoro=25" -- a following bare "25" token is a separate
// (non-option) argument, not the flag's value -- so pflag's equivalent
// (NoOptDefVal, set in registerLegacyFlags) is exercised the same way here.
func TestLegacyPomodoroFailsWithNotSupportedError(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "--pomodoro"); err == nil {
		t.Fatal("work --pomodoro: want error, got nil")
	} else if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("bare --pomodoro error should say \"not supported\", got %q", err.Error())
	}

	_, err := runWork(t, store, "--pomodoro=25")
	if err == nil {
		t.Fatal("work --pomodoro=25: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("--pomodoro=25 error should say \"not supported\", got %q", err.Error())
	}
}

// TestLegacyFlagWithRealSubcommandErrors documents the chosen precedence
// between a legacy flag and an explicit new-style subcommand: legacy flags
// are registered as LOCAL flags on the "work" command itself (see
// registerLegacyFlags), not persistent ones, so a real subcommand never
// inherits them. `work start --login` therefore never reaches worklegacy.go
// at all -- cobra routes straight to "start", which doesn't recognize
// "--login" and fails with its own "unknown flag" error. An explicit
// subcommand always wins; this shim only ever fires for a bare `work
// <legacy-flags>` invocation with no subcommand argument.
func TestLegacyFlagWithRealSubcommandErrors(t *testing.T) {
	store := newScratchStore(t)

	_, err := runWork(t, store, "start", "--login")
	if err == nil {
		t.Fatal("work start --login: want error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("want cobra's unknown-flag error for a legacy flag on a real subcommand, got %q", err.Error())
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 0 {
		t.Errorf("a rejected invocation must not write any entry, got %+v", entries)
	}
}

// TestLegacyNoActionFlagShowsHelp confirms a bare `work` invocation, and one
// carrying only modifier flags (--what/--descr/--verbose/--epoch/--hours,
// none of which name an action on their own), falls through to the normal
// help output instead of this shim -- matching worktime.rb, which likewise
// does nothing when no action flag is set.
func TestLegacyNoActionFlagShowsHelp(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "--what", "pet")
	if err != nil {
		t.Fatalf("work --what pet (no action flag): %v", err)
	}
	if !strings.Contains(out, "Track and adjust worked time") {
		t.Errorf("want help output, got %q", out)
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 0 {
		t.Errorf("a modifier-only invocation must not write any entry, got %+v", entries)
	}
}
