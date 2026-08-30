package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestListShowsEveryEntryAddressedForModifyDelete confirms the default
// (no range, no filters) path lists every entry as a table row leading with
// "<host>:<id>", matching query.go's Row.Address shape so output can be
// pasted straight into a future modify/delete command.
func TestListShowsEveryEntryAddressedForModifyDelete(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-01-05T09:00"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	if _, err := runWork(t, store, "sub", "30m", "lunch", "--at", "2026-01-05T12:00"); err != nil {
		t.Fatalf("work sub: %v", err)
	}

	out, err := runWork(t, store, "list")
	if err != nil {
		t.Fatalf("work list: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		want := fmt.Sprintf("%s:%d", e.Host, e.ID)
		if !strings.Contains(out, want) {
			t.Errorf("list output missing address %q, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "ADDRESS") {
		t.Errorf("list output missing table header, got:\n%s", out)
	}
}

// TestListFiltersByTag confirms --tag narrows rows to entries carrying that
// tag, reusing query.go's Filter/Match rather than any list-local filtering.
func TestListFiltersByTag(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "coding"); err != nil {
		t.Fatalf("work add work: %v", err)
	}
	if _, err := runWork(t, store, "sub", "30m", "lunch", "--descr", "sandwich"); err != nil {
		t.Fatalf("work sub lunch: %v", err)
	}

	out, err := runWork(t, store, "list", "--tag", "lunch")
	if err != nil {
		t.Fatalf("work list --tag lunch: %v", err)
	}
	if strings.Contains(out, "coding") {
		t.Errorf("list --tag lunch should not include the work entry, got:\n%s", out)
	}
	if !strings.Contains(out, "sandwich") {
		t.Errorf("list --tag lunch should include the lunch entry, got:\n%s", out)
	}
}

// TestListMinMaxFiltersByValue confirms --min/--max are parsed as durations
// (not raw seconds) and applied as inclusive Value bounds.
func TestListMinMaxFiltersByValue(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "short"); err != nil {
		t.Fatalf("work add 1h: %v", err)
	}
	if _, err := runWork(t, store, "add", "3h", "work", "--descr", "long"); err != nil {
		t.Fatalf("work add 3h: %v", err)
	}

	out, err := runWork(t, store, "list", "--min", "2h")
	if err != nil {
		t.Fatalf("work list --min 2h: %v", err)
	}
	if strings.Contains(out, "short") {
		t.Errorf("list --min 2h should exclude the 1h entry, got:\n%s", out)
	}
	if !strings.Contains(out, "long") {
		t.Errorf("list --min 2h should include the 3h entry, got:\n%s", out)
	}
}

// TestListRangePositionalFiltersByMonth confirms the optional [range]
// positional (here a YYYY-MM month) narrows results the same way --since/
// --until would, via internal/timefmt.ParseRange.
func TestListRangePositionalFiltersByMonth(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-01-15T09:00", "--descr", "january"); err != nil {
		t.Fatalf("work add january: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-02-15T09:00", "--descr", "february"); err != nil {
		t.Fatalf("work add february: %v", err)
	}

	out, err := runWork(t, store, "list", "2026-01")
	if err != nil {
		t.Fatalf("work list 2026-01: %v", err)
	}
	if !strings.Contains(out, "january") {
		t.Errorf("list 2026-01 should include the january entry, got:\n%s", out)
	}
	if strings.Contains(out, "february") {
		t.Errorf("list 2026-01 should exclude the february entry, got:\n%s", out)
	}
}

// TestListUntilFlagOverridesRangeEnd confirms an explicit --until narrows a
// range's own end, per buildFilter's documented "explicit flags win for the
// bound they set" precedence -- the range still supplies Since.
func TestListUntilFlagOverridesRangeEnd(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-01-10T09:00", "--descr", "early"); err != nil {
		t.Fatalf("work add early: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work", "--at", "2026-01-20T09:00", "--descr", "late"); err != nil {
		t.Fatalf("work add late: %v", err)
	}

	out, err := runWork(t, store, "list", "2026-01", "--until", "2026-01-12")
	if err != nil {
		t.Fatalf("work list 2026-01 --until 2026-01-12: %v", err)
	}
	if !strings.Contains(out, "early") {
		t.Errorf("list with tightened --until should still include the early entry, got:\n%s", out)
	}
	if strings.Contains(out, "late") {
		t.Errorf("list with tightened --until should exclude the late entry, got:\n%s", out)
	}
}

// TestListJSONFormatMatchesQueryShape confirms --format json round-trips
// through worktime.FormatJSON's documented wire shape (address + Entry
// fields), rather than some CLI-local JSON encoding.
func TestListJSONFormatMatchesQueryShape(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "json-check"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "list", "--format", "json")
	if err != nil {
		t.Fatalf("work list --format json: %v", err)
	}

	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", out, err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 JSON row, got %d: %v", len(rows), rows)
	}
	entries := readEntries(t, store, host)
	wantAddress := fmt.Sprintf("%s:%d", entries[0].Host, entries[0].ID)
	if rows[0]["address"] != wantAddress {
		t.Errorf("json address = %v, want %q", rows[0]["address"], wantAddress)
	}
}

// TestListInvalidFormatFails confirms an unrecognized --format value is
// rejected rather than silently falling back to table.
func TestListInvalidFormatFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "list", "--format", "yaml"); err == nil {
		t.Fatal("list --format yaml: want error, got nil")
	}
}

// TestListInvalidRangeFails confirms an unparsable [range] positional
// surfaces internal/timefmt.ParseRange's error rather than being ignored.
func TestListInvalidRangeFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "list", "not-a-range"); err == nil {
		t.Fatal("list not-a-range: want error, got nil")
	}
}

// TestListEmptyStoreShowsOnlyHeader confirms an empty store is not an error:
// list still prints the table header with zero rows.
func TestListEmptyStoreShowsOnlyHeader(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "list")
	if err != nil {
		t.Fatalf("work list on empty store: %v", err)
	}
	if !strings.Contains(out, "ADDRESS") {
		t.Errorf("empty-store list should still print the header, got:\n%s", out)
	}
}
