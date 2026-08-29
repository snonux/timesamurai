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

func errorContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNoUndo) && substr == ErrNoUndo.Error() {
		return true
	}
	return strings.Contains(err.Error(), substr)
}
