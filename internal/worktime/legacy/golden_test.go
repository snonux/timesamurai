package legacy

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// This file is the capstone parity check for the whole worktime rewrite
// (task 271): it exercises migrate.go (p61), export.go (q61) and report.go
// (u61/v61) together against Paul's real ~/git/worktime/db.*.json history —
// nine years, 12,802 entries, the only copy of this data — rather than the
// small synthetic fixtures every other *_test.go file in this package uses.
//
// Two things follow from that:
//
//   - Both tests below are strictly read-only against ~/git/worktime: they
//     copy the real db.*.json files into a t.TempDir() before doing anything
//     that writes (Migrate, ExportAll), and md5-check the real files
//     unchanged in a t.Cleanup so any accidental write anywhere in this
//     package would fail the test loudly instead of corrupting history.
//   - Since that directory won't exist on every machine (CI, another
//     developer's checkout), both tests skip outright — via realWorktimeDBDir
//     — rather than failing, so this stays a strong *local* regression guard
//     without breaking portability. Skipping is confirmed to actually be a
//     skip, not a silent no-op: running `go test -v -run TestGolden` here
//     (where the data exists) shows PASS, not SKIP.
func realWorktimeDBDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("skip: cannot resolve home directory: %v", err)
	}
	dir := filepath.Join(home, "git", "worktime")
	matches, err := filepath.Glob(filepath.Join(dir, legacyDBFilePattern))
	if err != nil || len(matches) == 0 {
		t.Skipf("skip: no real worktime data at %s (local-only regression guard, task 271)", dir)
	}
	return dir
}

// md5SumsOf fingerprints every db.*.json file in dir, keyed by path, so a
// test can prove later that none of them changed.
func md5SumsOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, legacyDBFilePattern))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	sums := make(map[string]string, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("checksum read %s: %v", path, err)
		}
		sum := md5.Sum(data)
		sums[path] = hex.EncodeToString(sum[:])
	}
	return sums
}

// assertRealDataUnchanged fails loudly if dir's db.*.json files no longer
// match before, either in membership or content. This is the hard safety
// rail against this suite ever mutating the user's only copy of nine years
// of history — see this file's doc comment.
func assertRealDataUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := md5SumsOf(t, dir)
	if len(after) != len(before) {
		t.Fatalf("real worktime file count changed: before=%d after=%d -- this must never happen", len(before), len(after))
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Fatalf("real worktime file %s changed (md5 %s -> %s) -- this must never happen", path, sum, after[path])
		}
	}
}

// copyLegacyDBFiles copies every db.*.json from src into dst, so tests can
// run operations that write (Migrate opens and writes a store; ExportAll
// writes fresh legacy files) against a disposable scratch copy instead of
// ever touching src.
func copyLegacyDBFiles(t *testing.T, src, dst string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(src, legacyDBFilePattern))
	if err != nil {
		t.Fatalf("glob %s: %v", src, err)
	}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		dst := filepath.Join(dst, filepath.Base(path))
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write scratch copy %s: %v", dst, err)
		}
	}
}

// migrateScratchStore copies dbDir's db.*.json files into a fresh scratch
// directory and migrates them into a fresh scratch JSONL store, returning
// the opened Store. Both directories are t.TempDir()s owned solely by the
// caller; dbDir itself is only ever read from (via copyLegacyDBFiles).
// Shared by TestGolden_ReportMatchesRubyByteForByte and migrateThenExport
// so the same "copy, then migrate" step isn't written out twice.
func migrateScratchStore(t *testing.T, ctx context.Context, dbDir string) *worktime.Store {
	t.Helper()
	scratchDB := t.TempDir()
	copyLegacyDBFiles(t, dbDir, scratchDB)

	storeDir := t.TempDir()
	if _, err := Migrate(ctx, scratchDB, storeDir, MigrateOptions{Report: io.Discard}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open scratch store: %v", err)
	}
	return store
}

