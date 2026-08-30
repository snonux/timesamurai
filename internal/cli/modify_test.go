package cli

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// addOneEntry writes a single "add 1h work" entry (--descr "original") and
// returns its "<host>:<id>" address, the shared setup every modify test
// starts from.
func addOneEntry(t *testing.T, store string) string {
	t.Helper()
	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-01-05T09:00", "--descr", "original"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	host := currentHost(t)
	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	return fmt.Sprintf("%s:%d", host, entries[0].ID)
}

// TestModifyDescrOnlyTouchesDescr confirms an unset --at/--value/--tags/
// --action leaves those fields exactly as they were: buildModifyPatch must
// use Flags().Changed, not "is the flag's string non-empty".
func TestModifyDescrOnlyTouchesDescr(t *testing.T) {
	store := newScratchStore(t)
	addr := addOneEntry(t, store)
	before := readEntries(t, store, currentHost(t))[0]

	if _, err := runWork(t, store, "modify", addr, "--descr", "renamed"); err != nil {
		t.Fatalf("work modify --descr: %v", err)
	}

	after := readEntries(t, store, currentHost(t))[0]
	if after.Descr != "renamed" {
		t.Errorf("descr = %q, want %q", after.Descr, "renamed")
	}
	if after.Epoch != before.Epoch {
		t.Errorf("epoch changed to %d, want unchanged %d", after.Epoch, before.Epoch)
	}
	if after.Value != before.Value {
		t.Errorf("value changed to %d, want unchanged %d", after.Value, before.Value)
	}
	if strings.Join(after.Tags, ",") != strings.Join(before.Tags, ",") {
		t.Errorf("tags changed to %v, want unchanged %v", after.Tags, before.Tags)
	}
	if after.Action != before.Action {
		t.Errorf("action changed to %q, want unchanged %q", after.Action, before.Action)
	}
}

// TestModifyEachFlagIndividually exercises --at, --value, --tags, and
// --action one at a time, confirming each patches only its own field.
func TestModifyEachFlagIndividually(t *testing.T) {
	t.Run("at", func(t *testing.T) {
		store := newScratchStore(t)
		addr := addOneEntry(t, store)
		if _, err := runWork(t, store, "modify", addr, "--at", "2026-02-01T10:00"); err != nil {
			t.Fatalf("work modify --at: %v", err)
		}
		after := readEntries(t, store, currentHost(t))[0]
		wantEpoch := parseLocalTime(t, "2026-02-01T10:00")
		if after.Epoch != wantEpoch {
			t.Errorf("epoch = %d, want %d", after.Epoch, wantEpoch)
		}
		if after.Descr != "original" {
			t.Errorf("descr changed to %q, want unchanged", after.Descr)
		}
	})

	t.Run("value", func(t *testing.T) {
		store := newScratchStore(t)
		addr := addOneEntry(t, store)
		if _, err := runWork(t, store, "modify", addr, "--value", "-30m"); err != nil {
			t.Fatalf("work modify --value: %v", err)
		}
		after := readEntries(t, store, currentHost(t))[0]
		if after.Value != -1800 {
			t.Errorf("value = %d, want -1800", after.Value)
		}
	})

	t.Run("tags", func(t *testing.T) {
		store := newScratchStore(t)
		addr := addOneEntry(t, store)
		if _, err := runWork(t, store, "modify", addr, "--tags", "lunch,offsite"); err != nil {
			t.Fatalf("work modify --tags: %v", err)
		}
		after := readEntries(t, store, currentHost(t))[0]
		want := "lunch,offsite"
		if got := strings.Join(after.Tags, ","); got != want {
			t.Errorf("tags = %q, want %q", got, want)
		}
	})

	t.Run("action", func(t *testing.T) {
		store := newScratchStore(t)
		addr := addOneEntry(t, store)
		// add -> add is a no-op action-wise for validation, so flip to a
		// value that is still valid for the target action: login/logout
		// require Value == 0, so clear value together with action here to
		// stay a legal entry (see the combined-flags test for the same
		// dependency spelled out explicitly).
		if _, err := runWork(t, store, "modify", addr, "--action", "login", "--value", "0"); err != nil {
			t.Fatalf("work modify --action: %v", err)
		}
		after := readEntries(t, store, currentHost(t))[0]
		if after.Action != "login" {
			t.Errorf("action = %q, want login", after.Action)
		}
	})
}

