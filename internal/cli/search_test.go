package cli

import (
	"strings"
	"testing"
)

// TestSearchFindsCaseInsensitiveSubstring confirms <text> drives the same
// Descr substring match query.go's Filter.Descr documents as
// case-insensitive, matching "search" reading like a human search rather
// than a case-sensitive grep.
func TestSearchFindsCaseInsensitiveSubstring(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "30m", "lunch", "--descr", "Lunch break at cafe"); err != nil {
		t.Fatalf("work add lunch: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "Team meeting"); err != nil {
		t.Fatalf("work add meeting: %v", err)
	}

	out, err := runWork(t, store, "search", "lunch")
	if err != nil {
		t.Fatalf("work search lunch: %v", err)
	}
	if !strings.Contains(out, "Lunch break at cafe") {
		t.Errorf("search lunch should find the differently-cased match, got:\n%s", out)
	}
	if strings.Contains(out, "Team meeting") {
		t.Errorf("search lunch should not match the meeting entry, got:\n%s", out)
	}
}

// TestSearchCombinesWithTagFilter confirms search shares list's filter
// surface: --tag narrows the search the same way it narrows `work list`.
func TestSearchCombinesWithTagFilter(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "30m", "lunch", "--descr", "quick lunch"); err != nil {
		t.Fatalf("work add lunch: %v", err)
	}
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "quick standup"); err != nil {
		t.Fatalf("work add standup: %v", err)
	}

	out, err := runWork(t, store, "search", "quick", "--tag", "lunch")
	if err != nil {
		t.Fatalf("work search quick --tag lunch: %v", err)
	}
	if !strings.Contains(out, "quick lunch") {
		t.Errorf("search quick --tag lunch should include the lunch entry, got:\n%s", out)
	}
	if strings.Contains(out, "quick standup") {
		t.Errorf("search quick --tag lunch should exclude the standup entry, got:\n%s", out)
	}
}

// TestSearchJSONFormat confirms --format json is honored the same as list.
func TestSearchJSONFormat(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "needle in haystack"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "search", "needle", "--format", "json")
	if err != nil {
		t.Fatalf("work search --format json: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("search --format json should produce a JSON array, got:\n%s", out)
	}
	if !strings.Contains(out, "needle in haystack") {
		t.Errorf("search json output missing matched descr, got:\n%s", out)
	}
}

// TestSearchRequiresNonEmptyText confirms an all-whitespace/empty search
// argument is rejected rather than degenerating into "match everything".
func TestSearchRequiresNonEmptyText(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "search", "   "); err == nil {
		t.Fatal("search with blank text: want error, got nil")
	}
}

// TestSearchMissingArgFails confirms the required <text> positional is
// enforced by cobra.ExactArgs(1).
func TestSearchMissingArgFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "search"); err == nil {
		t.Fatal("search with no text: want error, got nil")
	}
}

// TestSearchFindsNothingReturnsEmptyResultsNotError confirms a search with
// no matches is a normal (zero-row) result, not a failure.
func TestSearchFindsNothingReturnsEmptyResultsNotError(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "unrelated"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "search", "nomatch")
	if err != nil {
		t.Fatalf("work search nomatch: %v", err)
	}
	if strings.Contains(out, "unrelated") {
		t.Errorf("search nomatch should not include the unrelated entry, got:\n%s", out)
	}
}
