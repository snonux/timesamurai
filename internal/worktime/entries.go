package worktime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/config"
)

// bufferSourceTag is the buffer category UseBuffer withdraws from. The CLI
// plan's `work usebuffer <duration>` takes no tag argument, so this mirrors
// the pre-rewrite tool's hardcoded selfdevelopment-to-work transfer instead
// of asking which of several buffer_for categories to draw down.
const bufferSourceTag = "selfdevelopment"

var (
	// ErrAlreadyLoggedIn indicates a category already has an open login
	// somewhere in the store (login/logout is a single cross-host state
	// machine, not per-host).
	ErrAlreadyLoggedIn = errors.New("already logged in")
	// ErrNotLoggedIn indicates a category has no open login to close.
	ErrNotLoggedIn = errors.New("not logged in")
	// ErrEntryNotFound indicates an address did not resolve to a stored entry.
	ErrEntryNotFound = errors.New("entry not found")
)

// EntryPatch carries only the fields a Modify call changes; nil fields keep
// the entry's current value. Host and ID are deliberately not patchable:
// <host>:<id> addressing must keep pointing at the same entry after a
// modify, or pasted addresses and undo history would silently go stale.
type EntryPatch struct {
	// Action replaces the entry's action (login/logout/add); validated by
	// ValidateEntry after the patch is applied.
	Action *string
	// Epoch replaces the entry's Unix timestamp.
	Epoch *int64
	// Value replaces the entry's signed duration in seconds.
	Value *int64
	// Tags replaces the entry's tags, going through the same
	// default-to-WorkTag normalization as a new entry (defaultTags), so
	// patching in an empty slice does not leave the entry tag-less.
	Tags *[]string
	// Descr replaces the entry's free-text description.
	Descr *string
}

// Start opens a login session for tags (default WorkTag when tags is empty)
// on host at "at" (time.Now() when zero). It fails if that category already
// has an open login anywhere in the store, since a person cannot be doing
// the same thing on two machines' clocks at once.
func Start(ctx context.Context, store *Store, cfg config.AccountingConfig, host string, tags []string, at time.Time, descr string) (Entry, error) {
	category, err := sessionKey(cfg, tags)
	if err != nil {
		return Entry{}, err
	}

	openHost, open, err := openSessionHost(store, cfg, category)
	if err != nil {
		return Entry{}, err
	}
	if open {
		return Entry{}, fmt.Errorf("%w: %q already open on host %q", ErrAlreadyLoggedIn, category, openHost)
	}

	return insertEntry(ctx, store, cfg, host, actionLogin, tags, epochOf(at), 0, descr)
}

// Stop closes the open login session for tags (default WorkTag) by writing
// a logout entry on host at "at". The logout is recorded on host regardless
// of which host holds the open login: it is just a timestamp marker, not a
// reference to the login entry it closes.
func Stop(ctx context.Context, store *Store, cfg config.AccountingConfig, host string, tags []string, at time.Time, descr string) (Entry, error) {
	category, err := sessionKey(cfg, tags)
	if err != nil {
		return Entry{}, err
	}

	_, open, err := openSessionHost(store, cfg, category)
	if err != nil {
		return Entry{}, err
	}
	if !open {
		return Entry{}, fmt.Errorf("%w: %q", ErrNotLoggedIn, category)
	}

	return insertEntry(ctx, store, cfg, host, actionLogout, tags, epochOf(at), 0, descr)
}

// Add records a positive duration against tags (default WorkTag) on host.
func Add(ctx context.Context, store *Store, cfg config.AccountingConfig, host string, tags []string, duration time.Duration, at time.Time, descr string) (Entry, error) {
	if duration <= 0 {
		return Entry{}, errors.New("duration must be positive")
	}
	return insertEntry(ctx, store, cfg, host, actionAdd, tags, epochOf(at), durationToSeconds(duration), descr)
}

// Sub records a negative duration (a withdrawal) against tags on host.
func Sub(ctx context.Context, store *Store, cfg config.AccountingConfig, host string, tags []string, duration time.Duration, at time.Time, descr string) (Entry, error) {
	if duration <= 0 {
		return Entry{}, errors.New("duration must be positive")
	}
	return insertEntry(ctx, store, cfg, host, actionAdd, tags, epochOf(at), -durationToSeconds(duration), descr)
}

