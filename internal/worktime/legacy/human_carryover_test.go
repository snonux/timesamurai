package legacy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/snonux/timesamurai/internal/worktime"
)

// TestExportPreservesHistoricalHuman locks in that export leaves an existing
// "human" string alone. worktime.rb writes it once from the local clock at
// insert time, so historical entries encode the timezone of the machine that
// logged them; re-deriving it would restate that history and churn every line
// of db.<host>.json on each export.
func TestExportPreservesHistoricalHuman(t *testing.T) {
	// A London-logged 2020 entry: epoch 1605432580 is 09:29:40 UTC/London
	// but 11:29:40 in Europe/Sofia, so a re-derived value differs wherever
	// the test runs outside London.
	const (
		epoch      = int64(1605432580)
		londonTime = "Sun 15.11.2020 09:29:40"
	)

	for _, tc := range []struct {
		name      string
		onDisk    []LegacyEntry
		wantHuman string
	}{
		{
			name:      "existing entry keeps its recorded local time",
			onDisk:    []LegacyEntry{{Action: "login", What: "work", Epoch: epoch, Source: "earth", Human: londonTime}},
			wantHuman: londonTime,
		},
		{
			name:      "entry absent from disk gets a derived timestamp",
			onDisk:    nil,
			wantHuman: FormatLegacyHuman(epoch),
		},
		{
			name:      "on-disk entry with a blank human gets a derived timestamp",
			onDisk:    []LegacyEntry{{Action: "login", What: "work", Epoch: epoch, Source: "earth"}},
			wantHuman: FormatLegacyHuman(epoch),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dbDir := t.TempDir()

			if tc.onDisk != nil {
				db := LegacyDB{Entries: map[string][]LegacyEntry{"earth": tc.onDisk}}
				if err := SaveLegacyHost(ctx, dbDir, "earth", db); err != nil {
					t.Fatalf("seed legacy host: %v", err)
				}
			}

			store, err := worktime.Open(ctx, filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}

			id, err := store.NextID("earth")
			if err != nil {
				t.Fatalf("next id: %v", err)
			}
			if err := store.Append(ctx, worktime.Entry{
				ID:     id,
				Action: worktime.ActionLogin,
				Epoch:  epoch,
				Host:   "earth",
				Tags:   []string{"work"},
			}); err != nil {
				t.Fatalf("append entry: %v", err)
			}

			if _, err := ExportHost(ctx, store, dbDir, "earth", ExportOptions{}); err != nil {
				t.Fatalf("export: %v", err)
			}

			got, err := LoadLegacyHost(ctx, dbDir, "earth")
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			entries := got.Entries["earth"]
			if len(entries) != 1 {
				t.Fatalf("got %d entries, want 1", len(entries))
			}
			if entries[0].Human != tc.wantHuman {
				t.Errorf("human = %q, want %q", entries[0].Human, tc.wantHuman)
			}
		})
	}
}

// TestCarryOverHumanMatchesMultiset checks that duplicate on-disk entries
// hand out their timestamps one apiece rather than one value winning twice,
// and that an entry whose epoch moved gets no carry-over at all.
func TestCarryOverHumanMatchesMultiset(t *testing.T) {
	onDisk := []LegacyEntry{
		{Action: "add", What: "work", Epoch: 100, Human: "first"},
		{Action: "add", What: "work", Epoch: 100, Human: "second"},
	}
	fresh := []LegacyEntry{
		{Action: "add", What: "work", Epoch: 100},
		{Action: "add", What: "work", Epoch: 100},
		{Action: "add", What: "work", Epoch: 999}, // epoch changed: no match
	}

	carryOverHuman(onDisk, fresh)

	if fresh[0].Human != "first" || fresh[1].Human != "second" {
		t.Errorf("duplicates got %q/%q, want first/second", fresh[0].Human, fresh[1].Human)
	}
	if fresh[2].Human != "" {
		t.Errorf("moved entry got %q, want it left blank for derivation", fresh[2].Human)
	}
}
