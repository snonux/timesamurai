package legacy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFormatLegacyHuman(t *testing.T) {
	epoch := time.Date(2020, 11, 15, 9, 29, 40, 0, time.Local).Unix()
	got := FormatLegacyHuman(epoch)
	want := time.Unix(epoch, 0).Local().Format("Mon 02.01.2006 15:04:05")
	if got != want {
		t.Fatalf("FormatLegacyHuman() = %q, want %q", got, want)
	}
	if !strings.Contains(got, "15.11.2020") {
		t.Fatalf("FormatLegacyHuman() = %q, want date 15.11.2020", got)
	}
}

func TestLegacyEntryUnmarshalJSONValueEncodings(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int64
		wantSet bool
		wantErr string
	}{
		{
			name:    "int value",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":5400}`,
			want:    5400,
			wantSet: true,
		},
		{
			name:    "zero value preserved",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":0}`,
			want:    0,
			wantSet: true,
		},
		{
			name:    "float value rounded",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":31680.000000000004}`,
			want:    31680,
			wantSet: true,
		},
		{
			name:    "negative float",
			raw:     `{"action":"add","what":"selfdevelopment","epoch":10,"source":"h","human":"x","value":-3600.4}`,
			want:    -3600,
			wantSet: true,
		},
		{
			name:    "numeric string",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":"7200"}`,
			want:    7200,
			wantSet: true,
		},
		{
			name:    "null value",
			raw:     `{"action":"login","what":"work","epoch":10,"source":"h","human":"x","value":null}`,
			want:    0,
			wantSet: false,
		},
		{
			name:    "missing value",
			raw:     `{"action":"login","what":"work","epoch":10,"source":"h","human":"x"}`,
			want:    0,
			wantSet: false,
		},
		{
			name:    "non-numeric string",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":"nope"}`,
			wantErr: `parse string value "nope"`,
		},
		{
			name:    "unsupported object value",
			raw:     `{"action":"add","what":"work","epoch":10,"source":"h","human":"x","value":{"x":1}}`,
			wantErr: "unsupported value encoding",
		},
		{
			name:    "invalid json",
			raw:     `{"action":`,
			wantErr: "unexpected end of JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entry LegacyEntry
			err := json.Unmarshal([]byte(tt.raw), &entry)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if entry.Value != tt.want {
				t.Fatalf("Value = %d, want %d", entry.Value, tt.want)
			}
			if entry.HasValue() != tt.wantSet {
				t.Fatalf("HasValue() = %v, want %v", entry.HasValue(), tt.wantSet)
			}
		})
	}
}

func TestLegacyEntryMarshalJSONKeyOrder(t *testing.T) {
	entry := LegacyEntry{
		Action: "add",
		What:   "work",
		Epoch:  1787951450,
		Source: "earth",
		Human:  "Thu 01.01.1970 00:00:00",
		Descr:  "note",
	}
	entry.SetValue(7200)

	data, err := json.Marshal(&entry)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	wantPrefix := `{"action":"add","what":"work","epoch":1787951450,"source":"earth","human":"Thu 01.01.1970 00:00:00","value":7200,"descr":"note"}`
	if got != wantPrefix {
		t.Fatalf("MarshalJSON mismatch:\n got %s\nwant %s", got, wantPrefix)
	}
}

func TestLegacyEntryMarshalJSONOmitsUnsetValueAndDescr(t *testing.T) {
	entry := LegacyEntry{
		Action: "login",
		What:   "work",
		Epoch:  10,
		Source: "earth",
		Human:  "h",
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	want := `{"action":"login","what":"work","epoch":10,"source":"earth","human":"h"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if strings.Contains(got, `"value"`) || strings.Contains(got, `"descr"`) {
		t.Fatalf("unexpected value/descr in %s", got)
	}
}

func TestLegacyEntryMarshalJSONZeroValue(t *testing.T) {
	entry := LegacyEntry{
		Action: "add",
		What:   "work",
		Epoch:  10,
		Source: "earth",
		Human:  "h",
	}
	entry.SetValue(0)

	data, err := json.Marshal(&entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"value":0`) {
		t.Fatalf("expected value:0 in %s", data)
	}
}

