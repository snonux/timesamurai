package cli

import (
	"fmt"
	"strings"
	"testing"
)

// TestUndoRevertsInsert confirms undo after `work add` removes the entry it
// just wrote and reports it as an "insert" undo.
func TestUndoRevertsInsert(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "undo")
	if err != nil {
		t.Fatalf("work undo: %v", err)
	}
	if got := out; !strings.Contains(got, "undo insert") {
		t.Errorf("output = %q, want it to mention \"undo insert\"", got)
	}
	if len(readEntries(t, store, currentHost(t))) != 0 {
		t.Errorf("undo of an insert should remove the entry")
	}
}

// TestUndoRevertsModify confirms undo after `work modify` restores the
// entry's pre-modify fields.
func TestUndoRevertsModify(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "original"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	addr := firstAddress(t, store)

	if _, err := runWork(t, store, "modify", addr, "--descr", "changed"); err != nil {
		t.Fatalf("work modify: %v", err)
	}
	if got := readEntries(t, store, currentHost(t))[0].Descr; got != "changed" {
		t.Fatalf("descr before undo = %q, want %q", got, "changed")
	}

	out, err := runWork(t, store, "undo")
	if err != nil {
		t.Fatalf("work undo: %v", err)
	}
	if !strings.Contains(out, "undo modify") {
		t.Errorf("output = %q, want it to mention \"undo modify\"", out)
	}
	if got := readEntries(t, store, currentHost(t))[0].Descr; got != "original" {
		t.Errorf("descr after undo = %q, want restored %q", got, "original")
	}
}

// TestUndoRevertsDelete confirms undo after `work delete` restores the
// deleted entry.
func TestUndoRevertsDelete(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "keep me"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	addr := firstAddress(t, store)

	if _, err := runWork(t, store, "delete", addr); err != nil {
		t.Fatalf("work delete: %v", err)
	}
	if len(readEntries(t, store, currentHost(t))) != 0 {
		t.Fatalf("entry should be gone before undo")
	}

	out, err := runWork(t, store, "undo")
	if err != nil {
		t.Fatalf("work undo: %v", err)
	}
	if !strings.Contains(out, "undo delete") {
		t.Errorf("output = %q, want it to mention \"undo delete\"", out)
	}
	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 1 || entries[0].Descr != "keep me" {
		t.Errorf("undo of a delete should restore the entry, got %+v", entries)
	}
}

// TestUndoVerboseShowsBeforeAfter confirms --verbose prints the modify's
// before/after detail line, not just the terse op+address confirmation.
func TestUndoVerboseShowsBeforeAfter(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "original"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	addr := firstAddress(t, store)
	if _, err := runWork(t, store, "modify", addr, "--descr", "changed"); err != nil {
		t.Fatalf("work modify: %v", err)
	}

	out, err := runWork(t, store, "undo", "--verbose")
	if err != nil {
		t.Fatalf("work undo --verbose: %v", err)
	}
	if !strings.Contains(out, "original") || !strings.Contains(out, "changed") {
		t.Errorf("--verbose undo output should show both descriptions, got %q", out)
	}
}

// TestUndoNoRecordFails confirms undo with nothing to undo on this host
// reports an error rather than succeeding silently.
func TestUndoNoRecordFails(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "undo"); err == nil {
		t.Fatal("undo with no undo record: want error, got nil")
	}
}

// firstAddress returns the current host's sole entry's "<host>:<id>"
// address.
func firstAddress(t *testing.T, store string) string {
	t.Helper()
	host := currentHost(t)
	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	return fmt.Sprintf("%s:%d", host, entries[0].ID)
}
