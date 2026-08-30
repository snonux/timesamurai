package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// blockHostRewrite pre-occupies db.<host>.jsonl.tmp with a directory, so
// Store.ReplaceHost's temp-file create fails for exactly that host while
// other hosts in the same migrate run are unaffected. Store's rewrite path
// (internal/worktime/store.go's rewriteJSONLFile) always creates that exact
// path before renaming it into place, so pre-claiming it as a directory is a
// clean, package-external way to simulate a per-host I/O failure without
// reaching into internal/worktime.
func blockHostRewrite(t *testing.T, storeDir, host string) string {
	t.Helper()
	blocked := filepath.Join(storeDir, "db."+host+".jsonl.tmp")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatalf("block rewrite for host %q: %v", host, err)
	}
	return blocked
}

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

// TestMigrate_PartialFailureReportsHostsAndForceRetryCompletes is the CLI
// regression test for task j81 (architecture resilience: `work migrate`
// must not leave a mid-run failure unclear). It mirrors
// legacy.TestMigrate_PartialHostFailureReportsAndRetrySucceeds but drives
// the failure through the actual `work migrate` command, confirming the
// partial-failure breakdown legacy.Migrate now writes to its report reaches
// the CLI's stdout, and that the surfaced error names the failed host and
// the --force retry path -- and that a --force retry then completes cleanly
// for every host, including the one that already succeeded.
func TestMigrate_PartialFailureReportsHostsAndForceRetryCompletes(t *testing.T) {
	dbDir := t.TempDir()
	storeDir := newScratchStore(t)

	writeLegacyFixture(t, dbDir, "okhost", `
		{"action":"login","what":"work","epoch":100,"source":"okhost","human":"h"},
		{"action":"logout","what":"work","epoch":200,"source":"okhost","human":"h"}
	`)
	writeLegacyFixture(t, dbDir, "brokenhost", `
		{"action":"login","what":"work","epoch":100,"source":"brokenhost","human":"h"},
		{"action":"logout","what":"work","epoch":200,"source":"brokenhost","human":"h"}
	`)
	blocked := blockHostRewrite(t, storeDir, "brokenhost")

	out, err := runWork(t, storeDir, "migrate", "--db", dbDir)
	if err == nil {
		t.Fatalf("expected a partial-failure error, output: %s", out)
	}
	if !strings.Contains(err.Error(), "brokenhost") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error %q must name the failed host and mention --force", err.Error())
	}
	if !strings.Contains(out, "succeeded: okhost") {
		t.Fatalf("stdout missing succeeded breakdown:\n%s", out)
	}
	if !strings.Contains(out, "failed: 1 host") || !strings.Contains(out, "brokenhost") {
		t.Fatalf("stdout missing failed breakdown:\n%s", out)
	}
	if got := len(readEntries(t, storeDir, "okhost")); got != 2 {
		t.Fatalf("okhost entries after partial failure = %d, want 2", got)
	}
	if got := len(readEntries(t, storeDir, "brokenhost")); got != 0 {
		t.Fatalf("brokenhost entries after partial failure = %d, want 0", got)
	}

	// Fix the underlying I/O problem and retry with --force: this
	// re-attempts both hosts and must succeed on okhost too, without
	// erroring on its already-migrated data (task 481's watermark fix).
	if err := os.RemoveAll(blocked); err != nil {
		t.Fatal(err)
	}
	out, err = runWork(t, storeDir, "migrate", "--db", dbDir, "--force")
	if err != nil {
		t.Fatalf("forced retry migrate: %v (output: %s)", err, out)
	}
	if got := len(readEntries(t, storeDir, "okhost")); got != 2 {
		t.Fatalf("okhost entries after forced retry = %d, want 2", got)
	}
	if got := len(readEntries(t, storeDir, "brokenhost")); got != 2 {
		t.Fatalf("brokenhost entries after forced retry = %d, want 2", got)
	}
}
