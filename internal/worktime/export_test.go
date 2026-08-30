package worktime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportHost_CreatesFile(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries := []Entry{
		{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}},
		{ID: 2, Action: "logout", Epoch: 200, Host: "earth", Tags: []string{"work"}},
		{ID: 3, Action: "add", Epoch: 300, Host: "earth", Value: 3600, Tags: []string{"work"}, Descr: "notes"},
	}
	for _, e := range entries {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("Append %+v: %v", e, err)
		}
	}

	var warn bytes.Buffer
	result, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("ExportHost: %v", err)
	}
	if result.Written != 3 || len(result.Discarded) != 0 {
		t.Fatalf("result = %+v, want Written=3 Discarded=empty", result)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning on first export:\n%s", warn.String())
	}

	path := filepath.Join(dbDir, "db.earth.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}

	db, err := LoadLegacyHost(ctx, dbDir, "earth")
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	got := db.Entries["earth"]
	if len(got) != 3 {
		t.Fatalf("legacy entries = %d, want 3: %+v", len(got), got)
	}
	if got[0].Action != "login" || got[0].What != "work" || got[0].Source != "earth" {
		t.Fatalf("entry[0] = %+v", got[0])
	}
	if got[2].Action != "add" || !got[2].HasValue() || got[2].Value != 3600 || got[2].Descr != "notes" {
		t.Fatalf("entry[2] = %+v", got[2])
	}
	// Human is derived from epoch by SaveLegacyHost, not left blank.
	if got[0].Human == "" {
		t.Fatalf("entry[0].Human not populated: %+v", got[0])
	}
}

func TestExportHost_SecondExportAfterMutationUpdates(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, first); err != nil {
		t.Fatalf("Append: %v", err)
	}

	var warn bytes.Buffer
	if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &warn}); err != nil {
		t.Fatalf("first ExportHost: %v", err)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning on first export:\n%s", warn.String())
	}

	second := Entry{ID: 2, Action: "logout", Epoch: 200, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, second); err != nil {
		t.Fatalf("Append: %v", err)
	}

	warn.Reset()
	result, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("second ExportHost: %v", err)
	}
	if result.Written != 2 || len(result.Discarded) != 0 {
		t.Fatalf("result = %+v, want Written=2 Discarded=empty", result)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning on second, undisturbed export:\n%s", warn.String())
	}

	db, err := LoadLegacyHost(ctx, dbDir, "earth")
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	if len(db.Entries["earth"]) != 2 {
		t.Fatalf("legacy entries = %d, want 2", len(db.Entries["earth"]))
	}
}

func TestExportHost_DiscardDetectionNamesDivergedEntries(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	kept := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, kept); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &bytes.Buffer{}}); err != nil {
		t.Fatalf("first ExportHost: %v", err)
	}

	// Simulate worktime.rb (or a human) hand-editing db.earth.json between
	// exports: append an entry the JSONL store has never heard of.
	strayRaw := `{
  "entries": {
    "earth": [
      {"action":"login","what":"work","epoch":100,"source":"earth","human":"h"},
      {"action":"add","what":"offsite","epoch":150,"source":"earth","human":"h","value":900,"descr":"hand-added"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.earth.json"), []byte(strayRaw), 0o644); err != nil {
		t.Fatal(err)
	}

	var warn bytes.Buffer
	result, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("ExportHost must never error on discard: %v", err)
	}
	if len(result.Discarded) != 1 {
		t.Fatalf("Discarded = %+v, want 1 entry", result.Discarded)
	}
	d := result.Discarded[0]
	if d.Action != "add" || d.What != "offsite" || d.Epoch != 150 || d.Value != 900 || d.Descr != "hand-added" {
		t.Fatalf("discarded entry = %+v", d)
	}

	warnText := warn.String()
	for _, want := range []string{"WARNING", "earth", "offsite", "150", "900", "hand-added"} {
		if !strings.Contains(warnText, want) {
			t.Fatalf("warning missing %q:\n%s", want, warnText)
		}
	}

	// Export proceeds: the stray entry is gone from the freshly written file,
	// and it must never have been folded back into the store.
	db, err := LoadLegacyHost(ctx, dbDir, "earth")
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	if len(db.Entries["earth"]) != 1 {
		t.Fatalf("post-export legacy entries = %+v, want only the login", db.Entries["earth"])
	}
	if got := store.Entries("earth"); len(got) != 1 {
		t.Fatalf("store must not re-import the stray entry: %+v", got)
	}
}

// TestExportHost_DefaultOverwritesWithWarning pins down requirement (1) of
// k81: with ExportOptions left at its zero value (Strict unset), a discard
// still warns-and-overwrites exactly as before Strict was introduced. This
// is a smaller, more direct check than
// TestExportHost_DiscardDetectionNamesDivergedEntries, which additionally
// exercises the warning's exact contents; here the point is only that the
// zero value never opts into fail-closed behavior.
func TestExportHost_DefaultOverwritesWithWarning(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	kept := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, kept); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{}); err != nil {
		t.Fatalf("first ExportHost: %v", err)
	}

	// Hand-edit db.earth.json to add an entry the store doesn't know about,
	// simulating worktime.rb or a human writing between exports.
	strayRaw := `{
  "entries": {
    "earth": [
      {"action":"login","what":"work","epoch":100,"source":"earth","human":"h"},
      {"action":"add","what":"offsite","epoch":150,"source":"earth","human":"h","value":900}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.earth.json"), []byte(strayRaw), 0o644); err != nil {
		t.Fatal(err)
	}

	var warn bytes.Buffer
	// ExportOptions{} (Strict unset, i.e. false) must behave exactly like
	// the pre-k81 signature did: warn, then overwrite.
	result, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("default ExportHost must never error on discard: %v", err)
	}
	if len(result.Discarded) != 1 {
		t.Fatalf("Discarded = %+v, want 1 entry", result.Discarded)
	}
	if warn.Len() == 0 {
		t.Fatal("expected a discard warning in default mode")
	}

	db, err := LoadLegacyHost(ctx, dbDir, "earth")
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	if len(db.Entries["earth"]) != 1 {
		t.Fatalf("post-export legacy entries = %+v, want only the login (overwrite must have proceeded)", db.Entries["earth"])
	}
}

