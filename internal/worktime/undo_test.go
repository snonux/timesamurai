package worktime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUndo_InsertModifyDeleteRevertWithTags(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, store *Store)
	}{
		{
			name: "insert reverts exactly including tags",
			run: func(t *testing.T, ctx context.Context, store *Store) {
				entry := Entry{
					ID: 1, Action: "add", Epoch: 100, Host: "earth", Value: 3600,
					Tags: []string{"work", "blogpost"}, Descr: "wrote post",
				}
				if err := store.Append(ctx, entry); err != nil {
					t.Fatalf("Append: %v", err)
				}
				if err := store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpInsert, ID: 1, After: &entry,
				}); err != nil {
					t.Fatalf("AppendUndo: %v", err)
				}

				got, err := store.UndoLast(ctx, "earth")
				if err != nil {
					t.Fatalf("UndoLast: %v", err)
				}
				if got.Op != OpInsert || got.ID != 1 {
					t.Fatalf("UndoLast record: %+v", got)
				}
				if entries := store.Entries("earth"); len(entries) != 0 {
					t.Fatalf("entries after undo insert: %+v", entries)
				}

				// Id must stay reserved across reopen via tombstone.
				reopened, err := Open(ctx, store.Dir())
				if err != nil {
					t.Fatalf("reopen: %v", err)
				}
				next, err := reopened.NextID("earth")
				if err != nil {
					t.Fatalf("NextID: %v", err)
				}
				if next != 2 {
					t.Fatalf("NextID after insert undo: got %d want 2", next)
				}
				err = reopened.Append(ctx, entry)
				if err == nil {
					t.Fatal("expected rejection of id reserved by insert-undo tombstone")
				}
			},
		},
		{
			name: "modify reverts exactly including tags",
			run: func(t *testing.T, ctx context.Context, store *Store) {
				before := Entry{
					ID: 1, Action: "add", Epoch: 100, Host: "earth", Value: 1800,
					Tags: []string{"work", "draft"}, Descr: "old",
				}
				after := Entry{
					ID: 1, Action: "add", Epoch: 100, Host: "earth", Value: 7200,
					Tags: []string{"off", "vacation"}, Descr: "new",
				}
				if err := store.Append(ctx, before); err != nil {
					t.Fatalf("Append: %v", err)
				}
				if err := store.ReplaceHost(ctx, "earth", []Entry{after}); err != nil {
					t.Fatalf("ReplaceHost: %v", err)
				}
				if err := store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 2, Op: OpModify, ID: 1, Before: &before, After: &after,
				}); err != nil {
					t.Fatalf("AppendUndo: %v", err)
				}

				if _, err := store.UndoLast(ctx, "earth"); err != nil {
					t.Fatalf("UndoLast: %v", err)
				}
				got := store.Entries("earth")
				if len(got) != 1 {
					t.Fatalf("entries: %+v", got)
				}
				if !reflect.DeepEqual(got[0], before) {
					t.Fatalf("entry after modify undo:\n got %#v\nwant %#v", got[0], before)
				}
			},
		},
		{
			name: "delete reverts exactly including tags",
			run: func(t *testing.T, ctx context.Context, store *Store) {
				entry := Entry{
					ID: 1, Action: "add", Epoch: 50, Host: "earth", Value: -600,
					Tags: []string{"lunch"}, Descr: "break",
				}
				other := Entry{
					ID: 2, Action: "login", Epoch: 10, Host: "earth",
					Tags: []string{"work"},
				}
				if err := store.Append(ctx, entry); err != nil {
					t.Fatalf("Append entry: %v", err)
				}
				if err := store.Append(ctx, other); err != nil {
					t.Fatalf("Append other: %v", err)
				}
				if err := store.ReplaceHost(ctx, "earth", []Entry{other}); err != nil {
					t.Fatalf("delete via ReplaceHost: %v", err)
				}
				if err := store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 3, Op: OpDelete, ID: 1, Before: &entry,
				}); err != nil {
					t.Fatalf("AppendUndo: %v", err)
				}

				if _, err := store.UndoLast(ctx, "earth"); err != nil {
					t.Fatalf("UndoLast: %v", err)
				}
				got := store.Entries("earth")
				want := []Entry{other, entry}
				sortEntriesByEpoch(want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("entries after delete undo:\n got %#v\nwant %#v", got, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx := context.Background()
			store, err := Open(ctx, dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			tt.run(t, ctx, store)
		})
	}
}

