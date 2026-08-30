package worktime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	dbFilePrefix   = "db."
	dbFileSuffix   = ".jsonl"
	undoFilePrefix = "undo."
	undoFileSuffix = ".jsonl"
	tmpFileSuffix  = ".tmp"
)

// Store is an in-memory view of per-host JSONL work-time files under a directory.
//
// Each host lives in db.<host>.jsonl (one Entry per LF-terminated line, sorted by
// epoch). Appends use a single O_APPEND write; modify/delete replace the file via
// temp + fsync + rename. Per-host ids are never reused: the next id is max(id)+1
// across both the entries file and undo.<host>.jsonl.
type Store struct {
	dir string

	mu     sync.Mutex
	byHost map[string][]Entry
	nextID map[string]int64
}

// Open loads every db.*.jsonl under dir into memory. Missing directories are created.
// Undo logs are consulted only for id allocation (max id), not loaded as entries.
func Open(ctx context.Context, dir string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("store directory must not be empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create store dir %q: %w", dir, err)
	}

	s := &Store{
		dir:    dir,
		byHost: make(map[string][]Entry),
		nextID: make(map[string]int64),
	}
	if err := s.loadAll(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Dir returns the store directory path.
func (s *Store) Dir() string {
	return s.dir
}

// Hosts returns host names currently loaded, sorted lexicographically.
func (s *Store) Hosts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	hosts := make([]string, 0, len(s.byHost))
	for host := range s.byHost {
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)
	return hosts
}

// Entries returns a defensive copy of the entries for host, sorted by epoch.
// Unknown hosts yield a nil slice.
//
// The copy is deep with respect to Tags: copy(out, entries) only duplicates
// the Entry structs, and Entry.Tags is a slice header that would otherwise
// still point at the store's backing array (100 Go Mistakes #25 — "not
// making a deep copy"). Without cloning each Tags slice, a caller mutating
// the Tags of a returned Entry would silently corrupt the in-memory store.
func (s *Store) Entries(host string) []Entry {
	host, err := NormalizeHost(host)
	if err != nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.byHost[host]
	if len(entries) == 0 {
		return nil
	}
	out := make([]Entry, len(entries))
	copy(out, entries)
	for i := range out {
		out[i].Tags = slices.Clone(out[i].Tags)
	}
	return out
}

// HostFileExists reports whether db.<host>.jsonl already exists on disk for
// host, independent of what this Store currently holds in memory, along with
// the file's base name (for callers that want to name it in a message).
//
// This exists for internal/worktime/legacy's migrate importer (task e81
// split that code out of this package): a non-Force migrate needs to refuse
// overwriting a host that was already migrated, which means checking the
// store's on-disk layout directly rather than trusting what Open happened to
// load into memory (a host with zero live entries but a still-present,
// now-empty db file must still count as "already migrated"). The file-naming
// convention itself (dbFileName) stays package-private; this method is the
// narrow, purpose-built way to ask the one question migrate actually needs
// answered without exposing that convention as its own API.
func (s *Store) HostFileExists(host string) (exists bool, fileName string, err error) {
	host, err = NormalizeHost(host)
	if err != nil {
		return false, "", err
	}
	fileName = dbFileName(host)

	_, statErr := os.Stat(filepath.Join(s.dir, fileName))
	if statErr == nil {
		return true, fileName, nil
	}
	if os.IsNotExist(statErr) {
		return false, fileName, nil
	}
	return false, fileName, fmt.Errorf("stat store file for host %q: %w", host, statErr)
}

// NextID returns the next unused id for host without consuming it.
func (s *Store) NextID(host string) (int64, error) {
	host, err := NormalizeHost(host)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.nextIDLocked(host), nil
}

// AllocID consumes and returns the next unused id for host.
func (s *Store) AllocID(host string) (int64, error) {
	host, err := NormalizeHost(host)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextIDLocked(host)
	s.nextID[host] = id + 1
	return id, nil
}

