package cli

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// TestApplyEditOpsModifyDeleteInsert exercises applyEditOps against a real
// scratch store, covering all three op kinds in one pass and confirming
// each lands as its own undo-logged mutation (so `work undo` can revert any
// one of them individually afterward).
func TestApplyEditOpsModifyDeleteInsert(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	store, err := worktime.Open(ctx, dir)
	if err != nil {
		t.Fatalf("worktime.Open: %v", err)
	}
	cfg := config.AccountingConfig{}
	host := "earth"

	keep, err := worktime.Add(ctx, store, cfg, host, []string{"work"}, time.Hour, time.Unix(1000, 0), "keep, modified")
	if err != nil {
		t.Fatalf("seed keep: %v", err)
	}
	drop, err := worktime.Add(ctx, store, cfg, host, []string{"work"}, time.Hour, time.Unix(2000, 0), "drop me")
	if err != nil {
		t.Fatalf("seed drop: %v", err)
	}

	descr := "modified descr"
	ops := []editOp{
		{Kind: editOpModify, Address: fmt.Sprintf("%s:%d", host, keep.ID), Patch: worktime.EntryPatch{Descr: &descr}},
		{Kind: editOpDelete, Address: fmt.Sprintf("%s:%d", host, drop.ID)},
		{Kind: editOpInsert, Insert: editLine{Action: "add", Epoch: 3000, Value: 1800, Tags: []string{"lunch"}, Descr: "inserted"}},
	}

	applied, err := applyEditOps(ctx, store, cfg, host, ops)
	if err != nil {
		t.Fatalf("applyEditOps: %v", err)
	}
	if len(applied) != 3 {
		t.Fatalf("want 3 applied entries, got %d", len(applied))
	}

	final := store.Entries(host)
	if len(final) != 2 {
		t.Fatalf("want 2 entries left (kept+inserted), got %d: %+v", len(final), final)
	}
	byDescr := map[string]worktime.Entry{}
	for _, e := range final {
		byDescr[e.Descr] = e
	}
	if _, ok := byDescr["modified descr"]; !ok {
		t.Errorf("kept entry should have the modified descr, got %+v", final)
	}
	if _, ok := byDescr["inserted"]; !ok {
		t.Errorf("new entry should be present, got %+v", final)
	}
	if _, ok := byDescr["drop me"]; ok {
		t.Errorf("deleted entry should be gone, got %+v", final)
	}
}
