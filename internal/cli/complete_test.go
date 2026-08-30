package cli

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// completionCmd returns a bare *cobra.Command carrying the same --db/--store
// flags newRuntime reads (see runtime.go), with --store pointed at storeDir,
// so a completer can be called directly (completeHosts(cmd, nil, "")) rather
// than only reachable through cobra's "__complete" plumbing on a full
// command tree.
func completionCmd(t *testing.T, storeDir string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("db", "", "")
	cmd.Flags().String("store", "", "")
	cmd.Flags().BoolP("verbose", "v", false, "")
	if err := cmd.Flags().Set("store", storeDir); err != nil {
		t.Fatalf("set --store: %v", err)
	}
	return cmd
}

// seedEntry writes one entry directly through worktime.Add, host and all,
// bypassing hostname resolution entirely -- the CLI always writes under the
// machine's own hostname, so multi-host fixtures (distinct-hosts tests)
// have to go through worktime's package API instead of `work add`.
func seedEntry(t *testing.T, storeDir, host string, at time.Time, tags []string, descr string) {
	t.Helper()
	store, err := worktime.Open(context.Background(), storeDir)
	if err != nil {
		t.Fatalf("worktime.Open: %v", err)
	}
	cfg := config.Default().Accounting
	if _, err := worktime.Add(context.Background(), store, cfg, host, tags, time.Hour, at, descr); err != nil {
		t.Fatalf("worktime.Add: %v", err)
	}
}

// completionValues extracts the plain completion strings (dropping any
// "\tdescription" suffix cobra.CompletionWithDesc appended) from a
// completer's result, for tests that only care which values were offered.
func completionValues(completions []cobra.Completion) []string {
	values := make([]string, len(completions))
	for i, c := range completions {
		values[i] = strings.SplitN(c, "\t", 2)[0]
	}
	return values
}

