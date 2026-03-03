package worktime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const dbFilePattern = "db.*.json"

// Entry is a single worktime event in a host database.
type Entry struct {
	Action string `json:"action"`
	What   string `json:"what"`
	Epoch  int64  `json:"epoch"`
	Source string `json:"source"`
	Human  string `json:"human"`
	Value  int64  `json:"value,omitempty"`
	Descr  string `json:"descr,omitempty"`
}

// Database is the on-disk JSON structure used by worktime.
type Database struct {
	Entries map[string][]Entry `json:"entries"`
}

// LoadAll reads all db.*.json files from dbDir, merges entries, and sorts by epoch.
func LoadAll(dbDir string) ([]Entry, error) {
	if strings.TrimSpace(dbDir) == "" {
		return nil, errors.New("db directory must not be empty")
	}

	dbFiles, err := filepath.Glob(filepath.Join(dbDir, dbFilePattern))
	if err != nil {
		return nil, fmt.Errorf("glob databases in %q: %w", dbDir, err)
	}

	entries := make([]Entry, 0)
	for _, dbFile := range dbFiles {
		db, err := loadDatabaseFile(dbFile)
		if err != nil {
			return nil, err
		}
		for _, hostEntries := range db.Entries {
			entries = append(entries, hostEntries...)
		}
	}

	sortEntries(entries)
	return entries, nil
}

// LoadHost reads one host database from dbDir. Missing files return an empty host section.
func LoadHost(dbDir, hostname string) (Database, error) {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return Database{}, err
	}
	if strings.TrimSpace(dbDir) == "" {
		return Database{}, errors.New("db directory must not be empty")
	}

	dbFile := filepath.Join(dbDir, dbFileName(host))
	db, err := loadDatabaseFile(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			return newHostDatabase(host), nil
		}
		return Database{}, err
	}

	if _, ok := db.Entries[host]; !ok {
		db.Entries[host] = []Entry{}
	}
	sortEntries(db.Entries[host])
	return db, nil
}

// SaveHost writes one host database to dbDir as db.<hostname>.json.
func SaveHost(dbDir, hostname string, db Database) error {
	host, err := normalizeHostname(hostname)
	if err != nil {
		return err
	}
	if strings.TrimSpace(dbDir) == "" {
		return errors.New("db directory must not be empty")
	}

	if db.Entries == nil {
		db.Entries = map[string][]Entry{}
	}
	if _, ok := db.Entries[host]; !ok {
		db.Entries[host] = []Entry{}
	}
	sortEntries(db.Entries[host])

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("encode database for host %q: %w", host, err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create db directory %q: %w", dbDir, err)
	}

	dbFile := filepath.Join(dbDir, dbFileName(host))
	if err := os.WriteFile(dbFile, data, 0o644); err != nil {
		return fmt.Errorf("write db file %q: %w", dbFile, err)
	}

	return nil
}

func loadDatabaseFile(dbFile string) (Database, error) {
	var db Database

	data, err := os.ReadFile(dbFile)
	if err != nil {
		return db, err
	}

	if err := json.Unmarshal(data, &db); err != nil {
		return db, fmt.Errorf("parse db file %q: %w", dbFile, err)
	}

	if db.Entries == nil {
		db.Entries = map[string][]Entry{}
	}

	return db, nil
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Epoch != entries[j].Epoch {
			return entries[i].Epoch < entries[j].Epoch
		}
		if entries[i].Source != entries[j].Source {
			return entries[i].Source < entries[j].Source
		}
		return entries[i].Action < entries[j].Action
	})
}

func normalizeHostname(hostname string) (string, error) {
	host := strings.TrimSpace(hostname)
	if host == "" {
		return "", errors.New("hostname must not be empty")
	}
	return host, nil
}

func dbFileName(hostname string) string {
	return "db." + hostname + ".json"
}

func newHostDatabase(hostname string) Database {
	return Database{
		Entries: map[string][]Entry{
			hostname: {},
		},
	}
}
