package worktime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpen_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := store.Hosts(); len(got) != 0 {
		t.Fatalf("Hosts: got %v, want empty", got)
	}
	id, err := store.NextID("earth")
	if err != nil {
		t.Fatalf("NextID: %v", err)
	}
	if id != 1 {
		t.Fatalf("NextID: got %d, want 1", id)
	}
}

func TestOpen_RejectsEmptyDir(t *testing.T) {
	_, err := Open(context.Background(), "  ")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestStore_AppendLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	entries := []Entry{
		{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}},
		{ID: 2, Action: "logout", Epoch: 200, Host: "earth", Tags: []string{"work"}},
		{ID: 3, Action: "add", Epoch: 300, Host: "earth", Value: 3600, Tags: []string{"work"}, Descr: "notes"},
	}
	for _, e := range entries {
		if err := store.Append(ctx, e); err != nil {
			t.Fatalf("Append %#v: %v", e, err)
		}
	}

	got := store.Entries("earth")
	if len(got) != len(entries) {
		t.Fatalf("Entries len: got %d want %d", len(got), len(entries))
	}
	for i := range entries {
		if !entryEqual(got[i], entries[i]) {
			t.Fatalf("entry %d: got %+v want %+v", i, got[i], entries[i])
		}
	}

	reloaded, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	got = reloaded.Entries("earth")
	if len(got) != len(entries) {
		t.Fatalf("reloaded len: got %d want %d", len(got), len(entries))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "db.earth.jsonl"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	wantLine := `{"id":3,"action":"add","epoch":300,"host":"earth","value":3600,"tags":["work"],"descr":"notes"}` + "\n"
	if !bytes.Contains(raw, []byte(wantLine)) {
		t.Fatalf("file missing stable JSON line:\n%s\nfile:\n%s", wantLine, raw)
	}
}

func TestStore_AppendOutOfOrderRewritesSorted(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 300, Host: "moon", Tags: []string{"work"}}); err != nil {
		t.Fatalf("Append first: %v", err)
	}
	if err := store.Append(ctx, Entry{ID: 2, Action: "add", Epoch: 100, Host: "moon", Value: 60, Tags: []string{"work"}}); err != nil {
		t.Fatalf("Append out of order: %v", err)
	}

	got := store.Entries("moon")
	if len(got) != 2 || got[0].Epoch != 100 || got[1].Epoch != 300 {
		t.Fatalf("expected epoch-sorted entries, got %+v", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "db.moon.jsonl"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d (%q)", len(lines), raw)
	}
	var first Entry
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if first.ID != 2 || first.Epoch != 100 {
		t.Fatalf("first on disk should be id=2 epoch=100, got %+v", first)
	}
}

func TestStore_ReplaceHost(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 10, Host: "mars", Tags: []string{"work"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(ctx, Entry{ID: 2, Action: "logout", Epoch: 20, Host: "mars", Tags: []string{"work"}}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Modify id=1 and delete id=2 via replace.
	err = store.ReplaceHost(ctx, "mars", []Entry{
		{ID: 1, Action: "login", Epoch: 15, Host: "mars", Tags: []string{"work"}, Descr: "fixed"},
	})
	if err != nil {
		t.Fatalf("ReplaceHost: %v", err)
	}

	got := store.Entries("mars")
	if len(got) != 1 || got[0].Epoch != 15 || got[0].Descr != "fixed" {
		t.Fatalf("after replace: %+v", got)
	}

	next, err := store.NextID("mars")
	if err != nil {
		t.Fatalf("NextID: %v", err)
	}
	if next != 3 {
		t.Fatalf("deleted id must stay reserved: NextID=%d want 3", next)
	}

	// Reintroducing deleted id 2 must fail (same invariant as Append).
	err = store.ReplaceHost(ctx, "mars", []Entry{
		{ID: 1, Action: "login", Epoch: 15, Host: "mars", Tags: []string{"work"}, Descr: "fixed"},
		{ID: 2, Action: "logout", Epoch: 20, Host: "mars", Tags: []string{"work"}},
	})
	if err == nil {
		t.Fatal("ReplaceHost must not reintroduce deleted id 2")
	}
}

func TestStore_ReplaceHostDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	err = store.ReplaceHost(ctx, "mars", []Entry{
		{ID: 1, Action: "login", Epoch: 10, Host: "mars", Tags: []string{"work"}},
		{ID: 1, Action: "logout", Epoch: 20, Host: "mars", Tags: []string{"work"}},
	})
	if err == nil {
		t.Fatal("expected duplicate id rejection in ReplaceHost batch")
	}
}

func TestStore_InvalidHost(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx := context.Background()
	for _, host := range []string{"", " ../earth", "earth/x"} {
		err := store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 1, Host: host, Tags: []string{"work"}})
		if err == nil {
			t.Fatalf("expected error for host %q", host)
		}
	}
}