func TestCompleteHostsReturnsDistinctSortedHosts(t *testing.T) {
	store := newScratchStore(t)
	at := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	seedEntry(t, store, "host-b", at, nil, "b entry")
	seedEntry(t, store, "host-a", at, nil, "a entry")
	seedEntry(t, store, "host-a", at.Add(time.Hour), nil, "a entry 2")

	cmd := completionCmd(t, store)
	completions, directive := completeHosts(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	got := completionValues(completions)
	want := []string{"host-a", "host-b"}
	if !equalStrings(got, want) {
		t.Errorf("completeHosts = %v, want %v", got, want)
	}
}

func TestCompleteHostsFiltersByPrefix(t *testing.T) {
	store := newScratchStore(t)
	at := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	seedEntry(t, store, "host-a", at, nil, "a")
	seedEntry(t, store, "other", at, nil, "o")

	cmd := completionCmd(t, store)
	completions, _ := completeHosts(cmd, nil, "host")
	got := completionValues(completions)
	want := []string{"host-a"}
	if !equalStrings(got, want) {
		t.Errorf("completeHosts(prefix host) = %v, want %v", got, want)
	}
}

func TestCompleteTagsReturnsDistinctSortedTags(t *testing.T) {
	store := newScratchStore(t)
	at := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	seedEntry(t, store, "host-a", at, []string{"work", "urgent"}, "one")
	seedEntry(t, store, "host-a", at.Add(time.Hour), []string{"lunch"}, "two")
	seedEntry(t, store, "host-a", at.Add(2*time.Hour), []string{"work"}, "three")

	cmd := completionCmd(t, store)
	completions, directive := completeTags(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	got := completionValues(completions)
	want := []string{"lunch", "urgent", "work"}
	if !equalStrings(got, want) {
		t.Errorf("completeTags = %v, want %v", got, want)
	}
}

func TestCompleteHostIDAddressIncludesDateAndDescr(t *testing.T) {
	store := newScratchStore(t)
	at := time.Date(2026, 3, 14, 8, 30, 0, 0, time.UTC)
	seedEntry(t, store, "host-a", at, []string{"work"}, "wrote the design doc")

	cmd := completionCmd(t, store)
	completions, directive := completeHostIDAddress(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(completions) != 1 {
		t.Fatalf("want 1 completion, got %d: %v", len(completions), completions)
	}

	got := string(completions[0])
	if !strings.HasPrefix(got, "host-a:1\t") {
		t.Errorf("completion = %q, want address host-a:1 with a description", got)
	}
	if !strings.Contains(got, at.Local().Format("2006-01-02 15:04:05")) {
		t.Errorf("completion = %q, want it to contain the entry's local date/time", got)
	}
	if !strings.Contains(got, "wrote the design doc") {
		t.Errorf("completion = %q, want it to contain the entry's description", got)
	}
}

func TestCompleteHostIDAddressFiltersByPrefix(t *testing.T) {
	store := newScratchStore(t)
	at := time.Date(2026, 3, 14, 8, 30, 0, 0, time.UTC)
	seedEntry(t, store, "host-a", at, nil, "on host-a")
	seedEntry(t, store, "host-b", at, nil, "on host-b")

	cmd := completionCmd(t, store)
	completions, _ := completeHostIDAddress(cmd, nil, "host-b")
	if len(completions) != 1 {
		t.Fatalf("want 1 completion, got %d: %v", len(completions), completions)
	}
	if !strings.HasPrefix(string(completions[0]), "host-b:") {
		t.Errorf("completion = %q, want it to start with host-b:", completions[0])
	}
}

func TestCompleteRangesOffersFixedKeywords(t *testing.T) {
	completions, directive := completeRanges(nil, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	got := completionValues(completions)
	for _, want := range []string{"today", "yesterday", "week", "lastweek", "month"} {
		if !containsString(got, want) {
			t.Errorf("completeRanges() = %v, missing keyword %q", got, want)
		}
	}
	monthHint := time.Now().Format("2006-01")
	if !containsString(got, monthHint) {
		t.Errorf("completeRanges() = %v, missing current-month hint %q", got, monthHint)
	}
}

func TestCompleteRangesFiltersByPrefix(t *testing.T) {
	completions, _ := completeRanges(nil, nil, "l")
	got := completionValues(completions)
	want := []string{"lastweek"}
	if !equalStrings(got, want) {
		t.Errorf("completeRanges(l) = %v, want %v", got, want)
	}
}

// TestCompleteEmptyStoreDoesNotPanic confirms every store-backed completer
// degrades to "no suggestions" (rather than panicking or erroring) when the
// store has no entries at all -- the state a fresh `timesamurai work` setup
// starts in.
func TestCompleteEmptyStoreDoesNotPanic(t *testing.T) {
	store := newScratchStore(t)
	cmd := completionCmd(t, store)

	if completions, directive := completeHosts(cmd, nil, ""); len(completions) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("completeHosts on empty store = %v/%v, want no completions, NoFileComp", completions, directive)
	}
	if completions, directive := completeTags(cmd, nil, ""); len(completions) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("completeTags on empty store = %v/%v, want no completions, NoFileComp", completions, directive)
	}
	if completions, directive := completeHostIDAddress(cmd, nil, ""); len(completions) != 0 || directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("completeHostIDAddress on empty store = %v/%v, want no completions, NoFileComp", completions, directive)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// TestCompleteHostIDAddressIsRecentFirstAndCapped guards the completion UX
// against the store's size. The real store holds 12802 entries; offering all
// of them on a bare Tab fills the pager with entries from years ago, which
// reads as the completion being broken. Newest-first plus a cap keeps the
// common case (correcting something logged recently) at the top, and the
// prefix filter still reaches anything older.
func TestCompleteHostIDAddressIsRecentFirstAndCapped(t *testing.T) {
	storeDir := t.TempDir()
	cmd := completionCmd(t, storeDir)

	// More entries than the cap, ascending in time so "newest" is
	// unambiguous and is also the highest id.
	total := maxAddressCompletions + 50
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local)
	for i := 0; i < total; i++ {
		seedEntry(t, storeDir, "earth", base.Add(time.Duration(i)*time.Hour), []string{"work"}, "")
	}

	got, directive := completeHostIDAddress(cmd, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp", directive)
	}
	if len(got) != maxAddressCompletions {
		t.Fatalf("offered %d completions, want the cap of %d", len(got), maxAddressCompletions)
	}

	values := completionValues(got)
	if want := fmt.Sprintf("earth:%d", total); values[0] != want {
		t.Errorf("first completion = %q, want the newest entry %q", values[0], want)
	}

	// A prefix still reaches older entries, since filtering precedes the cap.
	got, _ = completeHostIDAddress(cmd, nil, "earth:1\t")
	if len(got) != 0 {
		t.Errorf("nonsense prefix offered %d completions, want 0", len(got))
	}
}