// TestGolden_ReportMatchesRubyByteForByte migrates a scratch copy of the
// real db.*.json files into a JSONL store, renders the report through this
// package's own BuildReport/FormatReport, and diffs it byte-for-byte
// against testdata/report.golden.
//
// report.golden is a `ruby worktime.rb --report` run captured once
// (2026-08-30, ruby 4.0.6, against this exact dataset) and committed here to
// pin that already-verified-by-hand result (tasks u61/v61) down as a
// permanent, automated regression guard, rather than trusting it to stay
// true forever on faith. If this test ever fails, the fix is almost never
// to touch this test or regenerate the golden file: it means report.go's
// replay has drifted from worktime.rb's, which report.go's own doc comment
// says must not happen — recapture the golden file only after confirming
// (by hand, against a fresh `ruby worktime.rb --report` run) that the
// *Ruby* output itself legitimately changed, e.g. because history grew.
func TestGolden_ReportMatchesRubyByteForByte(t *testing.T) {
	dbDir := realWorktimeDBDir(t)
	before := md5SumsOf(t, dbDir)
	t.Cleanup(func() { assertRealDataUnchanged(t, dbDir, before) })

	ctx := context.Background()
	store := migrateScratchStore(t, ctx, dbDir)

	weeks, err := worktime.BuildReport(worktime.CollectEntries(store), config.Default().Accounting, io.Discard)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	got := worktime.FormatReport(weeks, false)

	want, err := os.ReadFile(filepath.Join("testdata", "report.golden"))
	if err != nil {
		t.Fatalf("read testdata/report.golden: %v", err)
	}

	if got != string(want) {
		t.Fatalf("report diverged from testdata/report.golden (got %d bytes, want %d bytes) -- "+
			"see this test's doc comment before regenerating the golden file",
			len(got), len(want))
	}
}

// comparableLegacyEntry is the semantic identity of one legacy entry for
// round-trip comparison. Source and Human are deliberately excluded: both
// are always derived (Source from the section key, Human formatted from
// Epoch by SaveLegacyHost/prepareLegacyEntry) rather than independent data,
// so two entries differing only in those fields describe the same event —
// the same reasoning export.go's legacyEntryKey already applies for discard
// detection. Host here IS included, but it is LegacyEntry.Source *after*
// LoadLegacyAll's backfill (so it reflects the section key an entry
// actually lives under), not raw on-disk Source.
type comparableLegacyEntry struct {
	Host     string
	Action   string
	What     string
	Epoch    int64
	HasValue bool
	Value    int64
	Descr    string
}

func toComparable(e LegacyEntry) comparableLegacyEntry {
	return comparableLegacyEntry{
		Host:     e.Source,
		Action:   e.Action,
		What:     e.What,
		Epoch:    e.Epoch,
		HasValue: e.HasValue(),
		Value:    e.Value,
		Descr:    e.Descr,
	}
}

// compareInt64 orders a and b without risking the overflow a bare
// subtraction into int could hit on extreme values.
func compareInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// normalizeLegacyEntries converts entries into comparableLegacyEntry and
// sorts them into a fixed total order, so two semantically-equal slices
// compare equal via reflect-free element-by-element comparison regardless
// of on-disk or in-memory ordering — this is what makes the round-trip
// check "modulo key/field order" rather than a byte-for-byte JSON diff.
func normalizeLegacyEntries(entries []LegacyEntry) []comparableLegacyEntry {
	out := make([]comparableLegacyEntry, len(entries))
	for i, e := range entries {
		out[i] = toComparable(e)
	}
	slices.SortFunc(out, compareComparableLegacyEntry)
	return out
}

// compareComparableLegacyEntry is a deterministic total order over every
// field of comparableLegacyEntry, used purely to give normalizeLegacyEntries
// a stable sort key — the ordering itself carries no semantic meaning.
func compareComparableLegacyEntry(a, b comparableLegacyEntry) int {
	if c := strings.Compare(a.Host, b.Host); c != 0 {
		return c
	}
	if c := compareInt64(a.Epoch, b.Epoch); c != 0 {
		return c
	}
	if c := strings.Compare(a.Action, b.Action); c != 0 {
		return c
	}
	if c := strings.Compare(a.What, b.What); c != 0 {
		return c
	}
	if c := compareInt64(a.Value, b.Value); c != 0 {
		return c
	}
	return strings.Compare(a.Descr, b.Descr)
}