// TestExportHost_StrictRefusesOnDiscard covers requirement (2) of k81: with
// ExportOptions.Strict set, a discard makes ExportHost refuse to write at
// all instead of overwriting. The on-disk file must be left byte-for-byte
// as the external edit left it, and the returned error must be a clear,
// matchable failure (wrapping ErrExportWouldDiscard) rather than a bare
// overwrite-with-warning.
func TestExportHost_StrictRefusesOnDiscard(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	kept := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, kept); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{}); err != nil {
		t.Fatalf("first ExportHost: %v", err)
	}

	// Externally modify db.earth.json after the last export -- worktime.rb
	// or a hand edit adding an entry the store has never heard of.
	legacyPath := filepath.Join(dbDir, "db.earth.json")
	strayRaw := `{
  "entries": {
    "earth": [
      {"action":"login","what":"work","epoch":100,"source":"earth","human":"h"},
      {"action":"add","what":"offsite","epoch":150,"source":"earth","human":"h","value":900,"descr":"hand-added"}
    ]
  }
}`
	if err := os.WriteFile(legacyPath, []byte(strayRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy file before strict export: %v", err)
	}

	var warn bytes.Buffer
	result, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{Strict: true, WarnOut: &warn})
	if err == nil {
		t.Fatal("expected strict ExportHost to refuse, got nil error")
	}
	if !errors.Is(err, ErrExportWouldDiscard) {
		t.Fatalf("error = %v, want it to wrap ErrExportWouldDiscard", err)
	}
	for _, want := range []string{"earth", "strict"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
	if len(result.Discarded) != 1 {
		t.Fatalf("result.Discarded = %+v, want the 1 entry that triggered the refusal", result.Discarded)
	}
	// Strict mode reports the refusal through the returned error, not
	// stderr-style warning output.
	if warn.Len() != 0 {
		t.Fatalf("strict refusal must not also print the warn-and-proceed banner, got:\n%s", warn.String())
	}

	// The file must be left exactly as the external edit left it: refusing
	// means refusing to write, not writing a partial or fresh version.
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy file after strict export: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("strict refusal must leave the on-disk file untouched:\nbefore:\n%s\nafter:\n%s", before, after)
	}

	// And, as always, the discarded entry must never be folded into the
	// store even when it triggered a refusal.
	if got := store.Entries("earth"); len(got) != 1 {
		t.Fatalf("store must not re-import the stray entry: %+v", got)
	}
}