func TestOpen_CanceledContext(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Open(ctx, dir)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestStore_IDAllocationConsidersUndo(t *testing.T) {
	tests := []struct {
		name    string
		dbLines []string
		undo    string
		wantID  int64
	}{
		{
			name:   "empty store starts at 1",
			wantID: 1,
		},
		{
			name: "max from entries only",
			dbLines: []string{
				`{"id":4,"action":"login","epoch":1,"host":"earth","tags":["work"]}`,
				`{"id":7,"action":"logout","epoch":2,"host":"earth","tags":["work"]}`,
			},
			wantID: 8,
		},
		{
			name: "max from undo top-level id",
			dbLines: []string{
				`{"id":2,"action":"login","epoch":1,"host":"earth","tags":["work"]}`,
			},
			undo:   `{"ts":1,"op":"delete","id":9,"before":{"id":9,"action":"add","epoch":1,"host":"earth","value":1,"tags":["work"]},"after":null}` + "\n",
			wantID: 10,
		},
		{
			name:   "max from undo before snapshot when higher",
			undo:   `{"ts":1,"op":"delete","id":3,"before":{"id":12,"action":"add","epoch":1,"host":"earth","value":1,"tags":["work"]},"after":null}` + "\n",
			wantID: 13,
		},
		{
			name:   "undo only host still reserves ids",
			undo:   `{"ts":1,"op":"delete","id":5,"before":{"id":5,"action":"add","epoch":1,"host":"earth","value":1,"tags":["work"]},"after":null}` + "\n",
			wantID: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if len(tt.dbLines) > 0 {
				var buf bytes.Buffer
				for _, line := range tt.dbLines {
					buf.WriteString(line)
					buf.WriteByte('\n')
				}
				if err := os.WriteFile(filepath.Join(dir, "db.earth.jsonl"), buf.Bytes(), 0o644); err != nil {
					t.Fatalf("write db: %v", err)
				}
			}
			if tt.undo != "" {
				if err := os.WriteFile(filepath.Join(dir, "undo.earth.jsonl"), []byte(tt.undo), 0o644); err != nil {
					t.Fatalf("write undo: %v", err)
				}
			}

			store, err := Open(context.Background(), dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			got, err := store.NextID("earth")
			if err != nil {
				t.Fatalf("NextID: %v", err)
			}
			if got != tt.wantID {
				t.Fatalf("NextID: got %d want %d", got, tt.wantID)
			}
			alloc, err := store.AllocID("earth")
			if err != nil {
				t.Fatalf("AllocID: %v", err)
			}
			if alloc != tt.wantID {
				t.Fatalf("AllocID: got %d want %d", alloc, tt.wantID)
			}
		})
	}
}

func TestStore_RejectIDReuse(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	e := Entry{ID: 1, Action: "login", Epoch: 10, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, e); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(ctx, e); err == nil {
		t.Fatal("expected duplicate id rejection")
	}

	// Delete via replace but keep id reserved.
	if err := store.ReplaceHost(ctx, "earth", nil); err != nil {
		t.Fatalf("ReplaceHost: %v", err)
	}
	if err := store.Append(ctx, e); err == nil {
		t.Fatal("expected reused deleted id rejection")
	}

	next, err := store.AllocID("earth")
	if err != nil {
		t.Fatalf("AllocID: %v", err)
	}
	if next != 2 {
		t.Fatalf("AllocID after delete: got %d want 2", next)
	}
}

func TestStore_RejectIDReuseFromUndoLog(t *testing.T) {
	dir := t.TempDir()
	undo := `{"ts":1,"op":"delete","id":5,"before":{"id":5,"action":"add","epoch":1,"host":"earth","value":1,"tags":["work"]},"after":null}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "undo.earth.jsonl"), []byte(undo), 0o644); err != nil {
		t.Fatalf("write undo: %v", err)
	}

	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	err = store.Append(ctx, Entry{ID: 5, Action: "add", Epoch: 1, Host: "earth", Value: 1, Tags: []string{"work"}})
	if err == nil {
		t.Fatal("expected rejection of id present only in undo log")
	}

	id, err := store.AllocID("earth")
	if err != nil {
		t.Fatalf("AllocID: %v", err)
	}
	if id != 6 {
		t.Fatalf("AllocID: got %d want 6", id)
	}
}

func TestOpen_Negatives(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "torn line missing newline",
			files: map[string]string{
				"db.earth.jsonl": `{"id":1,"action":"login","epoch":1,"host":"earth","tags":["work"]}`,
			},
			wantErr: "torn line",
		},
		{
			name: "bad JSON",
			files: map[string]string{
				"db.earth.jsonl": "{not-json\n",
			},
			wantErr: "bad JSON",
		},
		{
			name: "non-positive id",
			files: map[string]string{
				"db.earth.jsonl": `{"id":0,"action":"login","epoch":1,"host":"earth","tags":["work"]}` + "\n",
			},
			wantErr: "positive",
		},
		{
			name: "duplicate id on load",
			files: map[string]string{
				"db.earth.jsonl": `{"id":1,"action":"login","epoch":1,"host":"earth","tags":["work"]}` + "\n" +
					`{"id":1,"action":"logout","epoch":2,"host":"earth","tags":["work"]}` + "\n",
			},
			wantErr: "duplicate entry id",
		},
		{
			name: "torn undo line",
			files: map[string]string{
				"undo.earth.jsonl": `{"ts":1,"op":"delete","id":1`,
			},
			wantErr: "torn line",
		},
		{
			name: "bad undo JSON",
			files: map[string]string{
				"undo.earth.jsonl": "not-json\n",
			},
			wantErr: "bad JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			_, err := Open(context.Background(), dir)
			if err == nil {
				t.Fatal("expected error")
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tt.wantErr)) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestStore_AppendCanceledContext(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 1, Host: "earth", Tags: []string{"work"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestAppendJSONL_ConcurrentNoTornLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.earth.jsonl")

	const goroutines = 32
	const perG = 40

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				entry := Entry{
					ID:     int64(g*perG + i + 1),
					Action: "add",
					Epoch:  int64(g*perG + i + 1),
					Host:   "earth",
					Value:  1,
					Tags:   []string{"work"},
					Descr:  fmt.Sprintf("g%d-%d", g, i),
				}
				line, err := marshalEntryLine(entry)
				if err != nil {
					errCh <- err
					return
				}
				if err := appendJSONLLine(path, line); err != nil {
					errCh <- err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent append: %v", err)
	}

	entries, _, err := readEntriesFile(path)
	if err != nil {
		t.Fatalf("reload after concurrent append: %v", err)
	}
	want := goroutines * perG
	if len(entries) != want {
		t.Fatalf("entry count: got %d want %d", len(entries), want)
	}

	seen := make(map[int64]struct{}, len(entries))
	for _, e := range entries {
		if _, ok := seen[e.ID]; ok {
			t.Fatalf("duplicate id %d after concurrent append", e.ID)
		}
		seen[e.ID] = struct{}{}
		if e.Host != "earth" || e.Action != "add" {
			t.Fatalf("corrupted entry: %+v", e)
		}
	}
}

func TestStore_MultiHost(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 1, Host: "earth", Tags: []string{"work"}}); err != nil {
		t.Fatalf("earth: %v", err)
	}
	if err := store.Append(ctx, Entry{ID: 1, Action: "login", Epoch: 1, Host: "moon", Tags: []string{"work"}}); err != nil {
		t.Fatalf("moon: %v", err)
	}

	hosts := store.Hosts()
	if len(hosts) != 2 || hosts[0] != "earth" || hosts[1] != "moon" {
		t.Fatalf("Hosts: got %v", hosts)
	}
}

// TestStore_EntriesReturnsDeepCopy is a regression test for task 881 (100 Go
// Mistakes #25): Entries() must return Entry structs whose Tags slice does
// not alias the store's backing array. Mutating Tags on a returned Entry
// must never be visible in a subsequent call to Entries().
func TestStore_EntriesReturnsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	entry := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: []string{"work"}}
	if err := store.Append(ctx, entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got := store.Entries("earth")
	if len(got) != 1 {
		t.Fatalf("Entries len: got %d want 1", len(got))
	}

	// Mutate the Tags slice on the returned Entry in place.
	got[0].Tags[0] = "corrupted"
	got[0].Tags = append(got[0].Tags, "extra")

	again := store.Entries("earth")
	if len(again) != 1 {
		t.Fatalf("Entries len after mutation: got %d want 1", len(again))
	}
	if len(again[0].Tags) != 1 || again[0].Tags[0] != "work" {
		t.Fatalf("store corrupted by mutating a returned Entry's Tags: got %+v", again[0].Tags)
	}
}

// TestStore_AppendClonesTagsSlice is a regression test for task 881 (100 Go
// Mistakes #25): Append() must clone the caller-supplied Tags slice on
// ingest. Mutating that slice locally after Append returns must not affect
// the entry stored in the Store.
func TestStore_AppendClonesTagsSlice(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	tags := []string{"work"}
	entry := Entry{ID: 1, Action: "login", Epoch: 100, Host: "earth", Tags: tags}
	if err := store.Append(ctx, entry); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Mutate the local slice that was passed into Append.
	tags[0] = "corrupted"

	got := store.Entries("earth")
	if len(got) != 1 {
		t.Fatalf("Entries len: got %d want 1", len(got))
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "work" {
		t.Fatalf("store corrupted by mutating the caller's pre-Append Tags slice: got %+v", got[0].Tags)
	}
}

// fakeSyncCloser is a writable fake standing in for *os.File so tests can
// exercise writeSyncClose's Sync/Close failure paths without needing to
// force a real file's Sync or Close to fail, which isn't portable.
type fakeSyncCloser struct {
	writeErr error
	syncErr  error
	closeErr error
	written  []byte
	synced   bool
	closed   bool
}

func (f *fakeSyncCloser) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.written = append(f.written, p...)
	return len(p), nil
}

func (f *fakeSyncCloser) Sync() error {
	f.synced = true
	return f.syncErr
}

func (f *fakeSyncCloser) Close() error {
	f.closed = true
	return f.closeErr
}

func TestWriteSyncClose_SyncsBeforeReturningSuccess(t *testing.T) {
	f := &fakeSyncCloser{}
	writeErr, closeErr := writeSyncClose(f, []byte("line\n"))
	if writeErr != nil || closeErr != nil {
		t.Fatalf("writeSyncClose: got writeErr=%v closeErr=%v, want nil, nil", writeErr, closeErr)
	}
	if !f.synced {
		t.Fatal("writeSyncClose did not call Sync before returning success")
	}
	if !f.closed {
		t.Fatal("writeSyncClose did not call Close")
	}
	if string(f.written) != "line\n" {
		t.Fatalf("written: got %q, want %q", f.written, "line\n")
	}
}

func TestWriteSyncClose_SurfacesSyncError(t *testing.T) {
	wantErr := errors.New("sync boom")
	f := &fakeSyncCloser{syncErr: wantErr}
	writeErr, closeErr := writeSyncClose(f, []byte("line\n"))
	if !errors.Is(writeErr, wantErr) {
		t.Fatalf("writeErr: got %v, want %v", writeErr, wantErr)
	}
	if closeErr != nil {
		t.Fatalf("closeErr: got %v, want nil", closeErr)
	}
	if !f.closed {
		t.Fatal("Close must still run even though Sync failed, so the fd is not leaked")
	}
}

func TestWriteSyncClose_SurfacesCloseErrorEvenWhenSyncSucceeds(t *testing.T) {
	wantErr := errors.New("close boom")
	f := &fakeSyncCloser{closeErr: wantErr}
	writeErr, closeErr := writeSyncClose(f, []byte("line\n"))
	if writeErr != nil {
		t.Fatalf("writeErr: got %v, want nil", writeErr)
	}
	if !errors.Is(closeErr, wantErr) {
		t.Fatalf("closeErr: got %v, want %v", closeErr, wantErr)
	}
	if !f.synced {
		t.Fatal("Sync must have run before the (failing) Close")
	}
}

func TestAppendJSONLLine_WritesLineDurably(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.earth.jsonl")
	if err := appendJSONLLine(path, []byte(`{"id":1}`+"\n")); err != nil {
		t.Fatalf("appendJSONLLine: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != `{"id":1}`+"\n" {
		t.Fatalf("file contents: got %q", got)
	}
}

func entryEqual(a, b Entry) bool {
	if a.ID != b.ID || a.Action != b.Action || a.Epoch != b.Epoch || a.Host != b.Host || a.Value != b.Value || a.Descr != b.Descr {
		return false
	}
	if len(a.Tags) != len(b.Tags) {
		return false
	}
	for i := range a.Tags {
		if a.Tags[i] != b.Tags[i] {
			return false
		}
	}
	return true
}