// UseBuffer withdraws duration from bufferSourceTag and credits it to
// WorkTag as two separate add entries.
//
// Not atomic: if the credit fails after the withdrawal already committed,
// the withdrawal stays on disk (matching the pre-rewrite tool's semantics —
// there is no cross-file transaction over two independent JSONL appends).
// The returned error says so explicitly so a caller can tell the difference
// between "nothing happened" and "half of it happened".
func UseBuffer(ctx context.Context, store *Store, cfg config.AccountingConfig, host string, duration time.Duration, at time.Time, descr string) ([]Entry, error) {
	if duration <= 0 {
		return nil, errors.New("duration must be positive")
	}

	removed, err := Sub(ctx, store, cfg, host, []string{bufferSourceTag}, duration, at, descr)
	if err != nil {
		return nil, err
	}

	added, err := Add(ctx, store, cfg, host, []string{WorkTag}, duration, at, descr)
	if err != nil {
		return []Entry{removed}, fmt.Errorf("withdrew %s but crediting %s failed: %w", bufferSourceTag, WorkTag, err)
	}
	return []Entry{removed, added}, nil
}

// Modify applies patch to the entry at addr (resolved against currentHost)
// and records a modify undo record carrying the before/after snapshots.
func Modify(ctx context.Context, store *Store, cfg config.AccountingConfig, addr, currentHost string, patch EntryPatch) (Entry, error) {
	host, id, err := ParseAddress(addr, currentHost)
	if err != nil {
		return Entry{}, err
	}

	before, err := findEntry(store, host, id)
	if err != nil {
		return Entry{}, err
	}
	after := patch.apply(before)
	if err := ValidateEntry(cfg, after); err != nil {
		return Entry{}, err
	}

	if err := replaceOne(ctx, store, host, id, &after); err != nil {
		return Entry{}, err
	}
	if err := store.AppendUndo(ctx, host, UndoRecord{Op: OpModify, ID: id, Before: &before, After: &after}); err != nil {
		return after, fmt.Errorf("entry %s modified but undo record failed: %w", addr, err)
	}
	return after, nil
}

// Delete removes the entry at addr (resolved against currentHost) and
// records a delete undo record so it can be restored later via UndoLast.
func Delete(ctx context.Context, store *Store, addr, currentHost string) (Entry, error) {
	host, id, err := ParseAddress(addr, currentHost)
	if err != nil {
		return Entry{}, err
	}

	before, err := findEntry(store, host, id)
	if err != nil {
		return Entry{}, err
	}

	if err := replaceOne(ctx, store, host, id, nil); err != nil {
		return Entry{}, err
	}
	if err := store.AppendUndo(ctx, host, UndoRecord{Op: OpDelete, ID: id, Before: &before, After: nil}); err != nil {
		return before, fmt.Errorf("entry %s deleted but undo record failed: %w", addr, err)
	}
	return before, nil
}

// ParseAddress splits addr into a host and id. addr is either "<host>:<id>"
// (e.g. "earth:412") or a bare id, in which case currentHost fills in the
// host — so any address a future list/search command prints can be pasted
// straight into Modify or Delete, while a same-host reference only needs
// the bare id.
func ParseAddress(addr, currentHost string) (string, int64, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, errors.New("entry address must not be empty")
	}

	host := currentHost
	idPart := addr
	if h, rest, found := strings.Cut(addr, ":"); found {
		host = h
		idPart = rest
	}

	host, err := normalizeHost(host)
	if err != nil {
		return "", 0, fmt.Errorf("address %q: %w", addr, err)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(idPart), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, fmt.Errorf("address %q: invalid entry id", addr)
	}
	return host, id, nil
}

func (p EntryPatch) apply(entry Entry) Entry {
	if p.Action != nil {
		entry.Action = strings.ToLower(strings.TrimSpace(*p.Action))
	}
	if p.Epoch != nil {
		entry.Epoch = *p.Epoch
	}
	if p.Value != nil {
		entry.Value = *p.Value
	}
	if p.Tags != nil {
		entry.Tags = defaultTags(*p.Tags)
	}
	if p.Descr != nil {
		entry.Descr = strings.TrimSpace(*p.Descr)
	}
	return entry
}

