package worktime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrAlreadyMigrated indicates the JSONL store already has data for a host
// that a non-force migration would overwrite.
var ErrAlreadyMigrated = errors.New("already migrated")

// Finding kinds reported during migration (imported, never silently dropped).
const (
	FindingUnpairedLogin = "unpaired-login"
	FindingZeroValueAdd  = "zero-value-add"
	FindingNegativeValue = "negative-value"
)

// MigrateOptions configures a one-shot legacy JSON → JSONL import.
type MigrateOptions struct {
	// Force rewrites hosts that already have db.<host>.jsonl in the store.
	Force bool
	// Report receives human-readable findings when non-nil.
	Report io.Writer
}

// MigrateFinding is one edge-case note from a migration run.
type MigrateFinding struct {
	Kind   string
	Host   string
	Epoch  int64
	Action string
	Tag    string
	Value  int64
	Detail string
}

// String returns a single-line description of the finding.
func (f MigrateFinding) String() string {
	when := time.Unix(f.Epoch, 0).UTC().Format(time.RFC3339)
	base := fmt.Sprintf("[%s] host=%s epoch=%d (%s)", f.Kind, f.Host, f.Epoch, when)
	if f.Action != "" {
		base += " action=" + f.Action
	}
	if f.Tag != "" {
		base += " tag=" + f.Tag
	}
	if f.Kind == FindingZeroValueAdd || f.Kind == FindingNegativeValue || f.Value != 0 {
		base += fmt.Sprintf(" value=%d", f.Value)
	}
	if f.Detail != "" {
		base += ": " + f.Detail
	}
	return base
}

// MigrateResult summarizes hosts written and findings reported.
type MigrateResult struct {
	Hosts    []string
	Entries  int
	Findings []MigrateFinding
}

// Migrate imports every db.*.json under dbDir into storeDir as per-host JSONL.
//
// Host sections drive output names: db.archive.json is split by section key into
// db.mc-lon-mb8477.jsonl and db.galaxytabs6.jsonl (never db.archive.jsonl).
// A second run without Force returns ErrAlreadyMigrated and changes nothing.
//
// Real-data cases (see ~/git/worktime; covered synthetically in testdata/migrate):
//   - one unpaired/superseded earth login at epoch 1781618168 (4377 logins vs 4376 logouts)
//   - 11 add entries with value==0 across hosts
//   - 243 negative selfdevelopment entries (−829.96h), imported as signed values
func Migrate(ctx context.Context, dbDir, storeDir string, opts MigrateOptions) (MigrateResult, error) {
	var result MigrateResult

	if err := ctx.Err(); err != nil {
		return result, err
	}
	dbDir = strings.TrimSpace(dbDir)
	storeDir = strings.TrimSpace(storeDir)
	if dbDir == "" {
		return result, errors.New("db directory must not be empty")
	}
	if storeDir == "" {
		return result, errors.New("store directory must not be empty")
	}

	byHost, err := loadLegacyHostsGrouped(ctx, dbDir)
	if err != nil {
		return result, err
	}
	if len(byHost) == 0 {
		return result, fmt.Errorf("no legacy host sections found in %q", dbDir)
	}

	store, err := Open(ctx, storeDir)
	if err != nil {
		return result, err
	}

	hosts := sortedHostKeys(byHost)
	if !opts.Force {
		if err := refuseIfAlreadyMigrated(storeDir, hosts); err != nil {
			return result, err
		}
	}

	for _, host := range hosts {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		legacyEntries := byHost[host]
		entries := convertLegacyHost(host, legacyEntries)
		findings := collectMigrateFindings(host, legacyEntries, entries)
		result.Findings = append(result.Findings, findings...)

		if err := store.ReplaceHost(ctx, host, entries); err != nil {
			return result, fmt.Errorf("write host %q: %w", host, err)
		}
		result.Hosts = append(result.Hosts, host)
		result.Entries += len(entries)
	}

	if err := writeMigrateReport(opts.Report, result); err != nil {
		return result, err
	}
	return result, nil
}

func loadLegacyHostsGrouped(ctx context.Context, dbDir string) (map[string][]LegacyEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dbFiles, err := filepath.Glob(filepath.Join(dbDir, legacyDBFilePattern))
	if err != nil {
		return nil, fmt.Errorf("glob databases in %q: %w", dbDir, err)
	}
	if len(dbFiles) == 0 {
		return nil, fmt.Errorf("no legacy databases matching %s in %q", legacyDBFilePattern, dbDir)
	}

	byHost := make(map[string][]LegacyEntry)
	for _, dbFile := range dbFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		db, err := loadLegacyFile(dbFile)
		if err != nil {
			return nil, err
		}
		for host, hostEntries := range db.Entries {
			host = strings.TrimSpace(host)
			if host == "" {
				return nil, fmt.Errorf("empty host section in %q", dbFile)
			}
			if _, err := normalizeHost(host); err != nil {
				return nil, fmt.Errorf("invalid host section %q in %q: %w", host, dbFile, err)
			}
			// The host comes from the section key, not leg.Source, so no
			// backfill is needed here (unlike LoadLegacyAll/LoadLegacyHost,
			// which merge multiple hosts into one flat list and need it).
			byHost[host] = append(byHost[host], hostEntries...)
		}
	}

	for host, entries := range byHost {
		sortLegacyEntries(entries)
		byHost[host] = entries
	}
	return byHost, nil
}

