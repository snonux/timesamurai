package worktime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// queryFixture returns a small, deliberately varied entry set (two hosts,
// mixed actions/tags/values/descriptions) that every test below filters
// against, so each test only needs to say which entries it expects back.
func queryFixture() []Entry {
	return []Entry{
		{ID: 1, Action: "login", Epoch: 1000, Host: "earth", Tags: []string{"work"}, Descr: "Morning standup"},
		{ID: 2, Action: "logout", Epoch: 2000, Host: "earth", Tags: []string{"work"}},
		{ID: 3, Action: "add", Epoch: 3000, Host: "earth", Tags: []string{"lunch"}, Value: 1800, Descr: "Lunch Break"},
		{ID: 1, Action: "add", Epoch: 4000, Host: "mars", Tags: []string{"work", "offsite"}, Value: -600, Descr: "Fixed CI outage"},
		{ID: 2, Action: "add", Epoch: 5000, Host: "mars", Tags: []string{"selfdevelopment"}, Value: 900, Descr: "Reading"},
	}
}

func addressesOf(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Address
	}
	return out
}

func mustQuery(t *testing.T, entries []Entry, f Filter) []Row {
	t.Helper()
	rows, err := Query(entries, f)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	return rows
}

func TestFilter_MatchSingleConditions(t *testing.T) {
	entries := queryFixture()

	tests := []struct {
		name string
		f    Filter
		want []string
	}{
		{"empty filter matches everything", Filter{}, []string{"earth:1", "earth:2", "earth:3", "mars:1", "mars:2"}},
		{"host exact match", Filter{Host: "mars"}, []string{"mars:1", "mars:2"}},
		{"host with no matches", Filter{Host: "venus"}, nil},
		{"tag match", Filter{Tag: "work"}, []string{"earth:1", "earth:2", "mars:1"}},
		{"tag is case-sensitive", Filter{Tag: "WORK"}, nil},
		{"action exact", Filter{Action: "add"}, []string{"earth:3", "mars:1", "mars:2"}},
		{"action is case-insensitive", Filter{Action: "ADD"}, []string{"earth:3", "mars:1", "mars:2"}},
		{"descr substring", Filter{Descr: "standup"}, []string{"earth:1"}},
		{"descr substring is case-insensitive", Filter{Descr: "BREAK"}, []string{"earth:3"}},
		{"descr substring no match", Filter{Descr: "nonexistent"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := mustQuery(t, entries, tt.f)
			got := addressesOf(rows)
			if !equalStrings(got, tt.want) {
				t.Fatalf("addresses = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_TimeRangeBoundaries(t *testing.T) {
	entries := queryFixture() // epochs 1000..5000

	tests := []struct {
		name         string
		since, until int64 // 0 means zero time.Time (no bound)
		want         []string
	}{
		{"since is inclusive of an exact match", 2000, 0, []string{"earth:2", "earth:3", "mars:1", "mars:2"}},
		{"until is inclusive of an exact match", 0, 2000, []string{"earth:1", "earth:2"}},
		{"since and until together bound a window", 2000, 4000, []string{"earth:2", "earth:3", "mars:1"}},
		{"since after every entry yields nothing", 6000, 0, nil},
		{"until before every entry yields nothing", 0, 500, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Filter{}
			if tt.since != 0 {
				f.Since = time.Unix(tt.since, 0)
			}
			if tt.until != 0 {
				f.Until = time.Unix(tt.until, 0)
			}
			rows := mustQuery(t, entries, f)
			got := addressesOf(rows)
			if !equalStrings(got, tt.want) {
				t.Fatalf("addresses = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_ValueRangeBoundaries(t *testing.T) {
	entries := queryFixture() // values: 0, 0, 1800, -600, 900

	tests := []struct {
		name     string
		min, max *int64
		want     []string
	}{
		{"min is inclusive of an exact match", int64Ptr(900), nil, []string{"earth:3", "mars:2"}},
		{"max is inclusive of an exact match", nil, int64Ptr(0), []string{"earth:1", "earth:2", "mars:1"}},
		{"min and max together bound a window", int64Ptr(0), int64Ptr(900), []string{"earth:1", "earth:2", "mars:2"}},
		{"negative min includes the withdrawal", int64Ptr(-600), int64Ptr(-600), []string{"mars:1"}},
		{"min above every value yields nothing", int64Ptr(10000), nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := mustQuery(t, entries, Filter{Min: tt.min, Max: tt.max})
			got := addressesOf(rows)
			if !equalStrings(got, tt.want) {
				t.Fatalf("addresses = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilter_CombinedConditions(t *testing.T) {
	entries := queryFixture()

	// Only mars:1 is host "mars" AND tagged "work" AND has a negative value.
	f := Filter{Host: "mars", Tag: "work", Max: int64Ptr(-1)}
	rows := mustQuery(t, entries, f)
	if got := addressesOf(rows); !equalStrings(got, []string{"mars:1"}) {
		t.Fatalf("addresses = %v, want [mars:1]", got)
	}

	// Combining host and descr narrows further than either alone.
	f2 := Filter{Host: "earth", Descr: "break"}
	rows2 := mustQuery(t, entries, f2)
	if got := addressesOf(rows2); !equalStrings(got, []string{"earth:3"}) {
		t.Fatalf("addresses = %v, want [earth:3]", got)
	}
}

func TestQuery_Limit(t *testing.T) {
	entries := queryFixture()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero limit is unlimited", 0, 5},
		{"negative limit is unlimited", -1, 5},
		{"positive limit caps results", 2, 2},
		{"limit larger than the result set is a no-op", 100, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := mustQuery(t, entries, Filter{Limit: tt.limit})
			if len(rows) != tt.want {
				t.Fatalf("len(rows) = %d, want %d", len(rows), tt.want)
			}
		})
	}
}

func TestQuery_EmptyEntrySetAndEmptyResult(t *testing.T) {
	rows := mustQuery(t, nil, Filter{})
	if len(rows) != 0 {
		t.Fatalf("expected no rows from an empty entry set, got %+v", rows)
	}

	rows = mustQuery(t, queryFixture(), Filter{Host: "does-not-exist"})
	if len(rows) != 0 {
		t.Fatalf("expected no rows for an unmatched host, got %+v", rows)
	}
}

func TestFilter_ValidateRejectsInvertedRanges(t *testing.T) {
	sinceAfterUntil := Filter{Since: time.Unix(2000, 0), Until: time.Unix(1000, 0)}
	if err := sinceAfterUntil.Validate(); err == nil {
		t.Fatal("expected an error when since is after until")
	}
	if _, err := Query(queryFixture(), sinceAfterUntil); err == nil {
		t.Fatal("expected Query to propagate the inverted-range error")
	}

	minAboveMax := Filter{Min: int64Ptr(10), Max: int64Ptr(5)}
	if err := minAboveMax.Validate(); err == nil {
		t.Fatal("expected an error when min is above max")
	}

	valid := Filter{Since: time.Unix(1000, 0), Until: time.Unix(2000, 0), Min: int64Ptr(5), Max: int64Ptr(10)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate on a well-formed range: %v", err)
	}
}

func TestCollectEntries_MergesAndSortsAcrossHosts(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Appended out of epoch order and across two hosts; CollectEntries must
	// still come back sorted by epoch regardless of append or host order.
	if err := store.Append(ctx, Entry{ID: 1, Action: "add", Epoch: 3000, Host: "mars", Tags: []string{"work"}, Value: 60}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(ctx, Entry{ID: 1, Action: "add", Epoch: 1000, Host: "earth", Tags: []string{"work"}, Value: 60}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(ctx, Entry{ID: 2, Action: "add", Epoch: 2000, Host: "earth", Tags: []string{"work"}, Value: 60}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	entries := CollectEntries(store)
	if len(entries) != 3 {
		t.Fatalf("CollectEntries returned %d entries, want 3", len(entries))
	}
	wantEpochs := []int64{1000, 2000, 3000}
	for i, e := range entries {
		if e.Epoch != wantEpochs[i] {
			t.Fatalf("entries[%d].Epoch = %d, want %d (full: %+v)", i, e.Epoch, wantEpochs[i], entries)
		}
	}

	rows := mustQuery(t, entries, Filter{})
	if got := addressesOf(rows); !equalStrings(got, []string{"earth:1", "earth:2", "mars:1"}) {
		t.Fatalf("addresses = %v", got)
	}
}

func TestFormatTable_RendersHeaderAndRows(t *testing.T) {
	rows := []Row{
		{Address: "earth:1", Entry: Entry{ID: 1, Action: "add", Epoch: 0, Host: "earth", Value: 60, Tags: []string{"work"}, Descr: "note"}},
	}
	out := FormatTable(rows)

	if !strings.Contains(out, "ADDRESS") || !strings.Contains(out, "DESCR") {
		t.Fatalf("table missing expected header columns:\n%s", out)
	}
	if !strings.Contains(out, "earth:1") || !strings.Contains(out, "note") {
		t.Fatalf("table missing expected row content:\n%s", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected a header line plus one row, got %d lines:\n%s", len(lines), out)
	}
}

func TestFormatTable_EmptyRowsStillRendersHeader(t *testing.T) {
	out := FormatTable(nil)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only the header line for no rows, got:\n%s", out)
	}
	if !strings.Contains(out, "ADDRESS") {
		t.Fatalf("expected header line, got:\n%s", out)
	}
}

func TestFormatJSON_RoundTripsRowFields(t *testing.T) {
	rows := []Row{
		{Address: "mars:1", Entry: Entry{ID: 1, Action: "add", Epoch: 4000, Host: "mars", Value: -600, Tags: []string{"work", "offsite"}, Descr: "Fixed CI outage"}},
	}

	out, err := FormatJSON(rows)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 element, got %d", len(decoded))
	}
	got := decoded[0]
	if got["address"] != "mars:1" {
		t.Fatalf("address = %v, want mars:1", got["address"])
	}
	if got["descr"] != "Fixed CI outage" {
		t.Fatalf("descr = %v", got["descr"])
	}
	tags, ok := got["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "work" || tags[1] != "offsite" {
		t.Fatalf("tags = %v", got["tags"])
	}
}

func TestFormatJSON_EmptyRowsRendersEmptyArray(t *testing.T) {
	out, err := FormatJSON(nil)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("FormatJSON(nil) = %q, want []", out)
	}
}

func int64Ptr(v int64) *int64 { return &v }

// equalStrings compares two string slices treating nil and an empty,
// non-nil slice as equal, since Query returns a non-nil empty slice for
// "no matches" while test tables more naturally spell that as nil.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
