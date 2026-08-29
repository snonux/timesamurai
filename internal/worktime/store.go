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
func (s *Store) Entries(host string) []Entry {
	host, err := normalizeHost(host)
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
	return out
}

// NextID returns the next unused id for host without consuming it.
func (s *Store) NextID(host string) (int64, error) {
	host, err := normalizeHost(host)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.nextIDLocked(host), nil
}

// AllocID consumes and returns the next unused id for host.
func (s *Store) AllocID(host string) (int64, error) {
	host, err := normalizeHost(host)
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

	host, err := normalizeHost(entry.Host)
	if err != nil {
		return err
	}
	entry.Host = host

	if entry.ID <= 0 {
		return errors.New("entry id must be positive")
	}

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
// even when the replacement omits them.
func (s *Store) ReplaceHost(ctx context.Context, host string, entries []Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := normalizeHost(host)
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
			if _, ok := currentIDs[e.ID]; !ok {
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

func appendJSONLLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %q for append: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	n, err := f.Write(line)
	if err != nil {
		return fmt.Errorf("append to %q: %w", path, err)
	}
	if n != len(line) {
		return fmt.Errorf("append to %q: short write %d/%d", path, n, len(line))
	}
	return nil
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
			normalized, err := normalizeHost(entryHost)
			if err != nil {
				return nil, err
			}
			if normalized != host {
				return nil, fmt.Errorf("entry host %q does not match %q", normalized, host)
			}
			e.Host = host
		}
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

func normalizeHost(host string) (string, error) {
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
	return normalizeHost(host)
}

func hostFromUndoPath(path string) (string, error) {
	base := filepath.Base(path)
	if !strings.HasPrefix(base, undoFilePrefix) || !strings.HasSuffix(base, undoFileSuffix) {
		return "", fmt.Errorf("unexpected undo file name %q", base)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(base, undoFilePrefix), undoFileSuffix)
	return normalizeHost(host)
}
