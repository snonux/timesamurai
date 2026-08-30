package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/worktime"
)

// TestExport_WritesLegacyJSON confirms `work export --db ...` regenerates
// db.<host>.json from the store's current entries for every known host.
func TestExport_WritesLegacyJSON(t *testing.T) {
	store := newScratchStore(t)
	dbDir := t.TempDir()
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "exported"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	out, err := runWork(t, store, "export", "--db", dbDir)
	if err != nil {
		t.Fatalf("work export: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "1 entries written") {
		t.Fatalf("output = %q, want it to report 1 entry written", out)
	}

	db, err := worktime.LoadLegacyHost(context.Background(), dbDir, host)
	if err != nil {
		t.Fatalf("LoadLegacyHost: %v", err)
	}
	got := db.Entries[host]
	if len(got) != 1 || got[0].What != "work" || got[0].Value != 3600 || got[0].Descr != "exported" {
		t.Fatalf("exported legacy entries = %+v", got)
	}
}

// TestExport_WarnsOnDiscard confirms a stale on-disk legacy entry with no
// counterpart in the fresh export triggers ExportHost's discard warning
// (written to stderr, which runWork captures in the same buffer as
// stdout) and is reflected in the printed summary.
func TestExport_WarnsOnDiscard(t *testing.T) {
	store := newScratchStore(t)
	dbDir := t.TempDir()
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	// Seed a legacy file with an entry the store has no idea about, so
	// export's diff against on-disk content finds a stale entry to discard.
	raw := `{"entries":{"` + host + `":[` +
		`{"action":"add","what":"work","epoch":999999,"source":"` + host + `","human":"stale","value":60}` +
		`]}}`
	if err := os.WriteFile(filepath.Join(dbDir, "db."+host+".json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	out, err := runWork(t, store, "export", "--db", dbDir)
	if err != nil {
		t.Fatalf("work export: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "WARNING") || !strings.Contains(out, "discarding") {
		t.Fatalf("output = %q, want a discard warning", out)
	}
	if !strings.Contains(out, "1 discarded") {
		t.Fatalf("output = %q, want the summary to mention 1 discarded", out)
	}
}

// TestExport_StrictRefusesOnDiscardAndLeavesFileUntouched confirms `work
// export --strict` (k81) refuses instead of overwriting when a stale
// on-disk legacy entry has no counterpart in the fresh export, returns a
// clear error, and leaves db.<host>.json exactly as it was.
func TestExport_StrictRefusesOnDiscardAndLeavesFileUntouched(t *testing.T) {
	store := newScratchStore(t)
	dbDir := t.TempDir()
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	legacyPath := filepath.Join(dbDir, "db."+host+".json")
	raw := `{"entries":{"` + host + `":[` +
		`{"action":"add","what":"work","epoch":999999,"source":"` + host + `","human":"stale","value":60}` +
		`]}}`
	if err := os.WriteFile(legacyPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}
	before, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy file before export: %v", err)
	}

	out, err := runWork(t, store, "export", "--db", dbDir, "--strict")
	if err == nil {
		t.Fatalf("expected `work export --strict` to fail, output: %s", out)
	}
	if !strings.Contains(err.Error(), "refused") && !strings.Contains(out, "refused") {
		t.Fatalf("error/output want a clear refusal message: err=%v output=%q", err, out)
	}

	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy file after export: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("--strict must leave db.%s.json untouched on refusal:\nbefore:\n%s\nafter:\n%s", host, before, after)
	}
}