// parseLocalTime parses value with the same "2006-01-02T15:04" layout
// internal/timefmt.ParseTime accepts, returning its Unix seconds, for
// asserting against --at's effect without re-importing timefmt into the
// test.
func parseLocalTime(t *testing.T, value string) int64 {
	t.Helper()
	tm, err := time.ParseInLocation("2006-01-02T15:04", value, time.Local)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return tm.Unix()
}

// TestModifyCombinedFlags sets --at, --value, --descr, --tags, and --action
// together in one call and confirms every field lands.
func TestModifyCombinedFlags(t *testing.T) {
	store := newScratchStore(t)
	addr := addOneEntry(t, store)

	_, err := runWork(t, store, "modify", addr,
		"--at", "2026-03-01T08:00",
		"--value", "2h",
		"--descr", "combined edit",
		"--tags", "work,offsite",
		"--action", "add",
	)
	if err != nil {
		t.Fatalf("work modify (combined): %v", err)
	}

	after := readEntries(t, store, currentHost(t))[0]
	if after.Value != 7200 {
		t.Errorf("value = %d, want 7200", after.Value)
	}
	if after.Descr != "combined edit" {
		t.Errorf("descr = %q, want %q", after.Descr, "combined edit")
	}
	if got := strings.Join(after.Tags, ","); got != "work,offsite" {
		t.Errorf("tags = %q, want %q", got, "work,offsite")
	}
	if after.Action != "add" {
		t.Errorf("action = %q, want add", after.Action)
	}
}

// TestModifyUnknownAddressFails confirms a nonexistent id surfaces
// worktime.ErrEntryNotFound rather than silently doing nothing.
func TestModifyUnknownAddressFails(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "modify", currentHost(t)+":999", "--descr", "x"); err == nil {
		t.Fatal("modify of a nonexistent address: want error, got nil")
	}
}

// TestModifyVerboseShowsUpdatedEntry confirms --verbose prints the entry's
// full fields, not just the terse "<verb> <host>:<id>" confirmation.
func TestModifyVerboseShowsUpdatedEntry(t *testing.T) {
	store := newScratchStore(t)
	addr := addOneEntry(t, store)

	out, err := runWork(t, store, "modify", addr, "--descr", "verbose check", "--verbose")
	if err != nil {
		t.Fatalf("work modify --verbose: %v", err)
	}
	if !strings.Contains(out, `descr="verbose check"`) {
		t.Errorf("--verbose output missing updated descr, got %q", out)
	}
}

// TestModifyRefusesNoOp covers what someone hits when they reach for
// `work modify <addr>` to find out what it takes. It used to succeed
// silently, rewriting the entry identically and leaving a no-op record in
// the undo log. The shell only offers the field flags once a "-" is typed,
// so an error naming them is the discoverable path.
func TestModifyRefusesNoOp(t *testing.T) {
	store := newScratchStore(t)
	addr := addOneEntry(t, store)

	_, err := runWork(t, store, "modify", addr)
	if err == nil {
		t.Fatal("modify with no field flags succeeded; want an error")
	}
	for _, want := range []string{"nothing to change", "--at", "--value", "--descr", "--action", "--tags"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q; got: %v", want, err)
		}
	}

	// --verbose is not a change either: it only affects what is printed.
	if _, err := runWork(t, store, "modify", addr, "--verbose"); err == nil {
		t.Error("modify --verbose alone succeeded; want an error")
	}

	// The entry must be untouched, and no undo record left behind.
	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 1 || entries[0].Descr != "original" {
		t.Errorf("entry was disturbed by the refused modify: %+v", entries)
	}

	// A real field flag still works.
	if _, err := runWork(t, store, "modify", addr, "--value", "2h"); err != nil {
		t.Errorf("modify --value: %v", err)
	}
}