// Append adds entry to its host file with a single O_APPEND write when the new
// epoch keeps the file sorted; otherwise it rewrites the host file.
// Entry.ID must be positive and must not reuse a previously seen id for that host.
func (s *Store) Append(ctx context.Context, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	host, err := NormalizeHost(entry.Host)
	if err != nil {
		return err
	}
	entry.Host = host

	if entry.ID <= 0 {
		return errors.New("entry id must be positive")
	}

	// Clone Tags on ingest so the stored Entry owns its backing array.
	// Otherwise a caller mutating the slice it passed to Append after this
	// call returns would silently mutate the store's copy too (100 Go
	// Mistakes #25).
	entry.Tags = slices.Clone(entry.Tags)

	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID < s.nextIDLocked(host) || s.hasIDLocked(host, entry.ID) {
		return fmt.Errorf("entry id %d for host %q was already used", entry.ID, host)
	}

	cur := s.byHost[host]
	if len(cur) == 0 || entry.Epoch >= cur[len(cur)-1].Epoch {
		if err := s.appendLineLocked(ctx, host, entry); err != nil {
			return err
		}
		s.byHost[host] = append(cur, entry)
	} else {
		next := make([]Entry, len(cur)+1)
		copy(next, cur)
		next[len(cur)] = entry
		sortEntriesByEpoch(next)
		if err := s.rewriteHostLocked(ctx, host, next); err != nil {
			return err
		}
		s.byHost[host] = next
	}

	s.bumpNextIDLocked(host, entry.ID)
	return nil
}

// ReplaceHost rewrites db.<host>.jsonl with entries via temp + fsync + rename.
// Entries are sorted by epoch before writing. Ids already allocated stay reserved
// even when the replacement omits them. Deleted ids below the next-id watermark
// cannot be reintroduced here; undo restore calls replaceHostLocked directly with
// allowRestore set, bypassing this guard.
func (s *Store) ReplaceHost(ctx context.Context, host string, entries []Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := NormalizeHost(host)
	if err != nil {
		return err
	}

	normalized, err := normalizeHostEntries(host, entries)
	if err != nil {
		return err
	}
	sortEntriesByEpoch(normalized)

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.replaceHostLocked(ctx, host, normalized, false)
}

// replaceHostLocked rewrites the host file. When allowRestore is true, ids below
// the watermark that are absent from the current set may be reintroduced (undo
// of delete). Callers must hold s.mu and pass normalized, epoch-sorted entries.
func (s *Store) replaceHostLocked(ctx context.Context, host string, normalized []Entry, allowRestore bool) error {
	currentIDs := make(map[int64]struct{}, len(s.byHost[host]))
	for _, e := range s.byHost[host] {
		currentIDs[e.ID] = struct{}{}
	}
	watermark := s.nextIDLocked(host)

	seen := make(map[int64]struct{}, len(normalized))
	for _, e := range normalized {
		if _, ok := seen[e.ID]; ok {
			return fmt.Errorf("duplicate entry id %d for host %q", e.ID, host)
		}
		seen[e.ID] = struct{}{}
		if e.ID < watermark {
			if _, ok := currentIDs[e.ID]; !ok && !allowRestore {
				return fmt.Errorf("entry id %d for host %q was already used", e.ID, host)
			}
		}
	}

	if err := s.rewriteHostLocked(ctx, host, normalized); err != nil {
		return err
	}
	s.byHost[host] = normalized
	for _, e := range normalized {
		s.bumpNextIDLocked(host, e.ID)
	}
	return nil
}

func (s *Store) loadAll(ctx context.Context) error {
	pattern := filepath.Join(s.dir, dbFilePrefix+"*"+dbFileSuffix)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob store files in %q: %w", s.dir, err)
	}

	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		host, err := hostFromDBPath(path)
		if err != nil {
			return err
		}
		entries, maxID, err := readEntriesFile(path)
		if err != nil {
			return fmt.Errorf("load %q: %w", path, err)
		}
		undoMax, err := maxIDFromUndoFile(filepath.Join(s.dir, undoFileName(host)))
		if err != nil {
			return fmt.Errorf("scan undo for host %q: %w", host, err)
		}
		if undoMax > maxID {
			maxID = undoMax
		}
		sortEntriesByEpoch(entries)
		s.byHost[host] = entries
		if maxID > 0 {
			s.nextID[host] = maxID + 1
		}
	}

	// Hosts that only have an undo log still reserve those ids.
	undoPattern := filepath.Join(s.dir, undoFilePrefix+"*"+undoFileSuffix)
	undoFiles, err := filepath.Glob(undoPattern)
	if err != nil {
		return fmt.Errorf("glob undo files in %q: %w", s.dir, err)
	}
	for _, path := range undoFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		host, err := hostFromUndoPath(path)
		if err != nil {
			return err
		}
		if _, ok := s.byHost[host]; ok {
			continue
		}
		undoMax, err := maxIDFromUndoFile(path)
		if err != nil {
			return fmt.Errorf("scan undo for host %q: %w", host, err)
		}
		s.byHost[host] = nil
		if undoMax > 0 {
			s.nextID[host] = undoMax + 1
		}
	}
	return nil
}

