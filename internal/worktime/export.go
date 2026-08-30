package worktime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ExportResult summarizes one host's legacy export: how many entries were
// written, and which on-disk entries (if any) were overwritten away because
// they had no counterpart in the fresh export.
type ExportResult struct {
	Host      string
	Written   int
	Discarded []LegacyEntry
}

// ExportHost rewrites db.<host>.json under dbDir from store's current
// entries for host, so worktime.rb keeps a report-only-usable view of data
// that now lives in the JSONL store.
//
// WHY warn-and-proceed, never refuse, never re-import: this is a deliberate
// one-way projection (migrate.go is the one-way trip in the other
// direction; together they are never a two-way sync). The JSONL store is
// the single source of truth from here on. If worktime.rb or a human edits
// db.<host>.json after an export — the only way it can gain data the store
// doesn't have — that edit is about to be overwritten, since ExportHost
// always regenerates the file fresh from the store. That loss is real, so
// this function names exactly what is disappearing before it writes. But it
// deliberately never refuses to export (a stale or hand-edited legacy file
// must never block the JSONL side of the tool from doing its job) and never
// folds the discarded entries back into the store (merging report-only
// edits into the store would make the store itself only report-only
// reliable, defeating the point of the rewrite). Operators who need those
// edits keep them by re-applying them through the JSONL-side commands
// before the next export.
//
// warnOut receives the discard warning, if any; a nil warnOut defaults to
// os.Stderr so the warning is never silently swallowed by an uninterested
// caller.
func ExportHost(ctx context.Context, store *Store, dbDir, host string, warnOut io.Writer) (ExportResult, error) {
	if err := ctx.Err(); err != nil {
		return ExportResult{}, err
	}
	host, err := normalizeHost(host)
	if err != nil {
		return ExportResult{}, err
	}
	if warnOut == nil {
		warnOut = os.Stderr
	}

	onDisk, err := LoadLegacyHost(ctx, dbDir, host)
	if err != nil {
		return ExportResult{}, fmt.Errorf("read existing legacy db for host %q: %w", host, err)
	}

	fresh := buildFreshLegacyEntries(host, store.Entries(host))
	discarded := discardedLegacyEntries(onDisk.Entries[host], fresh)
	if len(discarded) > 0 {
		if err := warnDiscarded(warnOut, host, discarded); err != nil {
			return ExportResult{}, fmt.Errorf("write discard warning for host %q: %w", host, err)
		}
	}

	db := LegacyDB{Entries: map[string][]LegacyEntry{host: fresh}}
	if err := SaveLegacyHost(ctx, dbDir, host, db); err != nil {
		return ExportResult{}, fmt.Errorf("export legacy db for host %q: %w", host, err)
	}

	return ExportResult{Host: host, Written: len(fresh), Discarded: discarded}, nil
}

// ExportAll exports every host currently known to store, in sorted host
// order, into dbDir. It stops at the first hard error (I/O or encode
// failure); discard warnings never stop it, since they are advisory only —
// see ExportHost's warn-and-proceed contract.
func ExportAll(ctx context.Context, store *Store, dbDir string, warnOut io.Writer) ([]ExportResult, error) {
	hosts := store.Hosts()
	results := make([]ExportResult, 0, len(hosts))

	for _, host := range hosts {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result, err := ExportHost(ctx, store, dbDir, host, warnOut)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// buildFreshLegacyEntries converts store entries for host into the legacy
// shape, in the store's own order; SaveLegacyHost sorts and fills in
// Source/Human before writing, so no ordering or derived-field work is
// needed here.
func buildFreshLegacyEntries(host string, entries []Entry) []LegacyEntry {
	fresh := make([]LegacyEntry, len(entries))
	for i, e := range entries {
		fresh[i] = entryToLegacy(host, e)
	}
	return fresh
}

// entryToLegacy converts one JSONL Entry back into worktime.rb's shape.
//
// worktime.rb's schema carries exactly one "what" tag per entry; JSONL
// entries may carry several — fixing that one-tag limit was one of the
// reasons for the rewrite (see docs/worktime-rewrite-plan.md). Export
// collapses back down to the first tag, mirroring migrate.go's reverse
// conversion (legacyToEntry puts What into Tags[0]), so a migrate-then-
// export round trip is a no-op for the untouched, single-tag history that
// makes up the real dataset.
func entryToLegacy(host string, entry Entry) LegacyEntry {
	leg := LegacyEntry{
		Action: entry.Action,
		Epoch:  entry.Epoch,
		Source: host,
		Descr:  entry.Descr,
	}
	if len(entry.Tags) > 0 {
		leg.What = entry.Tags[0]
	}
	if strings.EqualFold(strings.TrimSpace(entry.Action), actionAdd) {
		leg.SetValue(entry.Value)
	}
	return leg
}

// discardedLegacyEntries returns the entries in onDisk that have no
// content-identical counterpart in fresh, using multiset matching so a
// legitimate duplicate on disk doesn't falsely consume two distinct fresh
// entries (or vice versa).
func discardedLegacyEntries(onDisk, fresh []LegacyEntry) []LegacyEntry {
	remaining := make(map[string]int, len(fresh))
	for _, e := range fresh {
		remaining[legacyEntryKey(e)]++
	}

	var discarded []LegacyEntry
	for _, e := range onDisk {
		key := legacyEntryKey(e)
		if remaining[key] > 0 {
			remaining[key]--
			continue
		}
		discarded = append(discarded, e)
	}
	return discarded
}

// legacyEntryKey builds a content-identity key for discard detection,
// deliberately excluding Source and Human: both are always derived (Source
// from the host section key, Human formatted from Epoch) rather than
// independent data, so two entries differing only in those fields describe
// the same event.
func legacyEntryKey(e LegacyEntry) string {
	value := ""
	if e.HasValue() {
		value = strconv.FormatInt(e.Value, 10)
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(e.Action)),
		e.What,
		strconv.FormatInt(e.Epoch, 10),
		value,
		e.Descr,
	}, "\x1f")
}

// warnDiscarded prints a loud, hard-to-miss notice naming every on-disk
// entry about to disappear from db.<host>.json. It is advisory output only
// (see ExportHost's warn-and-proceed contract): a write failure here is
// reported to the caller, but never changes what gets exported.
func warnDiscarded(w io.Writer, host string, discarded []LegacyEntry) error {
	banner := strings.Repeat("!", 72)
	header := fmt.Sprintf(
		"%s\nWARNING: export for host %q is discarding %d entr(y/ies)\n"+
			"present in %s but absent from the fresh export\n"+
			"(worktime.rb or a hand edit since the last export?). Exporting anyway;\n"+
			"these entries are NOT being re-imported into the store.\n",
		banner, host, len(discarded), legacyDBFileName(host),
	)
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	for _, e := range discarded {
		if _, err := fmt.Fprintf(w, "  - %s\n", describeLegacyEntry(e)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, banner)
	return err
}

// describeLegacyEntry renders one legacy entry for the discard warning.
func describeLegacyEntry(e LegacyEntry) string {
	desc := fmt.Sprintf("action=%s what=%s epoch=%d (%s)", e.Action, e.What, e.Epoch, FormatLegacyHuman(e.Epoch))
	if e.HasValue() {
		desc += fmt.Sprintf(" value=%d", e.Value)
	}
	if e.Descr != "" {
		desc += fmt.Sprintf(" descr=%q", e.Descr)
	}
	return desc
}
