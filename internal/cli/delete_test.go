package cli

import (
	"fmt"
	"strings"
	"testing"
)

// addTwoEntries writes two "add" entries and returns their "<host>:<id>"
// addresses in insertion order, the shared setup for delete's tests.
func addTwoEntries(t *testing.T, store string) (string, string) {
	t.Helper()
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "first"); err != nil {
		t.Fatalf("work add first: %v", err)
	}
	if _, err := runWork(t, store, "add", "2h", "work", "--descr", "second"); err != nil {
		t.Fatalf("work add second: %v", err)
	}
	host := currentHost(t)
	entries := readEntries(t, store, host)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	return fmt.Sprintf("%s:%d", host, entries[0].ID), fmt.Sprintf("%s:%d", host, entries[1].ID)
}

// TestDeleteDryRunDoesNotDelete confirms --dry-run previews without calling
// worktime.Delete.
func TestDeleteDryRunDoesNotDelete(t *testing.T) {
	store := newScratchStore(t)
	addr, _ := addTwoEntries(t, store)

	out, err := runWork(t, store, "delete", addr, "--dry-run")
	if err != nil {
		t.Fatalf("work delete --dry-run: %v", err)
	}
	if !strings.Contains(out, "would delete") || !strings.Contains(out, addr) {
		t.Errorf("dry-run output missing preview of %q, got %q", addr, out)
	}
	if len(readEntries(t, store, currentHost(t))) != 2 {
		t.Errorf("--dry-run must not remove any entry")
	}
}

// TestDeleteSingleAddressNeedsNoConfirmation confirms a single-address
// delete proceeds without reading stdin at all (an empty stdin reader would
// make a confirmation read fail/return "no").
func TestDeleteSingleAddressNeedsNoConfirmation(t *testing.T) {
	store := newScratchStore(t)
	addr, other := addTwoEntries(t, store)

	out, err := runWorkWithStdin(t, store, "", "delete", addr)
	if err != nil {
		t.Fatalf("work delete (single, no stdin): %v", err)
	}
	if !strings.Contains(out, "delete "+addr) {
		t.Errorf("output missing confirmation of deleting %q, got %q", addr, out)
	}
	remaining := readEntries(t, store, currentHost(t))
	if len(remaining) != 1 {
		t.Fatalf("want 1 entry left, got %d", len(remaining))
	}
	if got := fmt.Sprintf("%s:%d", currentHost(t), remaining[0].ID); got != other {
		t.Errorf("wrong entry survived: got %q, want %q", got, other)
	}
}

// TestDeleteMultipleAcceptedConfirmationDeletesBoth confirms answering "y"
// to the multi-address prompt proceeds with the delete.
func TestDeleteMultipleAcceptedConfirmationDeletesBoth(t *testing.T) {
	store := newScratchStore(t)
	addr1, addr2 := addTwoEntries(t, store)

	out, err := runWorkWithStdin(t, store, "y\n", "delete", addr1, addr2)
	if err != nil {
		t.Fatalf("work delete (multi, confirmed): %v", err)
	}
	if !strings.Contains(out, "delete "+addr1) || !strings.Contains(out, "delete "+addr2) {
		t.Errorf("output missing confirmation of both deletes, got %q", out)
	}
	if len(readEntries(t, store, currentHost(t))) != 0 {
		t.Errorf("both entries should be gone after confirmed multi-delete")
	}
}

// TestDeleteMultipleDeclinedConfirmationDeletesNothing confirms answering
// anything other than y/yes cancels the whole batch, leaving every entry in
// place.
func TestDeleteMultipleDeclinedConfirmationDeletesNothing(t *testing.T) {
	store := newScratchStore(t)
	addr1, addr2 := addTwoEntries(t, store)

	out, err := runWorkWithStdin(t, store, "n\n", "delete", addr1, addr2)
	if err != nil {
		t.Fatalf("work delete (multi, declined): %v", err)
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("output should report the delete was cancelled, got %q", out)
	}
	if len(readEntries(t, store, currentHost(t))) != 2 {
		t.Errorf("declining confirmation must leave both entries in place")
	}
}

// TestDeleteMultipleNoStdinDefaultsToCancel confirms an empty/EOF stdin (an
// unattended run that never answers) is treated as "no", not an error and
// not an implicit "yes".
func TestDeleteMultipleNoStdinDefaultsToCancel(t *testing.T) {
	store := newScratchStore(t)
	addr1, addr2 := addTwoEntries(t, store)

	_, err := runWorkWithStdin(t, store, "", "delete", addr1, addr2)
	if err != nil {
		t.Fatalf("work delete (multi, empty stdin): %v", err)
	}
	if len(readEntries(t, store, currentHost(t))) != 2 {
		t.Errorf("EOF on the confirmation prompt must leave both entries in place")
	}
}

// TestDeleteReportsEachDeletedEntry confirms a confirmed multi-delete
// reports each entry that actually left the store, not just a count.
func TestDeleteReportsEachDeletedEntry(t *testing.T) {
	store := newScratchStore(t)
	addr1, addr2 := addTwoEntries(t, store)

	out, err := runWorkWithStdin(t, store, "yes\n", "delete", addr1, addr2, "--verbose")
	if err != nil {
		t.Fatalf("work delete (multi, verbose): %v", err)
	}
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Errorf("--verbose delete output missing both entries' descriptions, got %q", out)
	}
}

// TestDeleteUnknownAddressFails confirms deleting a nonexistent id surfaces
// an error rather than silently succeeding.
func TestDeleteUnknownAddressFails(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "delete", currentHost(t)+":999"); err == nil {
		t.Fatal("delete of a nonexistent address: want error, got nil")
	}
}