func (s *Store) nextIDLocked(host string) int64 {
	if id, ok := s.nextID[host]; ok && id > 0 {
		return id
	}
	return 1
}

func (s *Store) bumpNextIDLocked(host string, id int64) {
	next := id + 1
	if cur, ok := s.nextID[host]; !ok || next > cur {
		s.nextID[host] = next
	}
}

func (s *Store) hasIDLocked(host string, id int64) bool {
	for _, e := range s.byHost[host] {
		if e.ID == id {
			return true
		}
	}
	return false
}

func (s *Store) appendLineLocked(ctx context.Context, host string, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	line, err := marshalEntryLine(entry)
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, dbFileName(host))
	return appendJSONLLine(path, line)
}

func (s *Store) rewriteHostLocked(ctx context.Context, host string, entries []Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(s.dir, dbFileName(host))
	return rewriteJSONLFile(path, entries)
}

// appendJSONLLine appends a single line to path and fsyncs it before
// returning, so a successful call is durable across a crash/power loss
// (100 Go Mistakes #54: not fsyncing after a write and swallowing Close
// errors can both silently lose the very data the caller thinks was
// saved). Mirrors rewriteJSONLFile's write -> Sync -> Close ordering,
// with every step's error surfaced instead of being discarded.
func appendJSONLLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %q for append: %w", path, err)
	}

	writeErr, closeErr := writeSyncClose(f, line)
	if writeErr != nil {
		return fmt.Errorf("append to %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %q after append: %w", path, closeErr)
	}
	return nil
}

// syncCloser is the narrow file surface writeSyncClose needs. *os.File
// satisfies it; tests substitute a fake to exercise the Sync/Close failure
// paths, which are impractical to trigger reliably on a real file.
type syncCloser interface {
	io.Writer
	Sync() error
	Close() error
}

// writeSyncClose writes line to f, then always Syncs and Closes it (Close
// runs even if Write/Sync failed, so the fd is never leaked), returning the
// write/Sync error and the Close error separately so the caller can report
// each failure mode with its own message.
func writeSyncClose(f syncCloser, line []byte) (writeErr, closeErr error) {
	n, err := f.Write(line)
	switch {
	case err != nil:
		writeErr = err
	case n != len(line):
		writeErr = fmt.Errorf("short write %d/%d", n, len(line))
	default:
		writeErr = f.Sync()
	}
	closeErr = f.Close()
	return writeErr, closeErr
}

func rewriteJSONLFile(path string, entries []Entry) error {
	tmp := path + tmpFileSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create temp %q: %w", tmp, err)
	}

	writeErr := writeEntries(f, entries)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write temp %q: %w", tmp, writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close temp %q: %w", tmp, closeErr)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %q -> %q: %w", tmp, path, err)
	}
	return nil
}

func writeEntries(w io.Writer, entries []Entry) error {
	for i := range entries {
		line, err := marshalEntryLine(entries[i])
		if err != nil {
			return err
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
	}
	return nil
}

func marshalEntryLine(entry Entry) ([]byte, error) {
	payload, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshal entry id %d: %w", entry.ID, err)
	}
	if bytes.Contains(payload, []byte{'\n'}) {
		return nil, fmt.Errorf("entry id %d marshaled with embedded newline", entry.ID)
	}
	return append(payload, '\n'), nil
}

func readEntriesFile(path string) ([]Entry, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	return decodeEntries(f, path)
}

func decodeEntries(r io.Reader, name string) ([]Entry, int64, error) {
	br := bufio.NewReader(r)
	var (
		entries []Entry
		maxID   int64
		lineNo  int
		seen    = make(map[int64]struct{})
	)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			if line[len(line)-1] != '\n' {
				return nil, 0, fmt.Errorf("%s:%d: torn line (missing trailing newline)", name, lineNo)
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}
			var entry Entry
			if err := json.Unmarshal(trimmed, &entry); err != nil {
				return nil, 0, fmt.Errorf("%s:%d: bad JSON: %w", name, lineNo, err)
			}
			if entry.ID <= 0 {
				return nil, 0, fmt.Errorf("%s:%d: entry id must be positive", name, lineNo)
			}
			if _, dup := seen[entry.ID]; dup {
				return nil, 0, fmt.Errorf("%s:%d: duplicate entry id %d", name, lineNo, entry.ID)
			}
			seen[entry.ID] = struct{}{}
			entries = append(entries, entry)
			if entry.ID > maxID {
				maxID = entry.ID
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("%s: read: %w", name, err)
		}
	}
	return entries, maxID, nil
}

