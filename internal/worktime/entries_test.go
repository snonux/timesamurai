package worktime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// openStore is a small helper shared by the entries tests below; it mirrors
// the setup every store_test.go case starts with.
func openStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, ctx
}

func TestStart_CreatesLoginAndUndoRecord(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	entry, err := Start(ctx, store, cfg, "earth", nil, time.Unix(100, 0), "begin")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if entry.ID != 1 || entry.Action != actionLogin || entry.Host != "earth" || entry.Epoch != 100 {
		t.Fatalf("unexpected login entry: %+v", entry)
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != WorkTag {
		t.Fatalf("expected default tag [%q], got %v", WorkTag, entry.Tags)
	}

	got := store.Entries("earth")
	if len(got) != 1 || got[0].ID != entry.ID {
		t.Fatalf("store.Entries(earth): got %+v", got)
	}

	// Every mutation must write an undo record; UndoLast is the existing
	// r61 machinery, so a successful revert proves the record was written
	// correctly, not just present.
	rec, err := store.UndoLast(ctx, "earth")
	if err != nil {
		t.Fatalf("UndoLast: %v", err)
	}
	if rec.Op != OpInsert || rec.ID != entry.ID {
		t.Fatalf("unexpected undo record: %+v", rec)
	}
	if got := store.Entries("earth"); len(got) != 0 {
		t.Fatalf("expected undo to remove the login, got %+v", got)
	}
}

func TestStart_ZeroTimeDefaultsToNow(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	before := time.Now().Unix()
	entry, err := Start(ctx, store, cfg, "earth", nil, time.Time{}, "")
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if entry.Epoch < before || entry.Epoch > after {
		t.Fatalf("Epoch = %d, want within [%d, %d]", entry.Epoch, before, after)
	}
}

func TestStart_AlreadyLoggedInSameHost(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", nil, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	_, err := Start(ctx, store, cfg, "earth", nil, time.Unix(200, 0), "")
	if !errors.Is(err, ErrAlreadyLoggedIn) {
		t.Fatalf("second Start: got %v, want ErrAlreadyLoggedIn", err)
	}
}

func TestStart_AlreadyLoggedInAcrossHosts(t *testing.T) {
	// Login/logout is a single cross-host state machine: logging "work" on
	// earth must block starting "work" on moon too, not just on earth.
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", nil, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Start earth: %v", err)
	}
	_, err := Start(ctx, store, cfg, "moon", nil, time.Unix(200, 0), "")
	if !errors.Is(err, ErrAlreadyLoggedIn) {
		t.Fatalf("Start moon: got %v, want ErrAlreadyLoggedIn", err)
	}
}

func TestStart_DifferentCategoriesDoNotConflict(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", []string{"work"}, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Start work: %v", err)
	}
	if _, err := Start(ctx, store, cfg, "earth", []string{"lunch"}, time.Unix(110, 0), ""); err != nil {
		t.Fatalf("Start lunch: %v", err)
	}
}

func TestStart_LabelOnlyTagsFallBackToWork(t *testing.T) {
	// A tag set with no work/plus/minus/buffer tag (only a label) falls back
	// to WorkTag, so it must conflict with an explicit "work" login.
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", []string{"offsite"}, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Start offsite: %v", err)
	}
	_, err := Start(ctx, store, cfg, "earth", []string{"work"}, time.Unix(110, 0), "")
	if !errors.Is(err, ErrAlreadyLoggedIn) {
		t.Fatalf("Start work: got %v, want ErrAlreadyLoggedIn", err)
	}
}

func TestStart_RejectsMultipleAccountingTags(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	_, err := Start(ctx, store, cfg, "earth", []string{"work", "lunch"}, time.Unix(100, 0), "")
	if !errors.Is(err, ErrMultipleAccountingTags) {
		t.Fatalf("Start: got %v, want ErrMultipleAccountingTags", err)
	}
}