// assertRoundTripLossless compares original and roundTripped as multisets
// of comparableLegacyEntry (see its doc comment for what "identical" means
// here): same host, action, what, epoch, value and descr for every entry,
// with within-host ordering and on-disk field order both irrelevant.
func assertRoundTripLossless(t *testing.T, original, roundTripped []LegacyEntry) {
	t.Helper()
	want := normalizeLegacyEntries(original)
	got := normalizeLegacyEntries(roundTripped)
	if len(got) != len(want) {
		t.Fatalf("normalized entry count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entry %d diverged after round trip:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

// assertNoDiscardsAndFullCoverage checks ExportAll's own bookkeeping agrees
// with a lossless round trip: no host discarded anything (see ExportHost's
// warn-and-proceed contract in export.go — a discard here would mean the
// scratch export directory already had conflicting data before this test
// wrote to it, which should be impossible for a fresh t.TempDir()), and the
// total written across all hosts equals the original entry count.
func assertNoDiscardsAndFullCoverage(t *testing.T, results []ExportResult, wantTotal int) {
	t.Helper()
	total := 0
	for _, r := range results {
		if len(r.Discarded) != 0 {
			t.Fatalf("host %q discarded %d entries on a lossless round trip: %+v", r.Host, len(r.Discarded), r.Discarded)
		}
		total += r.Written
	}
	if total != wantTotal {
		t.Fatalf("export wrote %d entries total, want %d", total, wantTotal)
	}
}

// loadOriginalLegacyEntries reads every event directly from the real
// worktime data directory — read-only, since LoadLegacyAll only ever calls
// os.ReadFile — and sanity-bounds the count. The bound is deliberately a
// round number well below the real total (not a hardcoded exact count,
// which lives nowhere in this file): it exists purely to catch this test
// silently comparing against an empty or truncated read, without breaking
// the moment new entries get logged to the real history.
func loadOriginalLegacyEntries(t *testing.T, ctx context.Context, dbDir string) []LegacyEntry {
	t.Helper()
	original, err := LoadLegacyAll(ctx, dbDir)
	if err != nil {
		t.Fatalf("LoadLegacyAll(real): %v", err)
	}
	if len(original) < 10000 {
		t.Fatalf("original entry count = %d, suspiciously low for the real dataset", len(original))
	}
	return original
}

// migrateThenExport runs the two directions under test — migrateScratchStore
// (legacy JSON -> JSONL), then ExportAll (JSONL -> legacy JSON) into a
// second scratch directory — and returns the round-tripped entries read
// back from that second directory.
func migrateThenExport(t *testing.T, ctx context.Context, dbDir string, wantTotal int) []LegacyEntry {
	t.Helper()
	store := migrateScratchStore(t, ctx, dbDir)

	exportDir := t.TempDir()
	var warn bytes.Buffer
	results, err := ExportAll(ctx, store, exportDir, ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if warn.Len() != 0 {
		t.Fatalf("ExportAll warned about discards on a fresh migrate-then-export round trip (want none): %s", warn.String())
	}
	assertNoDiscardsAndFullCoverage(t, results, wantTotal)

	roundTripped, err := LoadLegacyAll(ctx, exportDir)
	if err != nil {
		t.Fatalf("LoadLegacyAll(round-tripped): %v", err)
	}
	return roundTripped
}

// TestRoundTrip_LegacyToJSONLToLegacyPreservesAllEvents proves the two
// directions this package provides — Migrate (legacy JSON -> JSONL) and
// ExportAll (JSONL -> legacy JSON) — compose into a lossless round trip
// across every one of the real dataset's 12,802 events. See
// assertRoundTripLossless's doc comment for the precise definition of
// "identical modulo key order" this test enforces.
func TestRoundTrip_LegacyToJSONLToLegacyPreservesAllEvents(t *testing.T) {
	dbDir := realWorktimeDBDir(t)
	before := md5SumsOf(t, dbDir)
	t.Cleanup(func() { assertRealDataUnchanged(t, dbDir, before) })

	ctx := context.Background()
	original := loadOriginalLegacyEntries(t, ctx, dbDir)
	roundTripped := migrateThenExport(t, ctx, dbDir, len(original))

	if len(roundTripped) != len(original) {
		t.Fatalf("round-tripped entry count = %d, want %d", len(roundTripped), len(original))
	}
	assertRoundTripLossless(t, original, roundTripped)
}