func maxIDFromUndoFile(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = f.Close() }()

	br := bufio.NewReader(f)
	var (
		maxID  int64
		lineNo int
	)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			if line[len(line)-1] != '\n' {
				return 0, fmt.Errorf("%s:%d: torn line (missing trailing newline)", path, lineNo)
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}
			id, scanErr := maxIDFromUndoLine(trimmed)
			if scanErr != nil {
				return 0, fmt.Errorf("%s:%d: %w", path, lineNo, scanErr)
			}
			if id > maxID {
				maxID = id
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("%s: read: %w", path, err)
		}
	}
	return maxID, nil
}

func maxIDFromUndoLine(line []byte) (int64, error) {
	var rec struct {
		ID     int64           `json:"id"`
		Before json.RawMessage `json:"before"`
		After  json.RawMessage `json:"after"`
	}
	if err := json.Unmarshal(line, &rec); err != nil {
		return 0, fmt.Errorf("bad JSON: %w", err)
	}
	maxID := rec.ID
	for _, raw := range []json.RawMessage{rec.Before, rec.After} {
		id, err := idFromRawEntry(raw)
		if err != nil {
			return 0, err
		}
		if id > maxID {
			maxID = id
		}
	}
	return maxID, nil
}

func idFromRawEntry(raw json.RawMessage) (int64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, nil
	}
	var entry struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(trimmed, &entry); err != nil {
		return 0, fmt.Errorf("bad entry snapshot: %w", err)
	}
	return entry.ID, nil
}

// normalizeHostEntries validates and normalizes entries for host, and clones
// each Entry's Tags slice so the returned entries (which ReplaceHost stores
// directly into s.byHost) do not alias the caller-supplied Tags backing
// arrays. Without this, a caller mutating a Tags slice it passed into
// ReplaceHost could later corrupt the store's in-memory state (100 Go
// Mistakes #25).
func normalizeHostEntries(host string, entries []Entry) ([]Entry, error) {
	out := make([]Entry, len(entries))
	for i, e := range entries {
		if e.ID <= 0 {
			return nil, fmt.Errorf("entry id must be positive")
		}
		entryHost := strings.TrimSpace(e.Host)
		if entryHost == "" {
			e.Host = host
		} else {
			normalized, err := NormalizeHost(entryHost)
			if err != nil {
				return nil, err
			}
			if normalized != host {
				return nil, fmt.Errorf("entry host %q does not match %q", normalized, host)
			}
			e.Host = host
		}
		e.Tags = slices.Clone(e.Tags)
		out[i] = e
	}
	return out, nil
}

func sortEntriesByEpoch(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		switch {
		case a.Epoch < b.Epoch:
			return -1
		case a.Epoch > b.Epoch:
			return 1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
}

// NormalizeHost trims host and rejects empty names, path-traversal segments,
// and newline/NUL bytes, since host feeds directly into db.<host>.jsonl and
// undo.<host>.jsonl file names.
//
// Exported (rather than kept package-private) so internal/worktime/legacy's
// migrate/export code — split out of this package in task e81 — can apply
// the same host-name validation the store itself uses, without duplicating
// the rules or reaching into store internals.
func NormalizeHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("host must not be empty")
	}
	if strings.ContainsAny(host, "/\\") || strings.Contains(host, "..") {
		return "", fmt.Errorf("invalid host %q", host)
	}
	if strings.ContainsRune(host, '\n') || strings.ContainsRune(host, '\x00') {
		return "", fmt.Errorf("invalid host %q", host)
	}
	return host, nil
}

func dbFileName(host string) string {
	return dbFilePrefix + host + dbFileSuffix
}

func undoFileName(host string) string {
	return undoFilePrefix + host + undoFileSuffix
}

func hostFromDBPath(path string) (string, error) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, dbFilePrefix) || !strings.HasSuffix(base, dbFileSuffix) {
		return "", fmt.Errorf("unexpected db file name %q", base)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(base, dbFilePrefix), dbFileSuffix)
	return NormalizeHost(host)
}

func hostFromUndoPath(path string) (string, error) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, undoFilePrefix) || !strings.HasSuffix(base, undoFileSuffix) {
		return "", fmt.Errorf("unexpected undo file name %q", base)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(base, undoFilePrefix), undoFileSuffix)
	return NormalizeHost(host)
}
