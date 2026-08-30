package legacy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/worktime"
)

// ErrAlreadyMigrated indicates the JSONL store already has data for a host
// that a non-force migration would overwrite.
var ErrAlreadyMigrated = errors.New("already migrated")

// Finding kinds reported during migration. FindingUnpairedLogin,
// FindingZeroValueAdd and FindingNegativeValue describe rows that are
// imported as-is (the data is odd but still meaningful to a report).
// FindingUnknownAction is different: the row is quarantined — left out of
// the entries written to the store — because BuildReport has no case for
// it and would otherwise fail on "unknown action" only once someone runs
// `work report`, long after the poisoned row was migrated in (task 781).
const (
	FindingUnpairedLogin = "unpaired-login"
	FindingZeroValueAdd  = "zero-value-add"
	FindingNegativeValue = "negative-value"
	FindingUnknownAction = "unknown-action"
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

// MigrateHostFailure names one host whose migration attempt failed, and why.
// Migrate collects these instead of aborting the whole run on the first host
// error, so a mid-run failure (in practice: a genuine I/O error inside
// Store.ReplaceHost, since every "bad data" case is already preflighted
// before any host is written — see Migrate's doc comment) never leaves the
// caller unsure which hosts actually landed (task j81). This is distinct
// from the --force id-gap bug fixed by task 481 and the unknown-action
// quarantine added by task 781; those two already close off the failure
// modes that used to make retries or malformed rows dangerous.
type MigrateHostFailure struct {
	Host string
	Err  error
}

// Error satisfies the error interface so a MigrateHostFailure can be used
// directly as one of errors.Join's arguments in partialMigrateError.
func (f MigrateHostFailure) Error() string {
	return fmt.Sprintf("host %q: %v", f.Host, f.Err)
}

// Unwrap exposes the underlying store/id error so errors.Is/As can still
// reach it (e.g. a caller checking for a specific sentinel) through the
// joined error partialMigrateError returns.
func (f MigrateHostFailure) Unwrap() error { return f.Err }

// MigrateResult summarizes hosts written and findings reported.
//
// Hosts and Failed are both always populated on a partial-failure run, so a
// caller — including the CLI's report, via writeMigrateReport — can tell
// which hosts landed in the store and which didn't without having to parse
// the returned error string (task j81).
type MigrateResult struct {
	Hosts    []string
	Entries  int
	Findings []MigrateFinding
	Failed   []MigrateHostFailure
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

// Migrate imports every db.*.json under dbDir into storeDir as per-host JSONL.
//
// Host sections drive output names: db.archive.json is split by section key into
// db.mc-lon-mb8477.jsonl and db.galaxytabs6.jsonl (never db.archive.jsonl).
// A second run without Force returns ErrAlreadyMigrated and changes nothing.
// A Force run replaces a host's file outright, but ids are still numbered
// from that host's current watermark (not always from 1), so a prior delete
// or undo-elevated watermark never collides with the store's id-reuse guard.
//
// A row with an unrecognized action (e.g. a typo'd "bogus") is quarantined:
// it is reported as a FindingUnknownAction finding but never written to the
// store, so a poisoned legacy row can't reach BuildReport's "unknown action"
// failure by way of a clean-looking migrate (task 781).
//
// Real-data cases (see ~/git/worktime; covered synthetically in testdata/migrate):
//   - one unpaired/superseded earth login at epoch 1781618168 (4377 logins vs 4376 logouts)
//   - 11 add entries with value==0 across hosts
//   - 243 negative selfdevelopment entries (−829.96h), imported as signed values
//
// Migrate is preflighted, not transactional, across hosts (task j81): parsing
// and host-name validation (loadLegacyHostsGrouped) and the already-migrated
// refusal (refuseIfAlreadyMigrated) both run for every host before any host
// is written, and unknown-action rows are quarantined rather than failing
// (task 781) — so by the time the per-host loop below starts, the only way a
// given host can still fail is a genuine I/O error inside Store.ReplaceHost
// (temp-file create/write/fsync/rename). Making the per-host writes
// themselves one atomic multi-host transaction would mean re-implementing
// Store's temp+rename machinery in this package, which is out of scope here
// (internal/worktime/legacy stays a caller of internal/worktime's exported
// Store API, not a re-implementer of it). Instead, on such a failure Migrate
// keeps going through the remaining hosts — each host's file is independent,
// so one host's I/O error shouldn't hide whether the others succeeded — and
// reports every host's outcome: MigrateResult.Hosts lists what landed,
// MigrateResult.Failed lists what didn't and why, and the returned error (if
// any) summarizes both. A retry with --force is then safe to run over ALL
// hosts again, not just the failed ones: task 481's watermark-based
// numbering means re-migrating an already-succeeded host just rewrites it
// with fresh, non-colliding ids instead of erroring or duplicating data.
func Migrate(ctx context.Context, dbDir, storeDir string, opts MigrateOptions) (MigrateResult, error) {
	var result MigrateResult

	hosts, byHost, store, err := prepareMigrate(ctx, dbDir, storeDir, opts)
	if err != nil {
		return result, err
	}
	// Context cancellation aborts the whole run immediately (a caller-level
	// stop request, unlike a single host's I/O failure below); result then
	// reflects only whatever landed before the cancellation was noticed, and
	// no report is written for an aborted run.
	if err := runMigrateHosts(ctx, store, hosts, byHost, &result); err != nil {
		return result, err
	}

	// Write the report unconditionally, including on partial failure, so the
	// CLI's stdout output (MigrateOptions.Report) always shows exactly which
	// hosts succeeded and which failed — a bare returned error alone would
	// force the operator to parse an error string to find out.
	if err := writeMigrateReport(opts.Report, result); err != nil {
		return result, err
	}
	if len(result.Failed) > 0 {
		return result, partialMigrateError(result)
	}
	return result, nil
}

// prepareMigrate is Migrate's preflight stage (task j81): it validates
// dbDir/storeDir, loads and groups every legacy host section, opens the
// JSONL store, and — unless Force is set — refuses to proceed at all if any
// host is already migrated. Everything here runs across all hosts before a
// single one is written, so any error return here means zero hosts were
// touched; only a genuine per-host I/O error in runMigrateHosts's loop can
// happen after this point.
func prepareMigrate(ctx context.Context, dbDir, storeDir string, opts MigrateOptions) ([]string, map[string][]LegacyEntry, *worktime.Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	dbDir = strings.TrimSpace(dbDir)
	storeDir = strings.TrimSpace(storeDir)
	if dbDir == "" {
		return nil, nil, nil, errors.New("db directory must not be empty")
	}
	if storeDir == "" {
		return nil, nil, nil, errors.New("store directory must not be empty")
	}

	byHost, err := loadLegacyHostsGrouped(ctx, dbDir)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(byHost) == 0 {
		return nil, nil, nil, fmt.Errorf("no legacy host sections found in %q", dbDir)
	}

	store, err := worktime.Open(ctx, storeDir)
	if err != nil {
		return nil, nil, nil, err
	}

	hosts := sortedHostKeys(byHost)
	if !opts.Force {
		if err := refuseIfAlreadyMigrated(store, hosts); err != nil {
			return nil, nil, nil, err
		}
	}
	return hosts, byHost, store, nil
}

// runMigrateHosts attempts every host in hosts in turn, folding each
// outcome into result: a success appends to result.Hosts/Entries/Findings, a
// failure appends to result.Failed and moves on to the next host rather than
// aborting the run (see Migrate's doc comment for why that's safe: by this
// point every preflightable failure has already been rejected or
// quarantined, so a per-host failure here is a genuine I/O error, and the
// remaining hosts' files are independent of it). The one thing that does
// abort immediately is context cancellation, returned as an error so Migrate
// can skip writing a report for a run that never finished.
func runMigrateHosts(ctx context.Context, store *worktime.Store, hosts []string, byHost map[string][]LegacyEntry, result *MigrateResult) error {
	for _, host := range hosts {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, findings, err := migrateOneHost(ctx, store, host, byHost[host])
		if err != nil {
			result.Failed = append(result.Failed, MigrateHostFailure{Host: host, Err: err})
			continue
		}
		result.Hosts = append(result.Hosts, host)
		result.Entries += len(entries)
		result.Findings = append(result.Findings, findings...)
	}
	return nil
}

// partialMigrateError summarizes a partial-failure Migrate run: how many
// hosts failed out of how many total, which hosts already succeeded, and how
// to retry. errors.Join keeps each host's underlying error individually
// reachable via errors.Is/As on the returned error, while still satisfying
// Migrate's single-error return signature.
func partialMigrateError(result MigrateResult) error {
	errs := make([]error, len(result.Failed))
	for i, f := range result.Failed {
		errs[i] = f
	}
	succeeded := "none"
	if len(result.Hosts) > 0 {
		succeeded = strings.Join(result.Hosts, ", ")
	}
	return fmt.Errorf(
		"migrate: %d of %d host(s) failed (succeeded: %s); retry with --force to safely re-attempt all hosts"+
			" -- already-succeeded hosts are rewritten with fresh, non-colliding ids, not duplicated (task 481): %w",
		len(result.Failed), len(result.Failed)+len(result.Hosts), succeeded, errors.Join(errs...))
}

// migrateOneHost converts one host's legacy entries and writes them to store,
// replacing whatever that host previously held.
//
// Entries are numbered starting at the host's current id watermark (via
// NextID) rather than always at 1. A fresh host has no watermark yet, so
// NextID returns 1 and behavior is unchanged from a first-time migration. On
// a Force re-migrate of a host that has since had entries deleted (or whose
// watermark was raised by an undo restore), starting at 1 again would
// collide with Store.ReplaceHost's id-reuse guard, which rejects any id
// below the watermark that is not currently present (see internal/worktime's
// store.go). Starting at the watermark instead keeps ids globally
// non-reused across the host's lifetime, matching the store's "ids never
// reused" invariant, without needing the allow-restore bypass undo restore
// uses internally for a different purpose (reinstating one specific
// historical id/entry pair).
func migrateOneHost(ctx context.Context, store *worktime.Store, host string, legacyEntries []LegacyEntry) ([]worktime.Entry, []MigrateFinding, error) {
	startID, err := store.NextID(host)
	if err != nil {
		return nil, nil, fmt.Errorf("next id for host %q: %w", host, err)
	}
	entries, quarantined := convertLegacyHost(host, legacyEntries, startID)
	findings := collectMigrateFindings(host, legacyEntries, entries)
	findings = append(quarantined, findings...)

	if err := store.ReplaceHost(ctx, host, entries); err != nil {
		return nil, nil, fmt.Errorf("write host %q: %w", host, err)
	}
	return entries, findings, nil
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
			if _, err := worktime.NormalizeHost(host); err != nil {
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

// refuseIfAlreadyMigrated checks store's on-disk layout (via the exported
// Store.HostFileExists) for a db.<host>.jsonl belonging to any of hosts, and
// fails on the first one found. Checking store directly rather than
// re-deriving the JSONL file-naming convention here keeps that convention
// package-private to internal/worktime, per task e81's "narrow exported
// surface" requirement.
func refuseIfAlreadyMigrated(store *worktime.Store, hosts []string) error {
	for _, host := range hosts {
		exists, fileName, err := store.HostFileExists(host)
		if err != nil {
			return fmt.Errorf("stat store file for host %q: %w", host, err)
		}
		if exists {
			return fmt.Errorf("%w: host %q (%s)", ErrAlreadyMigrated, host, fileName)
		}
	}
	return nil
}

// convertLegacyHost assigns each legacy entry a fresh, monotonically
// increasing id starting at startID (the host's current watermark from the
// store, or 1 for a host that has never been written). Numbering from the
// watermark instead of always from 1 keeps a Force re-migrate compatible
// with Store.ReplaceHost's id-reuse guard even after prior deletes have left
// gaps below the watermark.
//
// A row whose action worktime.IsValidAction rejects (e.g. a typo like
// "bogus") is quarantined instead of converted: it is left out of the
// returned entries and reported as a FindingUnknownAction finding instead.
// Previously such rows were imported unchecked and only surfaced later as a
// hard failure from BuildReport ("unknown action"), by which point the store
// already held the bad row and the only fix was manual JSONL surgery (task
// 781). Ids are only consumed by rows that are actually kept, so
// quarantining a row never leaves a gap that later confuses the id-reuse
// guard.
func convertLegacyHost(host string, legacyEntries []LegacyEntry, startID int64) ([]worktime.Entry, []MigrateFinding) {
	out := make([]worktime.Entry, 0, len(legacyEntries))
	var quarantined []MigrateFinding
	nextID := startID

	for _, leg := range legacyEntries {
		action := strings.ToLower(strings.TrimSpace(leg.Action))
		if !worktime.IsValidAction(action) {
			quarantined = append(quarantined, MigrateFinding{
				Kind:   FindingUnknownAction,
				Host:   host,
				Epoch:  leg.Epoch,
				Action: leg.Action,
				Tag:    strings.TrimSpace(leg.What),
				Detail: fmt.Sprintf("quarantined: unknown action %q not imported", leg.Action),
			})
			continue
		}
		out = append(out, legacyToEntry(nextID, host, leg))
		nextID++
	}
	return out, quarantined
}

// legacyToEntry converts one legacy row into a store Entry. The caller
// (convertLegacyHost) must have already confirmed the action is valid;
// legacyToEntry itself no longer re-checks it since that check now decides
// whether the row is converted at all.
func legacyToEntry(id int64, host string, leg LegacyEntry) worktime.Entry {
	entry := worktime.Entry{
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
	if leg.HasValue() || entry.Action == worktime.ActionAdd {
		entry.Value = leg.Value
	}
	return entry
}

func collectMigrateFindings(host string, legacyEntries []LegacyEntry, entries []worktime.Entry) []MigrateFinding {
	var findings []MigrateFinding

	for _, leg := range legacyEntries {
		action := strings.ToLower(strings.TrimSpace(leg.Action))
		tag := strings.TrimSpace(leg.What)
		if action == worktime.ActionAdd && leg.HasValue() && leg.Value == 0 {
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
		if action == worktime.ActionAdd && leg.Value < 0 {
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

func findUnpairedLogins(host string, entries []worktime.Entry) []MigrateFinding {
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
		case worktime.ActionLogin:
			if prev, ok := active[tag]; ok {
				findings = append(findings, MigrateFinding{
					Kind:   FindingUnpairedLogin,
					Host:   host,
					Epoch:  prev.epoch,
					Action: worktime.ActionLogin,
					Tag:    prev.tag,
					Detail: fmt.Sprintf("superseded by login at epoch %d (imported, not dropped)", entry.Epoch),
				})
			}
			active[tag] = openLogin{epoch: entry.Epoch, tag: tag}
		case worktime.ActionLogout:
			if _, ok := active[tag]; ok {
				delete(active, tag)
				continue
			}
			findings = append(findings, MigrateFinding{
				Kind:   FindingUnpairedLogin,
				Host:   host,
				Epoch:  entry.Epoch,
				Action: worktime.ActionLogout,
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
			Action: worktime.ActionLogin,
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
	if err := writeMigrateHostLists(w, result); err != nil {
		return err
	}
	for _, f := range result.Findings {
		if _, err := fmt.Fprintln(w, f.String()); err != nil {
			return fmt.Errorf("write migrate finding: %w", err)
		}
	}
	return nil
}

// writeMigrateHostLists prints the succeeded/failed host breakdown so a
// partial-failure run is legible straight from the CLI's stdout report
// without the reader having to parse the returned error string (task j81).
// The succeeded line is only printed when non-empty so a routine full
// success keeps its familiar single-summary-line report unchanged.
func writeMigrateHostLists(w io.Writer, result MigrateResult) error {
	if len(result.Hosts) > 0 {
		if _, err := fmt.Fprintf(w, "  succeeded: %s\n", strings.Join(result.Hosts, ", ")); err != nil {
			return fmt.Errorf("write migrate report: %w", err)
		}
	}
	if len(result.Failed) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "  failed: %d host(s) -- retry with --force to safely re-attempt all hosts\n",
		len(result.Failed)); err != nil {
		return fmt.Errorf("write migrate report: %w", err)
	}
	for _, f := range result.Failed {
		if _, err := fmt.Fprintf(w, "    - %s: %v\n", f.Host, f.Err); err != nil {
			return fmt.Errorf("write migrate report: %w", err)
		}
	}
	return nil
}
