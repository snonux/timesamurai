// Package legacy holds the worktime.rb-era codec, one-shot migration into
// the JSONL store, and export back out of it — everything that exists only
// for the dual-tool coexistence window rather than for this tool's own
// runtime domain logic (see internal/worktime's doc comment for the SRP
// rationale behind the split, task e81). It depends on internal/worktime
// for Store, Entry, and the small set of action constants/helpers it
// exports for exactly this purpose; internal/worktime never depends back on
// this package.
package legacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/worktime"
)

const (
	legacyDBFilePattern = "db.*.json"
	legacyHumanLayout   = "Mon 02.01.2006 15:04:05"
)

// LegacyEntry is one event in a worktime.rb db.*.json file.
// MarshalJSON emits keys in Ruby insert order: action, what, epoch, source,
// human, value, descr. value and descr are omitted when unset.
type LegacyEntry struct {
	Action string `json:"action"`
	What   string `json:"what"`
	Epoch  int64  `json:"epoch"`
	Source string `json:"source"`
	Human  string `json:"human"`
	Value  int64  `json:"value,omitempty"`
	Descr  string `json:"descr,omitempty"`

	// valueSet is true when "value" was present on decode or set via SetValue,
	// so encoding can emit value:0 for inert add entries.
	valueSet bool
}

// LegacyDB is the on-disk JSON object used by worktime.rb.
type LegacyDB struct {
	Entries map[string][]LegacyEntry `json:"entries"`
}

// SetValue records v for encoding, including zero.
func (e *LegacyEntry) SetValue(v int64) {
	e.Value = v
	e.valueSet = true
}

// ClearValue drops the value field from subsequent encoding.
func (e *LegacyEntry) ClearValue() {
	e.Value = 0
	e.valueSet = false
}

// HasValue reports whether a value field should be written.
// Pointer receiver for consistency with the other LegacyEntry methods
// (UnmarshalJSON must be a pointer receiver to mutate the receiver, so all
// methods on this type use pointer receivers per Go convention).
func (e *LegacyEntry) HasValue() bool {
	return e.valueSet
}

// FormatLegacyHuman renders epoch like worktime.rb get_human:
// strftime('%a %d.%m.%Y %H:%M:%S') in the local timezone.
func FormatLegacyHuman(epoch int64) string {
	return time.Unix(epoch, 0).Local().Format(legacyHumanLayout)
}

