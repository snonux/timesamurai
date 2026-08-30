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
	"time"
)

// Undo operation names written to undo.<host>.jsonl.
const (
	OpInsert    = "insert"
	OpModify    = "modify"
	OpDelete    = "delete"
	opTombstone = "tombstone" // id reservation after undoing an insert; not replayed
)

// ErrNoUndo is returned when UndoLast has no actionable record for the host.
var ErrNoUndo = errors.New("no undo record")

// UndoRecord is one append-only undo.<host>.jsonl line.
//
// Field order matches the plan examples: ts, op, id, before, after.
// before is null for insert; after is null for delete.
type UndoRecord struct {
	TS     int64  `json:"ts"`
	Op     string `json:"op"`
	ID     int64  `json:"id"`
	Before *Entry `json:"before"`
	After  *Entry `json:"after"`
}

// AppendUndo appends rec to undo.<host>.jsonl with a single O_APPEND write.
// Empty TS is filled with time.Now().Unix(). Ids in the record bump the host watermark.
func (s *Store) AppendUndo(ctx context.Context, host string, rec UndoRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	host, err := NormalizeHost(host)
	if err != nil {
		return err
	}
	if rec.TS == 0 {
		rec.TS = time.Now().Unix()
	}
	if err := validateUndoRecord(rec); err != nil {
		return err
	}

	line, err := marshalUndoLine(rec)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, undoFileName(host))
	if err := appendJSONLLine(path, line); err != nil {
		return fmt.Errorf("append undo for host %q: %w", host, err)
	}
	s.bumpNextIDFromUndoLocked(host, rec)
	if _, ok := s.byHost[host]; !ok {
		s.byHost[host] = nil
	}
	return nil
}

