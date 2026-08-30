package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLegacyFixture writes a minimal db.<host>.json under dbDir in the
// {"entries": {host: [...]}} shape legacy.Migrate reads, for tests that
// need a synthetic legacy database without depending on
// internal/worktime/legacy's own fixtures under testdata/migrate.
func writeLegacyFixture(t *testing.T, dbDir, host, entriesJSON string) {
	t.Helper()
	raw := `{"entries":{"` + host + `":[` + entriesJSON + `]}}`
	path := filepath.Join(dbDir, "db."+host+".json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy fixture %s: %v", path, err)
	}
}

// TestMigrate_ImportsIntoStore confirms `work migrate --db ... --store ...`
// converts a legacy db.<host>.json into the JSONL store and prints a
// findings report on stdout (writeMigrateReport's "migrated N host(s)..."
// summary line).
func TestMigrate_ImportsIntoStore(t *testing.T) {
	dbDir := t.TempDir()
	storeDir := newScratchStore(t)
	host := "migratehost"
	writeLegacyFixture(t, dbDir, host, `
		{"action":"add","what":"work","epoch":100,"source":"`+host+`","human":"h","value":3600}
	`)

	out, err := runWork(t, storeDir, "migrate", "--db", dbDir)
	if err != nil {
		t.Fatalf("work migrate: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "migrated 1 host") {
		t.Fatalf("output = %q, want it to mention migrating 1 host", out)
	}

	entries := readEntries(t, storeDir, host)
	if len(entries) != 1 || entries[0].Value != 3600 || entries[0].Tags[0] != "work" {
		t.Fatalf("migrated entries = %+v, want one work/3600 entry", entries)
	}
}

// TestMigrate_RefusesSecondRunUnlessForce confirms a second `work migrate`
// against the same store is refused with an ErrAlreadyMigrated-derived
// message naming --force, and that --force lets it re-run.
func TestMigrate_RefusesSecondRunUnlessForce(t *testing.T) {
	dbDir := t.TempDir()
	storeDir := newScratchStore(t)
	host := "refusehost"
	writeLegacyFixture(t, dbDir, host, `
		{"action":"add","what":"work","epoch":100,"source":"`+host+`","human":"h","value":1800}
	`)

	if _, err := runWork(t, storeDir, "migrate", "--db", dbDir); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	_, err := runWork(t, storeDir, "migrate", "--db", dbDir)
	if err == nil {
		t.Fatal("second migrate: expected an already-migrated error, got nil")
	}
	if !strings.Contains(err.Error(), "already migrated") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("second migrate error = %q, want it to mention already migrated and --force", err.Error())
	}
	// Refused migrate must leave the store untouched.
	if got := len(readEntries(t, storeDir, host)); got != 1 {
		t.Fatalf("entries after refused migrate = %d, want 1", got)
	}

	out, err := runWork(t, storeDir, "migrate", "--db", dbDir, "--force")
	if err != nil {
		t.Fatalf("forced migrate: %v (output: %s)", err, out)
	}
	if got := len(readEntries(t, storeDir, host)); got != 1 {
		t.Fatalf("entries after forced migrate = %d, want 1", got)
	}
}
