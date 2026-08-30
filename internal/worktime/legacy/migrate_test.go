package legacy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// Real-data reminders (not loaded here — fixtures stay small):
// unpaired earth login epoch 1781618168; 11 zero-value adds; 243 negative
// selfdevelopment (−829.96h); archive → mc-lon-mb8477 + galaxytabs6.

func TestMigrate_ImportsArchiveSplitAndFindings(t *testing.T) {
	ctx := context.Background()
	dbDir := filepath.Join("testdata", "migrate")
	storeDir := t.TempDir()

	var report bytes.Buffer
	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{Report: &report})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	wantHosts := []string{"earth", "galaxytabs6", "mc-lon-mb8477"}
	if strings.Join(result.Hosts, ",") != strings.Join(wantHosts, ",") {
		t.Fatalf("Hosts = %v, want %v", result.Hosts, wantHosts)
	}
	if result.Entries != 8 {
		t.Fatalf("Entries = %d, want 8", result.Entries)
	}

	// Archive must not produce db.archive.jsonl.
	if _, err := os.Stat(filepath.Join(storeDir, "db.archive.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("db.archive.jsonl must not exist, stat err=%v", err)
	}
	for _, host := range wantHosts {
		// db.<host>.jsonl is the store's documented on-disk naming
		// convention (see Store's doc comment in internal/worktime); this
		// test names it literally rather than importing the unexported
		// dbFileName helper, which stays package-private to worktime.
		wantFile := "db." + host + ".jsonl"
		if _, err := os.Stat(filepath.Join(storeDir, wantFile)); err != nil {
			t.Fatalf("missing %s: %v", wantFile, err)
		}
	}

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}

	earth := store.Entries("earth")
	if len(earth) != 5 {
		t.Fatalf("earth entries = %d, want 5", len(earth))
	}
	// Epoch-sorted: zero-add (100), negative (200), then logins/logout.
	if earth[0].Action != "add" || earth[0].Value != 0 || earth[0].Tags[0] != "work" {
		t.Fatalf("expected zero-value add first, got %+v", earth[0])
	}
	if earth[1].Action != "add" || earth[1].Value != -3600 || earth[1].Tags[0] != "selfdevelopment" {
		t.Fatalf("expected negative selfdevelopment, got %+v", earth[1])
	}
	if earth[2].Epoch != 1781618168 || earth[2].Action != "login" {
		t.Fatalf("expected unpaired login preserved, got %+v", earth[2])
	}

	gal := store.Entries("galaxytabs6")
	if len(gal) != 1 || gal[0].Descr != "notes" || gal[0].Value != 3600 {
		t.Fatalf("galaxytabs6: %+v", gal)
	}
	mb := store.Entries("mc-lon-mb8477")
	if len(mb) != 2 || mb[1].Value != -7200 {
		t.Fatalf("mc-lon-mb8477: %+v", mb)
	}

	kinds := findingKindCounts(result.Findings)
	if kinds[FindingUnpairedLogin] != 1 {
		t.Fatalf("unpaired-login findings = %d, want 1; findings=%v", kinds[FindingUnpairedLogin], result.Findings)
	}
	if kinds[FindingZeroValueAdd] != 1 {
		t.Fatalf("zero-value-add findings = %d, want 1", kinds[FindingZeroValueAdd])
	}
	if kinds[FindingNegativeValue] != 2 {
		t.Fatalf("negative-value findings = %d, want 2", kinds[FindingNegativeValue])
	}

	unpaired := findFinding(result.Findings, FindingUnpairedLogin)
	if unpaired.Epoch != 1781618168 {
		t.Fatalf("unpaired epoch = %d, want 1781618168", unpaired.Epoch)
	}
	if !strings.Contains(report.String(), FindingUnpairedLogin) {
		t.Fatalf("report missing unpaired finding:\n%s", report.String())
	}
}

func TestMigrate_RefuseSecondRunUnlessForce(t *testing.T) {
	ctx := context.Background()
	dbDir := filepath.Join("testdata", "migrate")
	storeDir := t.TempDir()

	if _, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{}); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	_, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{})
	if !errors.Is(err, ErrAlreadyMigrated) {
		t.Fatalf("second Migrate error = %v, want ErrAlreadyMigrated", err)
	}

	// Store unchanged: still three host files, earth still has 5 entries.
	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := len(store.Entries("earth")); got != 5 {
		t.Fatalf("earth entries after refused migrate = %d, want 5", got)
	}

	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{Force: true})
	if err != nil {
		t.Fatalf("force Migrate: %v", err)
	}
	if result.Entries != 8 {
		t.Fatalf("force Entries = %d, want 8", result.Entries)
	}
	store, err = worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	if got := len(store.Entries("earth")); got != 5 {
		t.Fatalf("earth after force = %d, want 5", got)
	}
}