func TestUndo_Negatives(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, ctx context.Context, store *Store)
		call    func(t *testing.T, ctx context.Context, store *Store) error
		wantErr string
	}{
		{
			name: "UndoLast with empty log",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				_, err := store.UndoLast(ctx, "earth")
				return err
			},
			wantErr: ErrNoUndo.Error(),
		},
		{
			name: "UndoLast with only tombstones",
			setup: func(t *testing.T, ctx context.Context, store *Store) {
				path := filepath.Join(store.Dir(), "undo.earth.jsonl")
				line := `{"ts":1,"op":"tombstone","id":1,"before":{"id":1,"action":"add","epoch":1,"host":"earth","value":1,"tags":["work"]},"after":null}` + "\n"
				if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
					t.Fatalf("write tombstone: %v", err)
				}
			},
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				_, err := store.UndoLast(ctx, "earth")
				return err
			},
			wantErr: ErrNoUndo.Error(),
		},
		{
			name: "AppendUndo rejects insert with before",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				e := Entry{ID: 1, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}}
				return store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpInsert, ID: 1, Before: &e, After: &e,
				})
			},
			wantErr: "null before",
		},
		{
			name: "AppendUndo rejects modify without after",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				e := Entry{ID: 1, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}}
				return store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpModify, ID: 1, Before: &e,
				})
			},
			wantErr: "before and after",
		},
		{
			name: "AppendUndo rejects delete without before",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				return store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpDelete, ID: 1,
				})
			},
			wantErr: "must have before",
		},
		{
			name: "AppendUndo rejects unknown op",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				return store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: "update", ID: 1,
				})
			},
			wantErr: "unsupported undo op",
		},
		{
			name: "AppendUndo rejects id mismatch",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				e := Entry{ID: 2, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}}
				return store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpInsert, ID: 1, After: &e,
				})
			},
			wantErr: "does not match after.id",
		},
		{
			name: "undo insert when entry missing",
			setup: func(t *testing.T, ctx context.Context, store *Store) {
				e := Entry{ID: 1, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}}
				if err := store.AppendUndo(ctx, "earth", UndoRecord{
					TS: 1, Op: OpInsert, ID: 1, After: &e,
				}); err != nil {
					t.Fatalf("AppendUndo: %v", err)
				}
			},
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				_, err := store.UndoLast(ctx, "earth")
				return err
			},
			wantErr: "not found",
		},
		{
			name: "canceled context on AppendUndo",
			call: func(t *testing.T, ctx context.Context, store *Store) error {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				e := Entry{ID: 1, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}}
				return store.AppendUndo(canceled, "earth", UndoRecord{
					TS: 1, Op: OpInsert, ID: 1, After: &e,
				})
			},
			wantErr: context.Canceled.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx := context.Background()
			store, err := Open(ctx, dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if tt.setup != nil {
				tt.setup(t, ctx, store)
			}
			err = tt.call(t, ctx, store)
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != "" && !errorContains(err, tt.wantErr) {
				t.Fatalf("error %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestUndo_SkipsTombstoneToPriorRecord(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	a := Entry{ID: 1, Action: "add", Epoch: 10, Host: "earth", Value: 1, Tags: []string{"work", "a"}}
	b := Entry{ID: 2, Action: "add", Epoch: 20, Host: "earth", Value: 2, Tags: []string{"work", "b"}}
	if err := store.Append(ctx, a); err != nil {
		t.Fatalf("Append a: %v", err)
	}
	if err := store.Append(ctx, b); err != nil {
		t.Fatalf("Append b: %v", err)
	}
	if err := store.AppendUndo(ctx, "earth", UndoRecord{TS: 1, Op: OpInsert, ID: 1, After: &a}); err != nil {
		t.Fatalf("AppendUndo a: %v", err)
	}
	if err := store.AppendUndo(ctx, "earth", UndoRecord{TS: 2, Op: OpInsert, ID: 2, After: &b}); err != nil {
		t.Fatalf("AppendUndo b: %v", err)
	}

	if _, err := store.UndoLast(ctx, "earth"); err != nil {
		t.Fatalf("UndoLast b: %v", err)
	}
	if got := store.Entries("earth"); !reflect.DeepEqual(got, []Entry{a}) {
		t.Fatalf("after first undo: %+v", got)
	}

	// Second undo must skip the tombstone left by undoing insert 2.
	if _, err := store.UndoLast(ctx, "earth"); err != nil {
		t.Fatalf("UndoLast a: %v", err)
	}
	if got := store.Entries("earth"); len(got) != 0 {
		t.Fatalf("after second undo: %+v", got)
	}
}

func TestUndo_AppendRoundTripJSONShape(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	before := Entry{ID: 5, Action: "add", Epoch: 1, Host: "earth", Value: 9, Tags: []string{"work", "x"}}
	after := before
	after.Value = 10
	after.Tags = []string{"work", "y"}

	if err := store.AppendUndo(ctx, "earth", UndoRecord{
		TS: 99, Op: OpModify, ID: 5, Before: &before, After: &after,
	}); err != nil {
		t.Fatalf("AppendUndo: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "undo.earth.jsonl"))
	if err != nil {
		t.Fatalf("read undo: %v", err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Fatal("undo line must be LF-terminated")
	}
	var rec UndoRecord
	if err := json.Unmarshal(bytes.TrimSpace(raw), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.TS != 99 || rec.Op != OpModify || rec.ID != 5 {
		t.Fatalf("record meta: %+v", rec)
	}
	if !reflect.DeepEqual(rec.Before, &before) || !reflect.DeepEqual(rec.After, &after) {
		t.Fatalf("snapshots before=%#v after=%#v", rec.Before, rec.After)
	}
}

// TestUndo_FailedUndoLogRewriteRollsBackDB is the task-581 regression test.
//
// It forces rewriteUndoFile to fail *after* applyUndoLocked has already
// rewritten db.earth.jsonl on disk, by pre-creating the undo log's temp-file
// path ("undo.earth.jsonl.tmp") as a directory: rewriteUndoFile always writes
// through that exact path, and os.OpenFile(..., O_CREATE|O_WRONLY|O_TRUNC) on
// an existing directory fails immediately, before anything is written or
// renamed. This is a pure filesystem seam — no production code changes needed
// to inject the failure — and it leaves db.earth.jsonl (a different path in
// the same directory) completely unaffected by the induced failure, so any
// change observed in it must have come from applyUndoLocked or the fix's
// rollback path.
//
// Before the fix, this left the DB reverted (entry removed) while the undo
// log still listed the insert as pending — split state, and a follow-up
// UndoLast failed with "entry id not found" even though the log looked
// untouched. After the fix, UndoLast must roll the DB back to its pre-undo
// snapshot so the two files agree again: either the call fully succeeds, or
// both the DB and the undo log end up exactly as they were beforehand.
func TestUndo_FailedUndoLogRewriteRollsBackDB(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	entry := Entry{
		ID: 1, Action: "add", Epoch: 100, Host: "earth", Value: 3600,
		Tags: []string{"work", "blogpost"}, Descr: "wrote post",
	}
	if err := store.Append(ctx, entry); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.AppendUndo(ctx, "earth", UndoRecord{
		TS: 1, Op: OpInsert, ID: 1, After: &entry,
	}); err != nil {
		t.Fatalf("AppendUndo: %v", err)
	}

	dbPath := filepath.Join(dir, "db.earth.jsonl")
	undoPath := filepath.Join(dir, "undo.earth.jsonl")
	dbBefore, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db before: %v", err)
	}
	undoBefore, err := os.ReadFile(undoPath)
	if err != nil {
		t.Fatalf("read undo before: %v", err)
	}

	// Block the undo-log rewrite's temp file with a directory so it fails
	// after applyUndoLocked has already succeeded.
	undoTmp := undoPath + tmpFileSuffix
	if err := os.Mkdir(undoTmp, 0o755); err != nil {
		t.Fatalf("mkdir undo tmp blocker: %v", err)
	}

	_, err = store.UndoLast(ctx, "earth")
	if err == nil {
		// The blocker failed to trigger (e.g. platform quirk); nothing left
		// to assert about split state, but the log is a genuine surprise.
		t.Fatal("expected UndoLast to fail with the undo-log temp path blocked")
	}

	dbAfter, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db after: %v", err)
	}
	undoAfter, err := os.ReadFile(undoPath)
	if err != nil {
		t.Fatalf("read undo after: %v", err)
	}
	if !bytes.Equal(dbBefore, dbAfter) {
		t.Fatalf("db split from undo log after failed rewrite:\nbefore=%s\nafter=%s", dbBefore, dbAfter)
	}
	if !bytes.Equal(undoBefore, undoAfter) {
		t.Fatalf("undo log unexpectedly changed despite failed rewrite:\nbefore=%s\nafter=%s", undoBefore, undoAfter)
	}

	// In-memory view must match the rolled-back file, not the discarded
	// mid-flight mutation.
	got := store.Entries("earth")
	if len(got) != 1 || !reflect.DeepEqual(got[0], entry) {
		t.Fatalf("in-memory entries not rolled back: %+v", got)
	}

	// A fresh Open (reading the untouched files) should reach the identical
	// state, confirming disk and memory never actually diverged.
	if err := os.RemoveAll(undoTmp); err != nil {
		t.Fatalf("remove undo tmp blocker: %v", err)
	}
	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reGot := reopened.Entries("earth")
	if !reflect.DeepEqual(reGot, got) {
		t.Fatalf("reopened entries diverge from in-memory: got %+v want %+v", reGot, got)
	}

	// The undo record must still be undoable exactly as before the failed
	// attempt -- retrying should now succeed instead of failing with
	// "entry id not found".
	if _, err := reopened.UndoLast(ctx, "earth"); err != nil {
		t.Fatalf("retry UndoLast after rollback: %v", err)
	}
	if entries := reopened.Entries("earth"); len(entries) != 0 {
		t.Fatalf("entries after retried undo: %+v", entries)
	}
}

func errorContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNoUndo) && substr == ErrNoUndo.Error() {
		return true
	}
	return strings.Contains(err.Error(), substr)
}