func TestExportHost_NeverErrorsWithMultipleDiscards(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// The store knows nothing at all; every on-disk entry is stray.
	strayRaw := `{
  "entries": {
    "mars": [
      {"action":"login","what":"work","epoch":10,"source":"mars","human":"h"},
      {"action":"logout","what":"work","epoch":20,"source":"mars","human":"h"},
      {"action":"add","what":"work","epoch":30,"source":"mars","human":"h","value":120}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.mars.json"), []byte(strayRaw), 0o644); err != nil {
		t.Fatal(err)
	}

	var warn bytes.Buffer
	result, err := ExportHost(ctx, store, dbDir, "mars", ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("ExportHost must never error: %v", err)
	}
	if len(result.Discarded) != 3 {
		t.Fatalf("Discarded = %+v, want 3", result.Discarded)
	}
	if result.Written != 0 {
		t.Fatalf("Written = %d, want 0 (store has nothing for mars)", result.Written)
	}

	db, err := LoadLegacyHost(ctx, dbDir, "mars")
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	if len(db.Entries["mars"]) != 0 {
		t.Fatalf("post-export legacy entries = %+v, want empty", db.Entries["mars"])
	}
}

func TestExportAll_MultiHost(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	earthEntries := []Entry{
		{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}},
		{ID: 2, Action: "logout", Epoch: 200, Host: "earth", Tags: []string{"work"}},
	}
	marsEntries := []Entry{
		{ID: 1, Action: "add", Epoch: 50, Host: "mars", Value: 1800, Tags: []string{"selfdevelopment"}, Descr: "reading"},
	}
	for _, e := range append(append([]Entry{}, earthEntries...), marsEntries...) {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("Append %+v: %v", e, err)
		}
	}

	var warn bytes.Buffer
	results, err := ExportAll(ctx, store, dbDir, ExportOptions{WarnOut: &warn})
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want 2 hosts", results)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warnings:\n%s", warn.String())
	}

	wantByHost := map[string]int{"earth": 2, "mars": 1}
	for _, r := range results {
		if want := wantByHost[r.Host]; r.Written != want {
			t.Fatalf("host %q Written = %d, want %d", r.Host, r.Written, want)
		}
		if len(r.Discarded) != 0 {
			t.Fatalf("host %q Discarded = %+v, want empty", r.Host, r.Discarded)
		}
	}

	for host, want := range wantByHost {
		path := filepath.Join(dbDir, "db."+host+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		db, err := LoadLegacyHost(ctx, dbDir, host)
		if err != nil {
			t.Fatalf("LoadLegacyHost(%q): %v", host, err)
		}
		if got := len(db.Entries[host]); got != want {
			t.Fatalf("host %q legacy entries = %d, want %d", host, got, want)
		}
	}

	marsDB, err := LoadLegacyHost(ctx, dbDir, "mars")
	if err != nil {
		t.Fatalf("LoadLegacyHost(mars): %v", err)
	}
	mars := marsDB.Entries["mars"][0]
	if mars.What != "selfdevelopment" || mars.Value != 1800 || mars.Descr != "reading" {
		t.Fatalf("mars entry = %+v", mars)
	}
}

func TestExportHost_RejectsInvalidHost(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if _, err := ExportHost(ctx, store, dbDir, "  ", ExportOptions{}); err == nil {
		t.Fatal("expected error for empty host")
	}
	if _, err := ExportHost(ctx, store, dbDir, "../evil", ExportOptions{}); err == nil {
		t.Fatal("expected error for path-traversal host")
	}
}

func TestExportHost_RespectsCancelledContext(t *testing.T) {
	storeDir := t.TempDir()
	dbDir := t.TempDir()

	store, err := Open(context.Background(), storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{}); err == nil {
		t.Fatal("expected context error")
	}
}

func TestEntryToLegacy_CollapsesMultipleTagsToFirst(t *testing.T) {
	entry := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work", "offsite"}}
	leg := entryToLegacy("earth", entry)
	if leg.What != "work" {
		t.Fatalf("What = %q, want %q", leg.What, "work")
	}
	if leg.HasValue() {
		t.Fatalf("login entry must not carry a value: %+v", leg)
	}
}

func TestDiscardedLegacyEntries_MultisetMatching(t *testing.T) {
	a := LegacyEntry{Action: "login", What: "work", Epoch: 100, Source: "earth", Human: "x"}
	b := LegacyEntry{Action: "login", What: "work", Epoch: 100, Source: "earth", Human: "y"}

	// Two content-identical on-disk entries, only one fresh counterpart:
	// exactly one must be reported discarded, not zero or two.
	discarded := discardedLegacyEntries([]LegacyEntry{a, b}, []LegacyEntry{a})
	if len(discarded) != 1 {
		t.Fatalf("discarded = %+v, want exactly 1", discarded)
	}

	// Same on both sides: nothing discarded.
	discarded = discardedLegacyEntries([]LegacyEntry{a}, []LegacyEntry{b})
	if len(discarded) != 0 {
		t.Fatalf("discarded = %+v, want none (Source/Human differ but content matches)", discarded)
	}
}