func TestStop_NotLoggedIn(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	_, err := Stop(ctx, store, cfg, "earth", nil, time.Unix(100, 0), "")
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("Stop: got %v, want ErrNotLoggedIn", err)
	}
}

func TestStartStop_FullCycleThenRestart(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", nil, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	logout, err := Stop(ctx, store, cfg, "earth", nil, time.Unix(200, 0), "done")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if logout.Action != actionLogout || logout.Epoch != 200 {
		t.Fatalf("unexpected logout entry: %+v", logout)
	}

	// Logout closes the category everywhere, so starting again must succeed.
	if _, err := Start(ctx, store, cfg, "moon", nil, time.Unix(300, 0), ""); err != nil {
		t.Fatalf("restart on moon: %v", err)
	}
}

func TestStop_ClosesOnRequestedHostRegardlessOfLoginHost(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Start(ctx, store, cfg, "earth", nil, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Start earth: %v", err)
	}
	logout, err := Stop(ctx, store, cfg, "moon", nil, time.Unix(200, 0), "")
	if err != nil {
		t.Fatalf("Stop moon: %v", err)
	}
	if logout.Host != "moon" {
		t.Fatalf("logout.Host = %q, want moon", logout.Host)
	}
}

func TestAdd_DefaultAndExplicitTags(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	added, err := Add(ctx, store, cfg, "earth", nil, 30*time.Minute, time.Unix(100, 0), "manual")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.Value != 1800 || len(added.Tags) != 1 || added.Tags[0] != WorkTag {
		t.Fatalf("unexpected Add entry: %+v", added)
	}

	tagged, err := Add(ctx, store, cfg, "earth", []string{"work", "blogpost"}, time.Hour, time.Unix(200, 0), "")
	if err != nil {
		t.Fatalf("Add tagged: %v", err)
	}
	if tagged.Value != 3600 || len(tagged.Tags) != 2 || tagged.Tags[1] != "blogpost" {
		t.Fatalf("unexpected tagged Add entry: %+v", tagged)
	}
}

func TestSub_NegatesValue(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	subbed, err := Sub(ctx, store, cfg, "earth", []string{"lunch"}, 15*time.Minute, time.Unix(100, 0), "")
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if subbed.Value != -900 {
		t.Fatalf("Sub value = %d, want -900", subbed.Value)
	}
}

func TestAddSub_RejectNonPositiveDuration(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", nil, 0, time.Unix(1, 0), ""); err == nil {
		t.Fatal("Add(0): expected error")
	}
	if _, err := Sub(ctx, store, cfg, "earth", nil, -time.Minute, time.Unix(1, 0), ""); err == nil {
		t.Fatal("Sub(negative): expected error")
	}
}

func TestInsertEntry_ValidationFailureDoesNotBurnID(t *testing.T) {
	// insertEntry peeks NextID rather than consuming AllocID, so a rejected
	// mutation must not create a gap: the next successful entry gets the
	// same id the failed one would have used.
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", []string{"work", "lunch"}, time.Minute, time.Unix(1, 0), ""); !errors.Is(err, ErrMultipleAccountingTags) {
		t.Fatalf("expected ErrMultipleAccountingTags, got %v", err)
	}

	ok, err := Add(ctx, store, cfg, "earth", nil, time.Minute, time.Unix(2, 0), "")
	if err != nil {
		t.Fatalf("Add after failed Add: %v", err)
	}
	if ok.ID != 1 {
		t.Fatalf("ID = %d, want 1 (failed validation must not burn an id)", ok.ID)
	}
	if got := store.Entries("earth"); len(got) != 1 {
		t.Fatalf("store.Entries: got %+v, want exactly the successful entry", got)
	}
}

