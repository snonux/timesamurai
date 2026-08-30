package legacy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/snonux/timesamurai/internal/worktime"
)

// ErrExportWouldDiscard is the sentinel wrapped by ExportHost's error when
// ExportOptions.Strict is set and export would discard on-disk entries that
// have no counterpart in the fresh export (see ExportHost's package doc
// comment for the fail-closed rationale). Callers can match it with
// errors.Is to distinguish a refused export from any other failure (I/O,
// invalid host, etc.).
var ErrExportWouldDiscard = errors.New("export refused: writing would discard on-disk entries absent from the fresh export")

// ExportOptions configures ExportHost/ExportAll's legacy-export behavior.
type ExportOptions struct {
	// Strict switches ExportHost from warn-and-overwrite to fail-closed:
	// when export would discard on-disk entries with no counterpart in the
	// fresh export -- i.e. worktime.rb or a hand edit changed
	// db.<host>.json since the last export -- ExportHost refuses to write
	// and returns an error wrapping ErrExportWouldDiscard instead of
	// overwriting them. The on-disk file is left untouched.
	//
	// Strict exists for operators who want a hard stop during the
	// dual-tool coexistence window instead of relying on catching the
	// stderr warning. It is opt-in: the zero value (false) preserves the
	// original warn-and-overwrite behavior so existing workflows and
	// automation are unaffected.
	Strict bool
	// WarnOut receives the discard warning in non-strict mode; a nil
	// WarnOut defaults to os.Stderr so the warning is never silently
	// swallowed by an uninterested caller. Unused in strict mode, since a
	// refusal is reported through the returned error instead.
	WarnOut io.Writer
}

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
// WHY warn-and-proceed by default, never re-import: this is a deliberate
// one-way projection (migrate.go is the one-way trip in the other
// direction; together they are never a two-way sync). The JSONL store is
// the single source of truth from here on. If worktime.rb or a human edits
// db.<host>.json after an export — the only way it can gain data the store
// doesn't have — that edit is about to be overwritten, since ExportHost
// always regenerates the file fresh from the store. That loss is real, so
// this function names exactly what is disappearing before it writes. By
// default it never refuses to export (a stale or hand-edited legacy file
// must never block the JSONL side of the tool from doing its job by
// default) and never folds the discarded entries back into the store
// (merging report-only edits into the store would make the store itself
// only report-only reliable, defeating the point of the rewrite). Operators
// who need those edits keep them by re-applying them through the JSONL-side
// commands before the next export.
//
// opts.Strict flips that default: when set, a would-be discard makes
// ExportHost refuse to write at all (see ExportOptions.Strict) instead of
// warning and overwriting, for operators who'd rather stop and investigate
// than risk losing a Ruby-side edit during the coexistence window. Discard
// detection and the JSONL-source-of-truth contract are unchanged either
// way; Strict only changes what happens once a discard is detected.
func ExportHost(ctx context.Context, store *worktime.Store, dbDir, host string, opts ExportOptions) (ExportResult, error) {
	if err := ctx.Err(); err != nil {
		return ExportResult{}, err
	}
	host, err := worktime.NormalizeHost(host)
	if err != nil {
		return ExportResult{}, err
	}
	warnOut := opts.WarnOut
	if warnOut == nil {
		warnOut = os.Stderr
	}

	onDisk, err := LoadLegacyHost(ctx, dbDir, host)
	if err != nil {
		return ExportResult{}, fmt.Errorf("read existing legacy db for host %q: %w", host, err)
	}

	fresh := buildFreshLegacyEntries(host, store.Entries(host))
	carryOverHuman(onDisk.Entries[host], fresh)
	discarded := discardedLegacyEntries(onDisk.Entries[host], fresh)
	if len(discarded) > 0 {
		if opts.Strict {
			// Fail closed: return before SaveLegacyHost so the on-disk
			// file is left exactly as the operator (or worktime.rb) left
			// it, and the caller gets both the error and the entries that
			// triggered it for reporting.
			return ExportResult{Host: host, Discarded: discarded}, strictDiscardError(host, discarded)
		}
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

// strictDiscardError builds the error ExportHost returns in strict mode
// when export would discard on-disk entries; it names the host, the count,
// and the file, and wraps ErrExportWouldDiscard so callers can match it
// with errors.Is regardless of the exact wording.
func strictDiscardError(host string, discarded []LegacyEntry) error {
	return fmt.Errorf("%w: host %q has %d entr(y/ies) in %s absent from the fresh export "+
		"(worktime.rb or a hand edit since the last export?); re-apply the change through a "+
		"timesamurai work command before exporting again, or drop --strict to overwrite as before",
		ErrExportWouldDiscard, host, len(discarded), legacyDBFileName(host))
}

// ExportAll exports every host currently known to store, in sorted host
// order, into dbDir. It stops at the first hard error (I/O or encode
// failure). In the default (non-strict) mode discard warnings never stop
// it, since they are advisory only — see ExportHost's warn-and-proceed
// contract. In opts.Strict mode a would-be discard on any host IS a hard
// error (ErrExportWouldDiscard), so ExportAll stops there too, leaving that
// host's file untouched and every host after it unexported for this run.
func ExportAll(ctx context.Context, store *worktime.Store, dbDir string, opts ExportOptions) ([]ExportResult, error) {
	hosts := store.Hosts()
	results := make([]ExportResult, 0, len(hosts))

	for _, host := range hosts {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		result, err := ExportHost(ctx, store, dbDir, host, opts)
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
func buildFreshLegacyEntries(host string, entries []worktime.Entry) []LegacyEntry {
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
// reasons for the rewrite. Export collapses back down to the first tag,
// mirroring migrate.go's reverse conversion (legacyToEntry puts What into
// Tags[0]), so a migrate-then-
// export round trip is a no-op for the untouched, single-tag history that
// makes up the real dataset.
func entryToLegacy(host string, entry worktime.Entry) LegacyEntry {
	leg := LegacyEntry{
		Action: entry.Action,
		Epoch:  entry.Epoch,
		Source: host,
		Descr:  entry.Descr,
	}
	if len(entry.Tags) > 0 {
		leg.What = entry.Tags[0]
	}
	if strings.EqualFold(strings.TrimSpace(entry.Action), worktime.ActionAdd) {
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

// carryOverHuman copies each on-disk entry's "human" string onto the
// content-identical fresh entry before writing.
//
// "human" is a display field: worktime.rb sets it once from the local clock
// at insert time and never touches it again, so the historical files record
// the timezone each entry was logged in -- 2020 entries from the London
// hosts read two hours earlier than the same epoch formatted in Europe/Sofia
// today. Regenerating it for every entry would restate that history and
// rewrite ~29% of the lines in every db.<host>.json on the first export,
// burying real changes in derived-field churn.
//
// Matching uses the same content key and multiset accounting as
// discardedLegacyEntries, so an entry whose epoch changed via "work modify"
// finds no match and correctly gets a freshly derived timestamp. Entries
// with no counterpart on disk (new ones) are left blank for
// prepareLegacyEntry to fill in.
func carryOverHuman(onDisk, fresh []LegacyEntry) {
	if len(onDisk) == 0 {
		return
	}

	available := make(map[string][]string, len(onDisk))
	for _, e := range onDisk {
		if human := strings.TrimSpace(e.Human); human != "" {
			key := legacyEntryKey(e)
			available[key] = append(available[key], e.Human)
		}
	}

	for i := range fresh {
		key := legacyEntryKey(fresh[i])
		queued := available[key]
		if len(queued) == 0 {
			continue
		}
		fresh[i].Human = queued[0]
		available[key] = queued[1:]
	}
}
