package legacy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/snonux/timesamurai/internal/worktime"
)

// TestExportWritesBackIntoMultiHostFile is the regression guard for the
// coexistence break found in review.
//
// worktime.rb's one-file-per-host layout is a convention, not a rule:
// db.archive.json carries both mc-lon-mb8477 and galaxytabs6. Because
// worktime.rb globs db.*.json and merges every section it finds, exporting
// such a host into a fresh db.<host>.json while the original file is still
// present makes it read those entries twice -- doubling the hours for those
// weeks and desynchronising login/logout pairing until the report aborts
// with "Not logged in". Export must therefore write back into the file that
// already owns the host, leaving its sibling sections intact.
func TestExportWritesBackIntoMultiHostFile(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()

	// An archive-style file holding two hosts, neither named after it.
	archive := LegacyDB{Entries: map[string][]LegacyEntry{
		"host-a": {{Action: "login", What: "work", Epoch: 100, Source: "host-a", Human: "a1"}},
		"host-b": {{Action: "login", What: "work", Epoch: 200, Source: "host-b", Human: "b1"}},
	}}
	data, err := encodeLegacyDBForTest(archive)
	if err != nil {
		t.Fatalf("encode archive: %v", err)
	}
	archivePath := filepath.Join(dbDir, "db.archive.json")
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	store, err := worktime.Open(ctx, filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	id, err := store.NextID("host-a")
	if err != nil {
		t.Fatalf("next id: %v", err)
	}
	if err := store.Append(ctx, worktime.Entry{
		ID: id, Action: worktime.ActionLogin, Epoch: 100, Host: "host-a", Tags: []string{"work"},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if _, err := ExportHost(ctx, store, dbDir, "host-a", ExportOptions{}); err != nil {
		t.Fatalf("export host-a: %v", err)
	}

	// No stray per-host file: that duplicate is exactly what broke worktime.rb.
	if _, err := os.Stat(filepath.Join(dbDir, "db.host-a.json")); !os.IsNotExist(err) {
		t.Error("export created db.host-a.json while db.archive.json still owns host-a")
	}

	reloaded, err := loadLegacyFile(archivePath)
	if err != nil {
		t.Fatalf("reload archive: %v", err)
	}
	if got := len(reloaded.Entries["host-a"]); got != 1 {
		t.Errorf("host-a section has %d entries, want 1", got)
	}
	// The sibling host must survive untouched.
	sibling := reloaded.Entries["host-b"]
	if len(sibling) != 1 || sibling[0].Epoch != 200 || sibling[0].Human != "b1" {
		t.Errorf("host-b section was disturbed: %+v", sibling)
	}
}

// TestResolveLegacyHostFilePrefersCanonical checks the ordinary one-file-per-
// host case still wins, and that an unknown host resolves to the name it
// would be created under.
func TestResolveLegacyHostFilePrefersCanonical(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()

	canonical := filepath.Join(dbDir, "db.earth.json")
	db := LegacyDB{Entries: map[string][]LegacyEntry{"earth": {}}}
	data, err := encodeLegacyDBForTest(db)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(canonical, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A second file also claiming earth must lose to the canonical name.
	other := LegacyDB{Entries: map[string][]LegacyEntry{"earth": {}}}
	data, err = encodeLegacyDBForTest(other)
	if err != nil {
		t.Fatalf("encode other: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dbDir, "db.aaa-other.json"), data, 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	got, err := resolveLegacyHostFile(ctx, dbDir, "earth")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != canonical {
		t.Errorf("resolve(earth) = %q, want canonical %q", got, canonical)
	}

	want := filepath.Join(dbDir, "db.newhost.json")
	got, err = resolveLegacyHostFile(ctx, dbDir, "newhost")
	if err != nil {
		t.Fatalf("resolve newhost: %v", err)
	}
	if got != want {
		t.Errorf("resolve(newhost) = %q, want %q", got, want)
	}
}

// encodeLegacyDBForTest writes the on-disk shape directly rather than going
// through SaveLegacyHost, which is the function under test here and would
// route a multi-host fixture through the very resolution being verified.
func encodeLegacyDBForTest(db LegacyDB) ([]byte, error) {
	return json.MarshalIndent(db, "", "  ")
}
