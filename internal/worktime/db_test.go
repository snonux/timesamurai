package worktime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHostMissingFileReturnsEmptyDatabase(t *testing.T) {
	dbDir := t.TempDir()

	db, err := LoadHost(dbDir, "host-a")
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}

	hostEntries, ok := db.Entries["host-a"]
	if !ok {
		t.Fatal("LoadHost() missing host section")
	}
	if len(hostEntries) != 0 {
		t.Fatalf("LoadHost() entries len = %d, want 0", len(hostEntries))
	}
}

func TestLoadHostRejectsEmptyHostname(t *testing.T) {
	_, err := LoadHost(t.TempDir(), "")
	if err == nil {
		t.Fatal("LoadHost() error = nil, want error")
	}
}

func TestSaveHostRejectsEmptyHostname(t *testing.T) {
	err := SaveHost(t.TempDir(), "  ", Database{})
	if err == nil {
		t.Fatal("SaveHost() error = nil, want error")
	}
}

func TestSaveHostAndLoadHostRoundTrip(t *testing.T) {
	dbDir := filepath.Join(t.TempDir(), "nested", "db")
	host := "workstation"

	input := Database{
		Entries: map[string][]Entry{
			host: {
				{
					Action: "add",
					What:   "work",
					Epoch:  20,
					Source: host,
					Human:  "Tue 01.01.2026 10:00:00",
					Value:  1800,
					Descr:  "later",
				},
				{
					Action: "login",
					What:   "work",
					Epoch:  10,
					Source: host,
					Human:  "Tue 01.01.2026 09:00:00",
				},
			},
		},
	}

	if err := SaveHost(dbDir, host, input); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	dbFile := filepath.Join(dbDir, "db."+host+".json")
	if _, err := os.Stat(dbFile); err != nil {
		t.Fatalf("db file not created: %v", err)
	}

	output, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}

	hostEntries := output.Entries[host]
	if len(hostEntries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(hostEntries))
	}

	if hostEntries[0].Epoch != 10 || hostEntries[1].Epoch != 20 {
		t.Fatalf("entries not sorted by epoch: %+v", hostEntries)
	}
}

func TestLoadAllMergesAndSortsEntries(t *testing.T) {
	dbDir := t.TempDir()

	dbA := Database{
		Entries: map[string][]Entry{
			"host-a": {
				{Action: "add", What: "work", Epoch: 30, Source: "host-a", Human: "h3", Value: 60},
				{Action: "login", What: "work", Epoch: 10, Source: "host-a", Human: "h1"},
			},
		},
	}
	dbB := Database{
		Entries: map[string][]Entry{
			"host-b": {
				{Action: "logout", What: "work", Epoch: 20, Source: "host-b", Human: "h2"},
			},
		},
	}

	if err := SaveHost(dbDir, "host-a", dbA); err != nil {
		t.Fatalf("SaveHost(host-a) error = %v", err)
	}
	if err := SaveHost(dbDir, "host-b", dbB); err != nil {
		t.Fatalf("SaveHost(host-b) error = %v", err)
	}

	entries, err := LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("entries len = %d, want 3", len(entries))
	}

	if entries[0].Epoch != 10 || entries[1].Epoch != 20 || entries[2].Epoch != 30 {
		t.Fatalf("entries not merged/sorted: %+v", entries)
	}
}

func TestLoadAllBackfillsMissingSourceFromHost(t *testing.T) {
	dbDir := t.TempDir()
	dbFile := filepath.Join(dbDir, "db.host-a.json")
	content := `{
  "entries": {
    "host-a": [
      {"action":"login","what":"work","epoch":10,"human":"h1"}
    ]
  }
}`
	if err := os.WriteFile(dbFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	entries, err := LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Source != "host-a" {
		t.Fatalf("entries[0].Source = %q, want host-a", entries[0].Source)
	}
}

func TestLoadAllOnMissingDirectoryReturnsEmptySlice(t *testing.T) {
	dbDir := filepath.Join(t.TempDir(), "does-not-exist")

	entries, err := LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("entries len = %d, want 0", len(entries))
	}
}

func TestLoadHostInvalidJSON(t *testing.T) {
	dbDir := t.TempDir()
	badFile := filepath.Join(dbDir, "db.host-a.json")
	if err := os.WriteFile(badFile, []byte(`{"entries":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadHost(dbDir, "host-a")
	if err == nil {
		t.Fatal("LoadHost() error = nil, want parse error")
	}

	if !strings.Contains(err.Error(), "parse db file") {
		t.Fatalf("LoadHost() error = %v, want parse db file context", err)
	}
}

func TestLoadAllAcceptsFloatValueEncoding(t *testing.T) {
	dbDir := t.TempDir()
	dbFile := filepath.Join(dbDir, "db.host-a.json")
	content := `{
  "entries": {
    "host-a": [
      {"action":"add","what":"work","epoch":10,"source":"host-a","human":"h1","value":31680.000000000004}
    ]
  }
}`
	if err := os.WriteFile(dbFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	entries, err := LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Value != 31680 {
		t.Fatalf("entries[0].Value = %d, want 31680", entries[0].Value)
	}
}

func TestLoadAllInvalidJSON(t *testing.T) {
	dbDir := t.TempDir()
	badFile := filepath.Join(dbDir, "db.host-a.json")
	if err := os.WriteFile(badFile, []byte(`{"entries":`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := LoadAll(dbDir)
	if err == nil {
		t.Fatal("LoadAll() error = nil, want parse error")
	}
}

func TestLoadAllRejectsEmptyDirectory(t *testing.T) {
	_, err := LoadAll("")
	if err == nil {
		t.Fatal("LoadAll() error = nil, want error")
	}
}