// insertEntry peeks the next id for host, builds and validates an entry,
// appends it, and records the insertion in the undo log.
//
// Non-obvious: this deliberately calls NextID (peek), not AllocID (consume).
// Store.Append is the only place that actually advances a host's id
// watermark, on a successful write; AllocID advances it immediately, and
// handing that id to a later Append would make Append see it as already
// used (its reuse check compares against the *current* watermark). Peeking
// also means a validation failure below never burns an id — the watermark
// only moves once an entry is durably on disk. A benign race remains if two
// goroutines peek the same id concurrently: Append's own reuse check (under
// its lock) lets exactly one of them through and rejects the other, which
// is an acceptable outcome for what is a single-user local CLI tool, not a
// multi-writer service.
//
// The undo record is written only after the append that puts the entry on
// disk succeeds. If AppendUndo itself then fails, the entry is already
// durably written but has no undo coverage; the error says so explicitly
// rather than trying to roll back the append, since undoing a second,
// independent file write on top of a first failure risks leaving worse
// partial state than it fixes.
func insertEntry(ctx context.Context, store *Store, cfg config.AccountingConfig, host, action string, tags []string, epoch, value int64, descr string) (Entry, error) {
	id, err := store.NextID(host)
	if err != nil {
		return Entry{}, err
	}

	entry := Entry{
		ID:     id,
		Action: action,
		Epoch:  epoch,
		Host:   host,
		Value:  value,
		Tags:   defaultTags(tags),
		Descr:  strings.TrimSpace(descr),
	}
	if err := ValidateEntry(cfg, entry); err != nil {
		return Entry{}, err
	}

	if err := store.Append(ctx, entry); err != nil {
		return Entry{}, err
	}
	if err := store.AppendUndo(ctx, host, UndoRecord{Op: OpInsert, ID: entry.ID, After: &entry}); err != nil {
		return entry, fmt.Errorf("entry %d written but undo record failed: %w", entry.ID, err)
	}
	return entry, nil
}

// findEntry looks up id within host's entries.
func findEntry(store *Store, host string, id int64) (Entry, error) {
	for _, e := range store.Entries(host) {
		if e.ID == id {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("%w: %s:%d", ErrEntryNotFound, host, id)
}

// replaceOne rewrites host's file with the entry at id either replaced by
// *replacement, or removed when replacement is nil. ReplaceHost needs the
// whole host slice since modify/delete are rewrites, not appends.
func replaceOne(ctx context.Context, store *Store, host string, id int64, replacement *Entry) error {
	current := store.Entries(host)
	next := make([]Entry, 0, len(current))
	found := false
	for _, e := range current {
		if e.ID != id {
			next = append(next, e)
			continue
		}
		found = true
		if replacement != nil {
			next = append(next, *replacement)
		}
	}
	if !found {
		return fmt.Errorf("%w: %s:%d", ErrEntryNotFound, host, id)
	}
	return store.ReplaceHost(ctx, host, next)
}

// sessionKey maps tags to the single category that keys login/logout state,
// reusing AccountingTag's notion of "the one tag that matters" so a login
// tagged ["work","offsite"] and one tagged only ["work"] are the same
// session. A tag set with no work/plus/minus/buffer tag (only labels, or
// none at all) falls back to WorkTag, matching the historical default of an
// unset category meaning "work".
func sessionKey(cfg config.AccountingConfig, tags []string) (string, error) {
	tag, err := AccountingTag(cfg, tags)
	if err != nil {
		return "", err
	}
	if tag == "" {
		return WorkTag, nil
	}
	return tag, nil
}

// openSessionHost reports whether category has an unmatched login anywhere
// in the store, and on which host, by replaying every host's entries in
// global epoch order. Entries whose tags don't resolve to a session key
// (malformed historical data) are skipped rather than aborting the whole
// check, since they simply don't participate in this category's state.
func openSessionHost(store *Store, cfg config.AccountingConfig, category string) (string, bool, error) {
	openHost := ""
	for _, e := range allEntriesSorted(store) {
		key, err := sessionKey(cfg, e.Tags)
		if err != nil || key != category {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(e.Action)) {
		case actionLogin:
			openHost = e.Host
		case actionLogout:
			openHost = ""
		}
	}
	return openHost, openHost != "", nil
}

// allEntriesSorted merges every host's entries into one globally
// epoch-ordered slice, so login/logout state can be replayed across hosts
// rather than per host.
func allEntriesSorted(store *Store) []Entry {
	var all []Entry
	for _, host := range store.Hosts() {
		all = append(all, store.Entries(host)...)
	}
	sortEntriesByEpoch(all)
	return all
}

// defaultTags trims each tag and defaults to []string{WorkTag} when tags is
// empty, so every entry insertEntry builds carries at least one tag.
func defaultTags(tags []string) []string {
	if len(tags) == 0 {
		return []string{WorkTag}
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = strings.TrimSpace(t)
	}
	return out
}

func durationToSeconds(d time.Duration) int64 {
	return int64(d / time.Second)
}

func epochOf(at time.Time) int64 {
	if at.IsZero() {
		return time.Now().Unix()
	}
	return at.Unix()
}