func TestUseBuffer_WithdrawsAndCredits(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	entries, err := UseBuffer(ctx, store, cfg, "earth", 10*time.Minute, time.Unix(100, 0), "transfer")
	if err != nil {
		t.Fatalf("UseBuffer: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("UseBuffer: got %d entries, want 2", len(entries))
	}
	removed, added := entries[0], entries[1]
	if removed.Tags[0] != bufferSourceTag || removed.Value != -600 {
		t.Fatalf("unexpected withdrawal entry: %+v", removed)
	}
	if added.Tags[0] != WorkTag || added.Value != 600 {
		t.Fatalf("unexpected credit entry: %+v", added)
	}

	got := store.Entries("earth")
	if len(got) != 2 {
		t.Fatalf("store.Entries: got %d, want 2", len(got))
	}
}

func TestUseBuffer_RejectsNonPositiveDuration(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := UseBuffer(ctx, store, cfg, "earth", 0, time.Unix(1, 0), ""); err == nil {
		t.Fatal("UseBuffer(0): expected error")
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		currentHost string
		wantHost    string
		wantID      int64
		wantErr     bool
	}{
		{name: "host and id", addr: "earth:412", currentHost: "moon", wantHost: "earth", wantID: 412},
		{name: "bare id uses current host", addr: "412", currentHost: "moon", wantHost: "moon", wantID: 412},
		{name: "trims whitespace", addr: "  earth:412  ", currentHost: "moon", wantHost: "earth", wantID: 412},
		{name: "empty address", addr: "", currentHost: "moon", wantErr: true},
		{name: "missing id", addr: "earth:", currentHost: "moon", wantErr: true},
		{name: "non-numeric id", addr: "earth:abc", currentHost: "moon", wantErr: true},
		{name: "zero id", addr: "earth:0", currentHost: "moon", wantErr: true},
		{name: "negative id", addr: "earth:-1", currentHost: "moon", wantErr: true},
		{name: "empty host segment", addr: ":5", currentHost: "moon", wantErr: true},
		{name: "invalid host chars", addr: "ear/th:5", currentHost: "moon", wantErr: true},
		{name: "extra colon is invalid id", addr: "earth:5:6", currentHost: "moon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, id, err := ParseAddress(tt.addr, tt.currentHost)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseAddress(%q): expected error", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q): %v", tt.addr, err)
			}
			if host != tt.wantHost || id != tt.wantID {
				t.Fatalf("ParseAddress(%q) = (%q, %d), want (%q, %d)", tt.addr, host, id, tt.wantHost, tt.wantID)
			}
		})
	}
}

func TestModify_ByBareID(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	added, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), "orig")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	newDescr := "fixed"
	newEpoch := int64(150)
	got, err := Modify(ctx, store, cfg, "1", "earth", EntryPatch{Epoch: &newEpoch, Descr: &newDescr})
	if err != nil {
		t.Fatalf("Modify: %v", err)
	}
	if got.ID != added.ID || got.Host != "earth" || got.Epoch != 150 || got.Descr != "fixed" {
		t.Fatalf("unexpected modified entry: %+v", got)
	}
	if got.Value != added.Value {
		t.Fatalf("Value changed unexpectedly: got %d want %d", got.Value, added.Value)
	}

	stored := store.Entries("earth")
	if len(stored) != 1 || stored[0].Descr != "fixed" {
		t.Fatalf("store not updated: %+v", stored)
	}
}

func TestModify_CrossHostAddress(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), "orig"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	newDescr := "fixed from moon"
	got, err := Modify(ctx, store, cfg, "earth:1", "moon", EntryPatch{Descr: &newDescr})
	if err != nil {
		t.Fatalf("Modify with explicit host: %v", err)
	}
	if got.Host != "earth" {
		t.Fatalf("Modify must not move the entry to currentHost: got host %q", got.Host)
	}

	// A bare id under currentHost="moon" must not find the entry that lives
	// on earth: addressing is per-host, not global.
	if _, err := Modify(ctx, store, cfg, "1", "moon", EntryPatch{Descr: &newDescr}); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Modify bare id under wrong host: got %v, want ErrEntryNotFound", err)
	}
}

