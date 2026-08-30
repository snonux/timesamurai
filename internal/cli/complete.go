package cli

// Dynamic shell completers (task 571) for the flags/positionals that
// enumerate against the store rather than a fixed set: --host/--tag on
// list/search, the <host:id> address positionals on modify/delete, and the
// [range] positional shared by list/report/edit. Each completer opens its
// own runtime (newRuntime) rather than caching one across invocations --
// shell completion is a short-lived, one-shot process per keystroke, so
// there is no long-lived state to share, and re-opening keeps every
// completer trivially independent of call order.
//
// `mage completions` regenerates completions/timesamurai.fish from these
// (via `timesamurai completion fish`), so no shell script here needs manual
// upkeep when a completer's behavior changes.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/worktime"
)

// maxAddressCompletions caps how many <host>:<id> candidates a single Tab
// offers. Large enough that a normal correction ("something I logged this
// week") is always in the list, small enough that the pager stays readable.
const maxAddressCompletions = 200

// completionFunc is the shape cobra.Command.ValidArgsFunction and
// RegisterFlagCompletionFunc both expect, spelled out once so the wiring
// helper and completer signatures below don't repeat cobra's generic
// func(...) ([]cobra.Completion, cobra.ShellCompDirective) type.
type completionFunc = func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective)

// rangeKeywords are the bare keywords internal/timefmt.ParseRange accepts.
// YYYY-MM months and "date..date" spans are open-ended and can't be
// enumerated, so completeRanges below offers only these plus a literal hint
// for the current month.
var rangeKeywords = []string{"today", "yesterday", "week", "lastweek", "month"}

// registerFlagCompletion wires fn as name's dynamic completer, panicking if
// name isn't a registered flag on cmd. That can only happen from a typo'd
// flag name at construction time -- a coding bug, not a runtime condition --
// so failing loudly immediately (rather than silently leaving the flag
// without completion, or forcing every call site to check an error that can
// never legitimately occur) is the more honest failure mode.
func registerFlagCompletion(cmd *cobra.Command, name string, fn completionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(name, fn); err != nil {
		panic(fmt.Sprintf("register completion for --%s: %v", name, err))
	}
}

// completeRanges completes the [range] positional list/report/edit share,
// offering timefmt's fixed keywords plus the current YYYY-MM as a
// discoverable example of the open-ended month form, both filtered to
// toComplete as a prefix. It never touches the store, so unlike the other
// completers here it cannot fail.
func completeRanges(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	completions := make([]cobra.Completion, 0, len(rangeKeywords)+1)
	for _, kw := range rangeKeywords {
		if strings.HasPrefix(kw, toComplete) {
			completions = append(completions, kw)
		}
	}
	if hint := currentMonthHint(toComplete); hint != "" {
		completions = append(completions, cobra.CompletionWithDesc(hint, "this month (YYYY-MM)"))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// currentMonthHint returns this month's "YYYY-MM" form when it starts with
// toComplete, or "" otherwise -- a small worked example of the range form
// ParseRange's year-month branch accepts, since that form can't otherwise
// be enumerated.
func currentMonthHint(toComplete string) string {
	hint := time.Now().Format("2006-01")
	if strings.HasPrefix(hint, toComplete) {
		return hint
	}
	return ""
}

// completeHosts completes --host (list/search) with the distinct hosts seen
// across every entry in the store.
func completeHosts(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	entries, err := completionEntries(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return filterByPrefix(distinctHosts(entries), toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeTags completes --tag (list/search) with the distinct tags seen
// across every entry in the store.
func completeTags(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	entries, err := completionEntries(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return filterByPrefix(distinctTags(entries), toComplete), cobra.ShellCompDirectiveNoFileComp
}

// completeHostIDAddress completes the <host:id> positionals modify/delete
// take, offering every entry's address with a completion description
// showing its date and description -- so a caller picking an address from
// shell tab-completion sees enough to tell entries apart without first
// running `work list`.
func completeHostIDAddress(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	rt, err := newRuntime(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	rows, err := worktime.Query(worktime.CollectEntries(rt.store), worktime.Filter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	matched := make([]worktime.Row, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row.Address, toComplete) {
			matched = append(matched, row)
		}
	}

	// Newest first, then capped. The store holds the whole history -- 12802
	// entries here -- and offering all of them on a bare Tab buries the
	// pager in entries from years ago, which reads as the completion being
	// broken rather than merely long. The entry someone wants to correct is
	// almost always a recent one, and typing more of the address still
	// narrows to anything older because the prefix filter above runs first.
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Entry.Epoch > matched[j].Entry.Epoch
	})
	if len(matched) > maxAddressCompletions {
		matched = matched[:maxAddressCompletions]
	}

	completions := make([]cobra.Completion, 0, len(matched))
	for _, row := range matched {
		completions = append(completions, addressCompletion(row))
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// addressCompletion renders one row as a cobra completion whose description
// is "<date> <descr>" -- the two fields most likely to tell entries apart at
// a glance, in the same date format `work edit`'s text block already uses.
func addressCompletion(row worktime.Row) cobra.Completion {
	when := time.Unix(row.Entry.Epoch, 0).Format("2006-01-02 15:04:05")
	desc := when
	if row.Entry.Descr != "" {
		desc = when + "  " + row.Entry.Descr
	}
	return cobra.CompletionWithDesc(row.Address, desc)
}

// completionEntries opens a runtime for cmd and returns every entry across
// every host -- the shared first step of completeHosts/completeTags, split
// out so neither repeats newRuntime/CollectEntries wiring.
func completionEntries(cmd *cobra.Command) ([]worktime.Entry, error) {
	rt, err := newRuntime(cmd)
	if err != nil {
		return nil, err
	}
	return worktime.CollectEntries(rt.store), nil
}

// distinctHosts returns the sorted, deduplicated set of entries' Host
// fields.
func distinctHosts(entries []worktime.Entry) []string {
	seen := make(map[string]bool)
	for _, e := range entries {
		seen[e.Host] = true
	}
	return sortedKeys(seen)
}

// distinctTags returns the sorted, deduplicated set of tags across every
// entry's Tags.
func distinctTags(entries []worktime.Entry) []string {
	seen := make(map[string]bool)
	for _, e := range entries {
		for _, t := range e.Tags {
			seen[t] = true
		}
	}
	return sortedKeys(seen)
}

// sortedKeys returns seen's keys in sorted order, so completion output (and
// test assertions against it) is deterministic rather than following Go's
// randomized map iteration order.
func sortedKeys(seen map[string]bool) []string {
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// filterByPrefix returns the values in values that start with toComplete,
// preserving their (already sorted) order.
func filterByPrefix(values []string, toComplete string) []cobra.Completion {
	completions := make([]cobra.Completion, 0, len(values))
	for _, v := range values {
		if strings.HasPrefix(v, toComplete) {
			completions = append(completions, v)
		}
	}
	return completions
}