func TestMigrate_TableNegatives(t *testing.T) {
	ctx := context.Background()
	dbDir := filepath.Join("testdata", "migrate")

	tests := []struct {
		name    string
		dbDir   string
		store   string
		opts    MigrateOptions
		wantErr string
	}{
		{
			name:    "empty db dir",
			dbDir:   "   ",
			store:   t.TempDir(),
			wantErr: "db directory must not be empty",
		},
		{
			name:    "empty store dir",
			dbDir:   dbDir,
			store:   " ",
			wantErr: "store directory must not be empty",
		},
		{
			name:    "missing legacy dir",
			dbDir:   filepath.Join(t.TempDir(), "nope"),
			store:   t.TempDir(),
			wantErr: "no legacy databases",
		},
		{
			name:    "cancelled context",
			dbDir:   dbDir,
			store:   t.TempDir(),
			wantErr: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runCtx := ctx
			if tt.name == "cancelled context" {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := Migrate(runCtx, tt.dbDir, tt.store, tt.opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestMigrate_EmptyHostSectionFile(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	storeDir := t.TempDir()

	raw := `{"entries":{"lonely":[]}}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.lonely.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(result.Hosts) != 1 || result.Hosts[0] != "lonely" || result.Entries != 0 {
		t.Fatalf("result = %+v", result)
	}
	// Empty host still records migration via an empty jsonl file.
	if _, err := os.Stat(filepath.Join(storeDir, "db.lonely.jsonl")); err != nil {
		t.Fatalf("expected empty host file: %v", err)
	}
}

func TestMigrate_PreservesSameEpochOrderAndIDs(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	storeDir := t.TempDir()

	raw := `{
  "entries": {
    "host-a": [
      {"action":"login","what":"work","epoch":100,"source":"host-a","human":"a"},
      {"action":"add","what":"work","epoch":100,"source":"host-a","human":"a","value":60},
      {"action":"logout","what":"work","epoch":50,"source":"host-a","human":"a"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.host-a.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if result.Entries != 3 {
		t.Fatalf("Entries = %d", result.Entries)
	}

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Entries("host-a")
	if got[0].Action != "logout" || got[0].ID != 1 {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].Action != "login" || got[1].ID != 2 || got[2].Action != "add" || got[2].ID != 3 {
		t.Fatalf("same-epoch order/ids: %+v %+v", got[1], got[2])
	}
}

// TestMigrate_ForceAfterDeleteReusesWatermarkNotOne is the regression test for
// task 481: a Force re-migrate used to renumber entries 1..len(entries),
// which collided with replaceHostLocked's id-reuse guard whenever the host's
// watermark had moved past 1 (e.g. because an earlier entry was deleted).
// This exercises exactly that: three entries land on disk with ids 1..3, id
// 1 is deleted (leaving a gap below the watermark of 4), and a Force migrate
// of a one-entry legacy host must still succeed and produce a consistent,
// non-colliding store.
func TestMigrate_ForceAfterDeleteReusesWatermarkNotOne(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	for i, action := range []string{"login", "logout", "add"} {
		entry := worktime.Entry{
			ID:     int64(i + 1),
			Action: action,
			Epoch:  int64(1000 + i),
			Host:   "earth",
			Tags:   []string{"work"},
		}
		if action == "add" {
			entry.Value = 60
		}
		if err := store.Append(ctx, entry); err != nil {
			t.Fatalf("Append entry %d: %v", i+1, err)
		}
	}
	if _, err := worktime.Delete(ctx, store, "earth:1", "earth"); err != nil {
		t.Fatalf("Delete earth:1: %v", err)
	}
	// Watermark stays at 4 (ids never reused) even though id 1 is now gone.
	if next, err := store.NextID("earth"); err != nil || next != 4 {
		t.Fatalf("NextID(earth) = %d, %v; want 4, nil", next, err)
	}

	dbDir := t.TempDir()
	raw := `{
  "entries": {
    "earth": [
      {"action":"login","what":"selfdevelopment","epoch":2000,"source":"earth","human":"e"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.earth.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{Force: true})
	if err != nil {
		t.Fatalf("force Migrate: %v", err)
	}
	if result.Entries != 1 {
		t.Fatalf("Entries = %d, want 1", result.Entries)
	}

	reopened, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("re-Open store: %v", err)
	}
	earth := reopened.Entries("earth")
	if len(earth) != 1 {
		t.Fatalf("earth entries after force migrate = %d, want 1: %+v", len(earth), earth)
	}
	// The migrated entry must land at or above the pre-migrate watermark (4),
	// never at the deleted id 1 — that would silently resurrect an id whose
	// undo history belongs to a different, now-discarded entry.
	if earth[0].ID < 4 {
		t.Fatalf("migrated entry id = %d, want >= 4 (pre-migrate watermark)", earth[0].ID)
	}
	if next, err := reopened.NextID("earth"); err != nil || next <= earth[0].ID {
		t.Fatalf("NextID(earth) after force migrate = %d, %v; want > %d", next, err, earth[0].ID)
	}

	// A follow-up Append must not error out on an id collision, confirming
	// the store's id bookkeeping is consistent after the force migrate.
	next, err := reopened.NextID("earth")
	if err != nil {
		t.Fatalf("NextID: %v", err)
	}
	if err := reopened.Append(ctx, worktime.Entry{ID: next, Action: "logout", Epoch: 3000, Host: "earth", Tags: []string{"work"}}); err != nil {
		t.Fatalf("Append after force migrate: %v", err)
	}
}

// TestMigrate_QuarantinesUnknownAction is the regression test for task 781:
// a legacy row with an unrecognized action (e.g. a typo'd "bogus") used to
// migrate cleanly into the store and only blow up later, opaquely, when
// someone ran `work report` ("unknown action bogus"). Migrate must instead
// surface a FindingUnknownAction finding immediately and never write the
// bad row, so the store stays report-safe straight after migration.
func TestMigrate_QuarantinesUnknownAction(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	storeDir := t.TempDir()

	raw := `{
  "entries": {
    "earth": [
      {"action":"login","what":"work","epoch":100,"source":"earth","human":"a"},
      {"action":"bogus","what":"work","epoch":150,"source":"earth","human":"a"},
      {"action":"logout","what":"work","epoch":200,"source":"earth","human":"a"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.earth.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	var report bytes.Buffer
	result, err := Migrate(ctx, dbDir, storeDir, MigrateOptions{Report: &report})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Only the two valid rows (login, logout) are imported; the bogus-action
	// row is quarantined, not counted as an entry.
	if result.Entries != 2 {
		t.Fatalf("Entries = %d, want 2 (bogus action must be quarantined, not imported)", result.Entries)
	}

	kinds := findingKindCounts(result.Findings)
	if kinds[FindingUnknownAction] != 1 {
		t.Fatalf("unknown-action findings = %d, want 1; findings=%v", kinds[FindingUnknownAction], result.Findings)
	}
	bad := findFinding(result.Findings, FindingUnknownAction)
	if bad.Action != "bogus" || bad.Epoch != 150 {
		t.Fatalf("unknown-action finding = %+v, want action=bogus epoch=150", bad)
	}
	if !strings.Contains(report.String(), FindingUnknownAction) || !strings.Contains(report.String(), "bogus") {
		t.Fatalf("report missing unknown-action finding:\n%s", report.String())
	}

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	earth := store.Entries("earth")
	if len(earth) != 2 {
		t.Fatalf("earth entries = %d, want 2", len(earth))
	}
	for _, e := range earth {
		if e.Action == "bogus" {
			t.Fatalf("store must not contain the quarantined bogus-action entry: %+v", earth)
		}
	}

	// The store must now be report-safe: BuildReport must not fail with
	// "unknown action" the way it did before this fix quarantined the row.
	if _, err := worktime.BuildReport(earth, config.Default().Accounting, io.Discard); err != nil {
		t.Fatalf("BuildReport after migrate: %v", err)
	}
}

func TestMigrateFindingString(t *testing.T) {
	f := MigrateFinding{
		Kind:   FindingUnpairedLogin,
		Host:   "earth",
		Epoch:  1781618168,
		Action: "login",
		Tag:    "work",
		Detail: "superseded",
	}
	s := f.String()
	for _, want := range []string{FindingUnpairedLogin, "earth", "1781618168", "work", "superseded"} {
		if !strings.Contains(s, want) {
			t.Fatalf("String() = %q missing %q", s, want)
		}
	}
}

func findingKindCounts(findings []MigrateFinding) map[string]int {
	counts := make(map[string]int)
	for _, f := range findings {
		counts[f.Kind]++
	}
	return counts
}

func findFinding(findings []MigrateFinding, kind string) MigrateFinding {
	for _, f := range findings {
		if f.Kind == kind {
			return f
		}
	}
	return MigrateFinding{}
}