func TestModify_NonexistentID(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	newDescr := "x"
	_, err := Modify(ctx, store, cfg, "earth:999", "earth", EntryPatch{Descr: &newDescr})
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Modify nonexistent: got %v, want ErrEntryNotFound", err)
	}
}

func TestModify_RejectsInvalidResultAndLeavesStoreUnchanged(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	badAction := "not-a-real-action"
	_, err := Modify(ctx, store, cfg, "1", "earth", EntryPatch{Action: &badAction})
	if err == nil {
		t.Fatal("Modify with invalid action: expected error")
	}

	stored := store.Entries("earth")
	if len(stored) != 1 || stored[0].Action != actionAdd {
		t.Fatalf("store must be unchanged after rejected modify: %+v", stored)
	}
}

func TestModify_UndoRestoresBefore(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), "orig"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	newDescr := "changed"
	if _, err := Modify(ctx, store, cfg, "1", "earth", EntryPatch{Descr: &newDescr}); err != nil {
		t.Fatalf("Modify: %v", err)
	}

	rec, err := store.UndoLast(ctx, "earth")
	if err != nil {
		t.Fatalf("UndoLast: %v", err)
	}
	if rec.Op != OpModify {
		t.Fatalf("undo op = %q, want modify", rec.Op)
	}
	stored := store.Entries("earth")
	if len(stored) != 1 || stored[0].Descr != "orig" {
		t.Fatalf("undo did not restore original descr: %+v", stored)
	}
}

func TestDelete_ByAddress(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	added, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	deleted, err := Delete(ctx, store, "1", "earth")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted.ID != added.ID {
		t.Fatalf("Delete returned %+v, want id %d", deleted, added.ID)
	}
	if got := store.Entries("earth"); len(got) != 0 {
		t.Fatalf("store.Entries after delete: %+v", got)
	}

	// The deleted id must stay reserved (Store's documented invariant).
	next, err := store.NextID("earth")
	if err != nil {
		t.Fatalf("NextID: %v", err)
	}
	if next != 2 {
		t.Fatalf("NextID after delete = %d, want 2", next)
	}
}

func TestDelete_CrossHostAddress(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	if _, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if _, err := Delete(ctx, store, "earth:1", "moon"); err != nil {
		t.Fatalf("Delete with explicit host: %v", err)
	}
	if got := store.Entries("earth"); len(got) != 0 {
		t.Fatalf("store.Entries(earth) after delete: %+v", got)
	}
}

func TestDelete_NonexistentID(t *testing.T) {
	store, ctx := openStore(t)

	_, err := Delete(ctx, store, "earth:1", "earth")
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("Delete nonexistent: got %v, want ErrEntryNotFound", err)
	}
}

func TestDelete_UndoRestoresEntry(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	added, err := Add(ctx, store, cfg, "earth", nil, time.Hour, time.Unix(100, 0), "keep me")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := Delete(ctx, store, "1", "earth"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rec, err := store.UndoLast(ctx, "earth")
	if err != nil {
		t.Fatalf("UndoLast: %v", err)
	}
	if rec.Op != OpDelete {
		t.Fatalf("undo op = %q, want delete", rec.Op)
	}

	stored := store.Entries("earth")
	if len(stored) != 1 || stored[0].ID != added.ID || stored[0].Descr != "keep me" {
		t.Fatalf("undo did not restore deleted entry: %+v", stored)
	}
}

func TestModify_InvalidAddress(t *testing.T) {
	store, ctx := openStore(t)
	cfg := testAccountingConfig()

	newDescr := "x"
	_, err := Modify(ctx, store, cfg, "earth:abc", "earth", EntryPatch{Descr: &newDescr})
	if err == nil {
		t.Fatal("Modify with malformed address: expected error")
	}
}