// UnmarshalJSON supports legacy value encodings where "value" can be int,
// float, or numeric string — matching encodings found in real history.
func (e *LegacyEntry) UnmarshalJSON(data []byte) error {
	type entryAlias LegacyEntry
	aux := struct {
		entryAlias
		Value json.RawMessage `json:"value"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	*e = LegacyEntry(aux.entryAlias)
	e.Value = 0
	e.valueSet = false

	raw := strings.TrimSpace(string(aux.Value))
	if raw == "" || raw == "null" {
		return nil
	}

	var intValue int64
	if err := json.Unmarshal(aux.Value, &intValue); err == nil {
		e.SetValue(intValue)
		return nil
	}

	var floatValue float64
	if err := json.Unmarshal(aux.Value, &floatValue); err == nil {
		e.SetValue(int64(math.Round(floatValue)))
		return nil
	}

	var stringValue string
	if err := json.Unmarshal(aux.Value, &stringValue); err == nil {
		parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(stringValue), 64)
		if parseErr != nil {
			return fmt.Errorf("parse string value %q: %w", stringValue, parseErr)
		}
		e.SetValue(int64(math.Round(parsed)))
		return nil
	}

	return fmt.Errorf("unsupported value encoding %s", raw)
}

// MarshalJSON writes fields in worktime.rb insert order.
// Pointer receiver for consistency with the other LegacyEntry methods; see
// HasValue's comment. Slice elements (e.g. []LegacyEntry) are addressable in
// Go, so encoding/json still finds and uses this method when marshaling a
// LegacyEntry stored in a slice or map-of-slice — only marshaling a bare,
// non-addressable LegacyEntry value directly would silently fall back to
// default struct encoding instead of failing to compile.
func (e *LegacyEntry) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(128)
	buf.WriteByte('{')

	if err := writeJSONString(&buf, "action", e.Action, false); err != nil {
		return nil, err
	}
	if err := writeJSONString(&buf, "what", e.What, true); err != nil {
		return nil, err
	}
	buf.WriteString(`,"epoch":`)
	buf.WriteString(strconv.FormatInt(e.Epoch, 10))
	if err := writeJSONString(&buf, "source", e.Source, true); err != nil {
		return nil, err
	}
	if err := writeJSONString(&buf, "human", e.Human, true); err != nil {
		return nil, err
	}
	if e.valueSet {
		buf.WriteString(`,"value":`)
		buf.WriteString(strconv.FormatInt(e.Value, 10))
	}
	if e.Descr != "" {
		if err := writeJSONString(&buf, "descr", e.Descr, true); err != nil {
			return nil, err
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// LoadLegacyAll reads every db.*.json under dbDir, merges entries, backfills
// empty source from the host section key, and sorts by epoch.
func LoadLegacyAll(ctx context.Context, dbDir string) ([]LegacyEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(dbDir) == "" {
		return nil, errors.New("db directory must not be empty")
	}

	dbFiles, err := filepath.Glob(filepath.Join(dbDir, legacyDBFilePattern))
	if err != nil {
		return nil, fmt.Errorf("glob databases in %q: %w", dbDir, err)
	}

	var entries []LegacyEntry
	for _, dbFile := range dbFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		db, err := loadLegacyFile(dbFile)
		if err != nil {
			return nil, err
		}
		for host, hostEntries := range db.Entries {
			for i := range hostEntries {
				if strings.TrimSpace(hostEntries[i].Source) == "" {
					hostEntries[i].Source = host
				}
			}
			entries = append(entries, hostEntries...)
		}
	}

	sortLegacyEntries(entries)
	return entries, nil
}

// LoadLegacyHost reads db.<hostname>.json from dbDir.
// A missing file returns an empty host section without error.
func LoadLegacyHost(ctx context.Context, dbDir, hostname string) (LegacyDB, error) {
	if err := ctx.Err(); err != nil {
		return LegacyDB{}, err
	}
	host, err := normalizeLegacyHostname(hostname)
	if err != nil {
		return LegacyDB{}, err
	}
	if strings.TrimSpace(dbDir) == "" {
		return LegacyDB{}, errors.New("db directory must not be empty")
	}

	dbFile := filepath.Join(dbDir, legacyDBFileName(host))
	db, err := loadLegacyFile(dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			return newLegacyHostDB(host), nil
		}
		return LegacyDB{}, err
	}

	if _, ok := db.Entries[host]; !ok {
		db.Entries[host] = []LegacyEntry{}
	}
	for i := range db.Entries[host] {
		if strings.TrimSpace(db.Entries[host][i].Source) == "" {
			db.Entries[host][i].Source = host
		}
	}
	sortLegacyEntries(db.Entries[host])
	return db, nil
}

// SaveLegacyHost writes db.<hostname>.json in worktime.rb shape: 2-space indent
// matching JSON.pretty_generate, human derived from epoch, no trailing newline.
func SaveLegacyHost(ctx context.Context, dbDir, hostname string, db LegacyDB) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := normalizeLegacyHostname(hostname)
	if err != nil {
		return err
	}
	if strings.TrimSpace(dbDir) == "" {
		return errors.New("db directory must not be empty")
	}

	if db.Entries == nil {
		db.Entries = map[string][]LegacyEntry{}
	}
	if _, ok := db.Entries[host]; !ok {
		db.Entries[host] = []LegacyEntry{}
	}

	prepared := make([]LegacyEntry, len(db.Entries[host]))
	for i, entry := range db.Entries[host] {
		prepared[i] = prepareLegacyEntry(entry, host)
	}
	sortLegacyEntries(prepared)
	db.Entries[host] = prepared

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("encode database for host %q: %w", host, err)
	}

	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create db directory %q: %w", dbDir, err)
	}

	dbFile := filepath.Join(dbDir, legacyDBFileName(host))
	tmp, err := os.CreateTemp(dbDir, legacyDBFileName(host)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp db file for host %q: %w", host, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp db file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp db file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, dbFile); err != nil {
		return fmt.Errorf("install db file %q: %w", dbFile, err)
	}
	return nil
}

func prepareLegacyEntry(entry LegacyEntry, host string) LegacyEntry {
	if strings.TrimSpace(entry.Source) == "" {
		entry.Source = host
	}
	entry.Human = FormatLegacyHuman(entry.Epoch)
	if strings.EqualFold(strings.TrimSpace(entry.Action), worktime.ActionAdd) {
		entry.valueSet = true
	}
	return entry
}

func loadLegacyFile(dbFile string) (LegacyDB, error) {
	var db LegacyDB

	data, err := os.ReadFile(dbFile)
	if err != nil {
		return db, err
	}

	if err := json.Unmarshal(data, &db); err != nil {
		return db, fmt.Errorf("parse db file %q: %w", dbFile, err)
	}
	if db.Entries == nil {
		db.Entries = map[string][]LegacyEntry{}
	}
	return db, nil
}

func sortLegacyEntries(entries []LegacyEntry) {
	// Match worktime.rb: stable sort by epoch only (no secondary key).
	slices.SortStableFunc(entries, func(a, b LegacyEntry) int {
		switch {
		case a.Epoch < b.Epoch:
			return -1
		case a.Epoch > b.Epoch:
			return 1
		default:
			return 0
		}
	})
}

func normalizeLegacyHostname(hostname string) (string, error) {
	host := strings.TrimSpace(hostname)
	if host == "" {
		return "", errors.New("hostname must not be empty")
	}
	if strings.ContainsAny(host, "/\\") || strings.Contains(host, "..") {
		return "", fmt.Errorf("invalid hostname %q", host)
	}
	return host, nil
}

func legacyDBFileName(hostname string) string {
	return "db." + hostname + ".json"
}

func newLegacyHostDB(hostname string) LegacyDB {
	return LegacyDB{
		Entries: map[string][]LegacyEntry{
			hostname: {},
		},
	}
}

func writeJSONString(buf *bytes.Buffer, key, value string, leadingComma bool) error {
	if leadingComma {
		buf.WriteByte(',')
	}
	keyJSON, err := json.Marshal(key)
	if err != nil {
		return err
	}
	valJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	buf.Write(keyJSON)
	buf.WriteByte(':')
	buf.Write(valJSON)
	return nil
}