func refuseIfAlreadyMigrated(storeDir string, hosts []string) error {
	for _, host := range hosts {
		path := filepath.Join(storeDir, dbFileName(host))
		_, err := os.Stat(path)
		if err == nil {
			return fmt.Errorf("%w: host %q (%s)", ErrAlreadyMigrated, host, dbFileName(host))
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat store file for host %q: %w", host, err)
		}
	}
	return nil
}

func convertLegacyHost(host string, legacyEntries []LegacyEntry) []Entry {
	out := make([]Entry, 0, len(legacyEntries))
	for i, leg := range legacyEntries {
		out = append(out, legacyToEntry(int64(i+1), host, leg))
	}
	return out
}

func legacyToEntry(id int64, host string, leg LegacyEntry) Entry {
	entry := Entry{
		ID:     id,
		Action: strings.ToLower(strings.TrimSpace(leg.Action)),
		Epoch:  leg.Epoch,
		Host:   host,
		Descr:  leg.Descr,
	}
	what := strings.TrimSpace(leg.What)
	if what != "" {
		entry.Tags = []string{what}
	}
	if leg.HasValue() || entry.Action == actionAdd {
		entry.Value = leg.Value
	}
	return entry
}

func collectMigrateFindings(host string, legacyEntries []LegacyEntry, entries []Entry) []MigrateFinding {
	var findings []MigrateFinding

	for _, leg := range legacyEntries {
		action := strings.ToLower(strings.TrimSpace(leg.Action))
		tag := strings.TrimSpace(leg.What)
		if action == actionAdd && leg.HasValue() && leg.Value == 0 {
			findings = append(findings, MigrateFinding{
				Kind:   FindingZeroValueAdd,
				Host:   host,
				Epoch:  leg.Epoch,
				Action: action,
				Tag:    tag,
				Value:  0,
				Detail: "imported add with value==0",
			})
		}
		if action == actionAdd && leg.Value < 0 {
			findings = append(findings, MigrateFinding{
				Kind:   FindingNegativeValue,
				Host:   host,
				Epoch:  leg.Epoch,
				Action: action,
				Tag:    tag,
				Value:  leg.Value,
				Detail: "imported negative duration as signed value",
			})
		}
	}

	findings = append(findings, findUnpairedLogins(host, entries)...)
	return findings
}

func findUnpairedLogins(host string, entries []Entry) []MigrateFinding {
	type openLogin struct {
		epoch int64
		tag   string
	}
	active := map[string]openLogin{}
	var findings []MigrateFinding

	for _, entry := range entries {
		tag := primaryTag(entry.Tags)
		action := strings.ToLower(strings.TrimSpace(entry.Action))
		switch action {
		case actionLogin:
			if prev, ok := active[tag]; ok {
				findings = append(findings, MigrateFinding{
					Kind:   FindingUnpairedLogin,
					Host:   host,
					Epoch:  prev.epoch,
					Action: actionLogin,
					Tag:    prev.tag,
					Detail: fmt.Sprintf("superseded by login at epoch %d (imported, not dropped)", entry.Epoch),
				})
			}
			active[tag] = openLogin{epoch: entry.Epoch, tag: tag}
		case actionLogout:
			if _, ok := active[tag]; ok {
				delete(active, tag)
				continue
			}
			findings = append(findings, MigrateFinding{
				Kind:   FindingUnpairedLogin,
				Host:   host,
				Epoch:  entry.Epoch,
				Action: actionLogout,
				Tag:    tag,
				Detail: "logout without a matching login (imported, not dropped)",
			})
		}
	}

	for _, open := range active {
		findings = append(findings, MigrateFinding{
			Kind:   FindingUnpairedLogin,
			Host:   host,
			Epoch:  open.epoch,
			Action: actionLogin,
			Tag:    open.tag,
			Detail: "login with no matching logout (imported, not dropped)",
		})
	}
	return findings
}

func primaryTag(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return tags[0]
}

func sortedHostKeys(byHost map[string][]LegacyEntry) []string {
	hosts := make([]string, 0, len(byHost))
	for host := range byHost {
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)
	return hosts
}

func writeMigrateReport(w io.Writer, result MigrateResult) error {
	if w == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "migrated %d host(s), %d entries, %d finding(s)\n",
		len(result.Hosts), result.Entries, len(result.Findings)); err != nil {
		return fmt.Errorf("write migrate report: %w", err)
	}
	for _, f := range result.Findings {
		if _, err := fmt.Fprintln(w, f.String()); err != nil {
			return fmt.Errorf("write migrate finding: %w", err)
		}
	}
	return nil
}