func TestSaveLegacyHostMatchesRubyShape(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	host := "host-a"
	epoch := time.Date(2020, 11, 15, 9, 29, 40, 0, time.Local).Unix()

	add := LegacyEntry{Action: "add", What: "work", Epoch: epoch, Descr: "x"}
	add.SetValue(0)
	login := LegacyEntry{Action: "login", What: "work", Epoch: 20}

	input := LegacyDB{
		Entries: map[string][]LegacyEntry{
			host: {add, login},
		},
	}
	if err := SaveLegacyHost(ctx, dbDir, host, input); err != nil {
		t.Fatalf("SaveLegacyHost() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dbDir, "db."+host+".json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.HasSuffix(got, "\n") {
		t.Fatal("legacy JSON must not have trailing newline (Ruby JSON.pretty_generate)")
	}
	if !strings.Contains(got, "\n  \"entries\"") {
		t.Fatalf("expected 2-space indent, got:\n%s", got)
	}

	human := FormatLegacyHuman(epoch)
	wantFrag := "{\n" +
		"  \"entries\": {\n" +
		"    \"host-a\": [\n" +
		"      {\n" +
		"        \"action\": \"login\",\n" +
		"        \"what\": \"work\",\n" +
		"        \"epoch\": 20,\n" +
		"        \"source\": \"host-a\",\n" +
		"        \"human\": \"" + FormatLegacyHuman(20) + "\"\n" +
		"      },\n" +
		"      {\n" +
		"        \"action\": \"add\",\n" +
		"        \"what\": \"work\",\n" +
		"        \"epoch\": " + strconv.FormatInt(epoch, 10) + ",\n" +
		"        \"source\": \"host-a\",\n" +
		"        \"human\": \"" + human + "\",\n" +
		"        \"value\": 0,\n" +
		"        \"descr\": \"x\"\n" +
		"      }\n" +
		"    ]\n" +
		"  }\n" +
		"}"
	if got != wantFrag {
		t.Fatalf("Ruby shape mismatch:\n got:\n%s\nwant:\n%s", got, wantFrag)
	}
}

func TestSortLegacyEntriesPreservesSameEpochOrder(t *testing.T) {
	// Ruby is stable sort_by epoch only; login then add at the same epoch must stay.
	entries := []LegacyEntry{
		{Action: "login", What: "work", Epoch: 100, Source: "h"},
		{Action: "add", What: "work", Epoch: 100, Source: "h"},
		{Action: "logout", What: "work", Epoch: 50, Source: "h"},
	}
	sortLegacyEntries(entries)
	if entries[0].Action != "logout" || entries[0].Epoch != 50 {
		t.Fatalf("expected logout first, got %+v", entries[0])
	}
	if entries[1].Action != "login" || entries[2].Action != "add" {
		t.Fatalf("same-epoch order not preserved: %+v %+v", entries[1], entries[2])
	}
}

func TestSaveLegacyHostAndLoadLegacyHostRoundTrip(t *testing.T) {
	ctx := context.Background()
	dbDir := filepath.Join(t.TempDir(), "nested", "db")
	host := "workstation"
	epochLater := int64(20)
	epochEarlier := int64(10)

	input := LegacyDB{
		Entries: map[string][]LegacyEntry{
			host: {
				{Action: "add", What: "work", Epoch: epochLater, Descr: "later"},
				{Action: "login", What: "work", Epoch: epochEarlier},
			},
		},
	}
	input.Entries[host][0].SetValue(1800)

	if err := SaveLegacyHost(ctx, dbDir, host, input); err != nil {
		t.Fatalf("SaveLegacyHost() error = %v", err)
	}

	output, err := LoadLegacyHost(ctx, dbDir, host)
	if err != nil {
		t.Fatalf("LoadLegacyHost() error = %v", err)
	}
	entries := output.Entries[host]
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0].Epoch != epochEarlier || entries[1].Epoch != epochLater {
		t.Fatalf("entries not sorted by epoch: %+v", entries)
	}
	if entries[1].Value != 1800 || !entries[1].HasValue() {
		t.Fatalf("add value not preserved: %+v", entries[1])
	}
	if entries[0].Human != FormatLegacyHuman(epochEarlier) {
		t.Fatalf("human = %q, want derived", entries[0].Human)
	}
}

func TestLoadLegacyHostMissingFileReturnsEmptyDatabase(t *testing.T) {
	db, err := LoadLegacyHost(context.Background(), t.TempDir(), "host-a")
	if err != nil {
		t.Fatalf("LoadLegacyHost() error = %v", err)
	}
	if len(db.Entries["host-a"]) != 0 {
		t.Fatalf("entries len = %d, want 0", len(db.Entries["host-a"]))
	}
}

func TestLoadLegacyAllMergesBackfillsAndSorts(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()

	if err := SaveLegacyHost(ctx, dbDir, "host-a", LegacyDB{
		Entries: map[string][]LegacyEntry{
			"host-a": {
				func() LegacyEntry {
					e := LegacyEntry{Action: "add", What: "work", Epoch: 30}
					e.SetValue(60)
					return e
				}(),
				{Action: "login", What: "work", Epoch: 10},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := SaveLegacyHost(ctx, dbDir, "host-b", LegacyDB{
		Entries: map[string][]LegacyEntry{
			"host-b": {{Action: "logout", What: "work", Epoch: 20}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Missing source should be backfilled from section key on load-all of raw file.
	raw := `{
  "entries": {
    "host-c": [
      {"action":"login","what":"work","epoch":5,"human":"h1"}
    ]
  }
}`
	if err := os.WriteFile(filepath.Join(dbDir, "db.host-c.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadLegacyAll(ctx, dbDir)
	if err != nil {
		t.Fatalf("LoadLegacyAll() error = %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("entries len = %d, want 4", len(entries))
	}
	if entries[0].Epoch != 5 || entries[0].Source != "host-c" {
		t.Fatalf("first entry = %+v, want epoch 5 source host-c", entries[0])
	}
	if entries[1].Epoch != 10 || entries[2].Epoch != 20 || entries[3].Epoch != 30 {
		t.Fatalf("not sorted: %+v", entries)
	}
}

func TestLoadLegacyNegatives(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		call    func(t *testing.T) error
		wantErr string
	}{
		{
			name: "LoadLegacyAll empty dir",
			call: func(t *testing.T) error {
				_, err := LoadLegacyAll(ctx, "")
				return err
			},
			wantErr: "db directory must not be empty",
		},
		{
			name: "LoadLegacyHost empty hostname",
			call: func(t *testing.T) error {
				_, err := LoadLegacyHost(ctx, t.TempDir(), "  ")
				return err
			},
			wantErr: "hostname must not be empty",
		},
		{
			name: "SaveLegacyHost empty hostname",
			call: func(t *testing.T) error {
				return SaveLegacyHost(ctx, t.TempDir(), "", LegacyDB{})
			},
			wantErr: "hostname must not be empty",
		},
		{
			name: "SaveLegacyHost path-unsafe hostname",
			call: func(t *testing.T) error {
				return SaveLegacyHost(ctx, t.TempDir(), "../escape", LegacyDB{})
			},
			wantErr: "invalid hostname",
		},
		{
			name: "SaveLegacyHost empty dir",
			call: func(t *testing.T) error {
				return SaveLegacyHost(ctx, "  ", "host-a", LegacyDB{})
			},
			wantErr: "db directory must not be empty",
		},
		{
			name: "LoadLegacyHost invalid JSON",
			call: func(t *testing.T) error {
				dbDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dbDir, "db.host-a.json"), []byte(`{"entries":`), 0o644); err != nil {
					t.Fatal(err)
				}
				_, err := LoadLegacyHost(ctx, dbDir, "host-a")
				return err
			},
			wantErr: "parse db file",
		},
		{
			name: "LoadLegacyAll invalid JSON",
			call: func(t *testing.T) error {
				dbDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dbDir, "db.host-a.json"), []byte(`{"entries":`), 0o644); err != nil {
					t.Fatal(err)
				}
				_, err := LoadLegacyAll(ctx, dbDir)
				return err
			},
			wantErr: "parse db file",
		},
		{
			name: "canceled context",
			call: func(t *testing.T) error {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				_, err := LoadLegacyAll(canceled, t.TempDir())
				return err
			},
			wantErr: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(t)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadLegacyAllMissingDirectoryReturnsEmpty(t *testing.T) {
	entries, err := LoadLegacyAll(context.Background(), filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("LoadLegacyAll() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(entries))
	}
}
