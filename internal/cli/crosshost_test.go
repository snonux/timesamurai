package cli

import (
	"strings"
	"testing"
)

// Entries are addressed <host>:<id> and every read verb already spans hosts,
// but writes used to be pinned to the current machine and undo could only
// reach the current machine's own log. These tests pin the two halves of
// that fix: mutations can target another host, and whatever they did can be
// undone from here.

// TestUndoRevertsCrossHostChange is the safety case: deleting another host's
// entry used to be irreversible from this machine, because undo only read
// the current host's log while the record was written to the target's.
func TestUndoRevertsCrossHostChange(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--host", "othermac", "--descr", "original"); err != nil {
		t.Fatalf("add --host: %v", err)
	}

	if _, err := runWork(t, store, "delete", "othermac:1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if entries := readEntries(t, store, "othermac"); len(entries) != 0 {
		t.Fatalf("delete left %d entries", len(entries))
	}

	// A bare undo must find the record in othermac's log, not this host's.
	out, err := runWork(t, store, "undo")
	if err != nil {
		t.Fatalf("undo after cross-host delete: %v", err)
	}
	if !strings.Contains(out, "othermac:1") {
		t.Errorf("undo output %q does not name the restored cross-host entry", out)
	}
	entries := readEntries(t, store, "othermac")
	if len(entries) != 1 || entries[0].Descr != "original" {
		t.Errorf("entry not restored: %+v", entries)
	}
}

// TestUndoHostFlagTargetsOneLog covers reverting a change some OTHER machine
// made, which a bare undo deliberately will not touch since it only reverts
// this machine's own work.
func TestUndoHostFlagTargetsOneLog(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--host", "othermac"); err != nil {
		t.Fatalf("add --host: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Bare undo takes this machine's most recent action, not othermac's.
	out, err := runWork(t, store, "undo")
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if strings.Contains(out, "othermac") {
		t.Errorf("bare undo reached into another host's work: %q", out)
	}

	// --host reaches it explicitly.
	if out, err = runWork(t, store, "undo", "--host", "othermac"); err != nil {
		t.Fatalf("undo --host: %v", err)
	}
	if !strings.Contains(out, "othermac:1") {
		t.Errorf("undo --host output %q does not name othermac:1", out)
	}
}

// TestHostFlagIsIdentityNotFilter guards the one genuinely confusable thing
// about reusing the name --host: on a mutating verb it says where to WRITE,
// on list/search it only says what to SHOW. Conflating them would silently
// re-identify the machine whenever someone filtered a listing.
func TestHostFlagIsIdentityNotFilter(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--host", "othermac"); err != nil {
		t.Fatalf("add --host: %v", err)
	}
	// Filtering a listing by another host must not move where writes land.
	if _, err := runWork(t, store, "list", "--host", "othermac"); err != nil {
		t.Fatalf("list --host: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("add: %v", err)
	}

	if got := len(readEntries(t, store, host)); got != 1 {
		t.Errorf("current host has %d entries, want 1: --host leaked from the filter into identity", got)
	}
	if got := len(readEntries(t, store, "othermac")); got != 1 {
		t.Errorf("othermac has %d entries, want 1", got)
	}
}

// TestBareIDOnEmptyHostExplainsItself: a bare id resolves against the current
// machine, so on a host that has never tracked anything every lookup failed
// with a plain "not found" that gave no hint the address had been resolved
// somewhere unexpected.
func TestBareIDOnEmptyHostExplainsItself(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--host", "othermac"); err != nil {
		t.Fatalf("add --host: %v", err)
	}

	_, err := runWork(t, store, "modify", "1", "--descr", "x")
	if err == nil {
		t.Fatal("modify of a bare id on an empty host succeeded; want an error")
	}
	for _, want := range []string{"has no entries", "<host>:<id>"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}