// UndoLast reverts the most recent actionable undo record for host.
//
// insert → remove the after entry (leaves a tombstone so the id stays reserved);
// modify → restore before; delete → restore before. The consumed record is removed
// from the log (tombstones and remaining history stay for id allocation).
//
// applyUndoLocked (rewrites db.<host>.jsonl) and rewriteUndoFile (rewrites
// undo.<host>.jsonl) each write their own file atomically via temp+fsync+rename,
// but the pair is not atomic together. Task 581: if rewriteUndoFile failed after
// applyUndoLocked had already landed, the DB came back reverted while the undo
// log still listed the same record as pending, so a retry died with "entry id
// not found" even though the caller believed nothing had happened. To close that
// window, UndoLast snapshots the pre-undo entries before mutating and, if the
// undo-log rewrite fails, rolls the DB back to that snapshot so the two files
// never disagree about whether rec is still undoable.
func (s *Store) UndoLast(ctx context.Context, host string) (UndoRecord, error) {
	if err := ctx.Err(); err != nil {
		return UndoRecord{}, err
	}
	host, err := NormalizeHost(host)
	if err != nil {
		return UndoRecord{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, undoFileName(host))
	records, err := readUndoRecords(path)
	if err != nil {
		return UndoRecord{}, err
	}

	idx := lastActionableUndoIndex(records)
	if idx < 0 {
		return UndoRecord{}, ErrNoUndo
	}
	rec := records[idx]
	kept := keptUndoRecords(records, idx, rec)
	prevEntries := cloneEntries(s.byHost[host])

	if err := s.applyUndoLocked(ctx, host, rec); err != nil {
		return UndoRecord{}, err
	}
	if err := rewriteUndoFile(path, kept); err != nil {
		return UndoRecord{}, s.rollbackAfterFailedUndoRewrite(ctx, host, prevEntries, err)
	}
	return rec, nil
}

// keptUndoRecords returns the undo log contents after consuming records[idx]: the
// consumed record is dropped, and an undone insert leaves a tombstone behind so
// its id stays reserved for allocation even though it is no longer replayable.
func keptUndoRecords(records []UndoRecord, idx int, rec UndoRecord) []UndoRecord {
	kept := append(append([]UndoRecord{}, records[:idx]...), records[idx+1:]...)
	if rec.Op == OpInsert && rec.After != nil {
		kept = append(kept, insertTombstone(rec))
	}
	return kept
}

// rollbackAfterFailedUndoRewrite restores host's DB to prevEntries after
// applyUndoLocked mutated it but the undo-log rewrite then failed. allowRestore
// is passed as true because prevEntries is simply the state that was on disk
// moments earlier, so the usual "id already used" guard does not apply.
//
// If the rollback write itself fails (e.g. disk exhausted for both files), the
// DB and undo log can end up genuinely split; that residual case is reported
// via the combined error message rather than silently swallowed, since a
// two-file store without a shared journal cannot guarantee full atomicity
// against a second consecutive I/O failure.
func (s *Store) rollbackAfterFailedUndoRewrite(ctx context.Context, host string, prevEntries []Entry, rewriteErr error) error {
	if rbErr := s.replaceHostLocked(ctx, host, prevEntries, true); rbErr != nil {
		return fmt.Errorf("rewrite undo for host %q: %w (rollback to prior db state also failed: %v)", host, rewriteErr, rbErr)
	}
	return fmt.Errorf("rewrite undo for host %q: %w (db rolled back, undo log unchanged)", host, rewriteErr)
}

// cloneEntries returns a deep copy of entries, cloning each Entry's Tags slice so
// the snapshot cannot alias — and later be corrupted by mutations of — the
// store's backing array (100 Go Mistakes #25 "not making a deep copy"; same
// rationale as Store.Entries).
func cloneEntries(entries []Entry) []Entry {
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

func insertTombstone(rec UndoRecord) UndoRecord {
	before := *rec.After
	return UndoRecord{
		TS:     time.Now().Unix(),
		Op:     opTombstone,
		ID:     rec.ID,
		Before: &before,
		After:  nil,
	}
}

func (s *Store) applyUndoLocked(ctx context.Context, host string, rec UndoRecord) error {
	switch rec.Op {
	case OpInsert:
		return s.removeEntryByIDLocked(ctx, host, rec.ID)
	case OpModify:
		if rec.Before == nil {
			return errors.New("modify undo missing before")
		}
		return s.putEntryRestoringLocked(ctx, host, *rec.Before)
	case OpDelete:
		if rec.Before == nil {
			return errors.New("delete undo missing before")
		}
		return s.putEntryRestoringLocked(ctx, host, *rec.Before)
	default:
		return fmt.Errorf("unsupported undo op %q", rec.Op)
	}
}

func (s *Store) removeEntryByIDLocked(ctx context.Context, host string, id int64) error {
	cur := s.byHost[host]
	next := make([]Entry, 0, len(cur))
	found := false
	for _, e := range cur {
		if e.ID == id {
			found = true
			continue
		}
		next = append(next, e)
	}
	if !found {
		return fmt.Errorf("undo insert: entry id %d for host %q not found", id, host)
	}
	return s.replaceHostLocked(ctx, host, next, false)
}

func (s *Store) putEntryRestoringLocked(ctx context.Context, host string, entry Entry) error {
	entry.Host = host
	cur := s.byHost[host]
	next := make([]Entry, len(cur))
	copy(next, cur)
	replaced := false
	for i, e := range next {
		if e.ID == entry.ID {
			next[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, entry)
	}
	sortEntriesByEpoch(next)
	return s.replaceHostLocked(ctx, host, next, true)
}

func (s *Store) bumpNextIDFromUndoLocked(host string, rec UndoRecord) {
	s.bumpNextIDLocked(host, rec.ID)
	if rec.Before != nil {
		s.bumpNextIDLocked(host, rec.Before.ID)
	}
	if rec.After != nil {
		s.bumpNextIDLocked(host, rec.After.ID)
	}
}

func validateUndoRecord(rec UndoRecord) error {
	if rec.TS <= 0 {
		return errors.New("undo ts must be positive")
	}
	if rec.ID <= 0 {
		return errors.New("undo id must be positive")
	}
	switch rec.Op {
	case OpInsert:
		if rec.Before != nil {
			return errors.New("insert undo must have null before")
		}
		if rec.After == nil {
			return errors.New("insert undo must have after")
		}
		if rec.After.ID != rec.ID {
			return fmt.Errorf("insert undo id %d does not match after.id %d", rec.ID, rec.After.ID)
		}
	case OpModify:
		if rec.Before == nil || rec.After == nil {
			return errors.New("modify undo requires before and after")
		}
		if rec.Before.ID != rec.ID || rec.After.ID != rec.ID {
			return fmt.Errorf("modify undo id %d must match before and after", rec.ID)
		}
	case OpDelete:
		if rec.After != nil {
			return errors.New("delete undo must have null after")
		}
		if rec.Before == nil {
			return errors.New("delete undo must have before")
		}
		if rec.Before.ID != rec.ID {
			return fmt.Errorf("delete undo id %d does not match before.id %d", rec.ID, rec.Before.ID)
		}
	case opTombstone:
		return errors.New("tombstone records are internal only")
	default:
		return fmt.Errorf("unsupported undo op %q", rec.Op)
	}
	return nil
}

func lastActionableUndoIndex(records []UndoRecord) int {
	for i := len(records) - 1; i >= 0; i-- {
		switch records[i].Op {
		case OpInsert, OpModify, OpDelete:
			return i
		}
	}
	return -1
}

func marshalUndoLine(rec UndoRecord) ([]byte, error) {
	payload, err := json.Marshal(rec)
	if err != nil {
		return nil, fmt.Errorf("marshal undo record id %d: %w", rec.ID, err)
	}
	if bytes.Contains(payload, []byte{'\n'}) {
		return nil, fmt.Errorf("undo record id %d marshaled with embedded newline", rec.ID)
	}
	return append(payload, '\n'), nil
}

func readUndoRecords(path string) ([]UndoRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	br := bufio.NewReader(f)
	var (
		records []UndoRecord
		lineNo  int
	)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			if line[len(line)-1] != '\n' {
				return nil, fmt.Errorf("%s:%d: torn line (missing trailing newline)", path, lineNo)
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}
			var rec UndoRecord
			if err := json.Unmarshal(trimmed, &rec); err != nil {
				return nil, fmt.Errorf("%s:%d: bad JSON: %w", path, lineNo, err)
			}
			records = append(records, rec)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", path, err)
		}
	}
	return records, nil
}

func rewriteUndoFile(path string, records []UndoRecord) error {
	if len(records) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove empty undo %q: %w", path, err)
		}
		return nil
	}

	tmp := path + tmpFileSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create temp %q: %w", tmp, err)
	}

	var writeErr error
	for i := range records {
		line, err := marshalUndoLine(records[i])
		if err != nil {
			writeErr = err
			break
		}
		if _, err := f.Write(line); err != nil {
			writeErr = err
			break
		}
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
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
