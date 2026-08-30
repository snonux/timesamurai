package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// sampleRow builds one worktime.Row for edit.go's pure-function tests,
// avoiding a real store for tests that never touch disk.
func sampleRow(host string, id, epoch, value int64, action string, tags []string, descr string) worktime.Row {
	entry := worktime.Entry{ID: id, Action: action, Epoch: epoch, Host: host, Value: value, Tags: tags, Descr: descr}
	return worktime.Row{Address: fmt.Sprintf("%s:%d", host, id), Entry: entry}
}

// TestRenderParseEditBlockRoundTrip confirms render -> parse recovers the
// same field values the row was built from, for every field the format
// carries.
func TestRenderParseEditBlockRoundTrip(t *testing.T) {
	row := sampleRow("earth", 7, 1767085200, 3600, "add", []string{"work", "offsite"}, "coding session")

	block := renderEditBlock([]worktime.Row{row})
	lines, err := parseEditBlock(block)
	if err != nil {
		t.Fatalf("parseEditBlock: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("want 1 parsed line, got %d:\n%s", len(lines), block)
	}
	got := lines[0]
	if got.Address != row.Address {
		t.Errorf("address = %q, want %q", got.Address, row.Address)
	}
	if got.Action != row.Entry.Action {
		t.Errorf("action = %q, want %q", got.Action, row.Entry.Action)
	}
	if got.Epoch != row.Entry.Epoch {
		t.Errorf("epoch = %d, want %d", got.Epoch, row.Entry.Epoch)
	}
	if got.Value != row.Entry.Value {
		t.Errorf("value = %d, want %d", got.Value, row.Entry.Value)
	}
	if strings.Join(got.Tags, ",") != strings.Join(row.Entry.Tags, ",") {
		t.Errorf("tags = %v, want %v", got.Tags, row.Entry.Tags)
	}
	if got.Descr != row.Entry.Descr {
		t.Errorf("descr = %q, want %q", got.Descr, row.Entry.Descr)
	}
}

// TestParseEditBlockSkipsBlankAndCommentLines confirms the header comment
// and stray blank lines a human might leave behind don't turn into entries.
func TestParseEditBlockSkipsBlankAndCommentLines(t *testing.T) {
	block := "# address\taction\tat\tvalue\ttags\tdescr\n\n\t add \t2026-01-05 09:00:00\t3600\twork\thello\n\n"
	lines, err := parseEditBlock(block)
	if err != nil {
		t.Fatalf("parseEditBlock: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("want 1 line (comment/blanks skipped), got %d: %+v", len(lines), lines)
	}
}

// TestDiffEditBlockDetectsModify confirms an edited field on an existing
// address produces exactly one editOpModify carrying only the changed
// field.
func TestDiffEditBlockDetectsModify(t *testing.T) {
	row := sampleRow("earth", 1, 1000, 3600, "add", []string{"work"}, "old")
	edited := []editLine{{Address: row.Address, Action: "add", Epoch: 1000, Value: 3600, Tags: []string{"work"}, Descr: "new"}}

	ops, err := diffEditBlock([]worktime.Row{row}, edited)
	if err != nil {
		t.Fatalf("diffEditBlock: %v", err)
	}
	if len(ops) != 1 || ops[0].Kind != editOpModify {
		t.Fatalf("want 1 modify op, got %+v", ops)
	}
	if ops[0].Patch.Descr == nil || *ops[0].Patch.Descr != "new" {
		t.Errorf("patch.Descr = %v, want \"new\"", ops[0].Patch.Descr)
	}
	if ops[0].Patch.Epoch != nil || ops[0].Patch.Value != nil || ops[0].Patch.Tags != nil || ops[0].Patch.Action != nil {
		t.Errorf("only Descr changed, want every other patch field nil: %+v", ops[0].Patch)
	}
}

// TestDiffEditBlockNoChangeProducesNoOp confirms an untouched line produces
// no op at all, not a no-op modify.
func TestDiffEditBlockNoChangeProducesNoOp(t *testing.T) {
	row := sampleRow("earth", 1, 1000, 3600, "add", []string{"work"}, "same")
	edited := []editLine{{Address: row.Address, Action: "add", Epoch: 1000, Value: 3600, Tags: []string{"work"}, Descr: "same"}}

	ops, err := diffEditBlock([]worktime.Row{row}, edited)
	if err != nil {
		t.Fatalf("diffEditBlock: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("want 0 ops for an unchanged line, got %+v", ops)
	}
}

// TestDiffEditBlockDetectsDelete confirms an address missing from the
// edited block becomes an editOpDelete.
func TestDiffEditBlockDetectsDelete(t *testing.T) {
	row := sampleRow("earth", 1, 1000, 3600, "add", []string{"work"}, "gone soon")

	ops, err := diffEditBlock([]worktime.Row{row}, nil)
	if err != nil {
		t.Fatalf("diffEditBlock: %v", err)
	}
	if len(ops) != 1 || ops[0].Kind != editOpDelete || ops[0].Address != row.Address {
		t.Fatalf("want 1 delete op for %q, got %+v", row.Address, ops)
	}
}

// TestDiffEditBlockDetectsInsert confirms an address-less line becomes an
// editOpInsert carrying the parsed line verbatim.
func TestDiffEditBlockDetectsInsert(t *testing.T) {
	newLine := editLine{Action: "add", Epoch: 2000, Value: 1800, Tags: []string{"lunch"}, Descr: "new entry"}

	ops, err := diffEditBlock(nil, []editLine{newLine})
	if err != nil {
		t.Fatalf("diffEditBlock: %v", err)
	}
	if len(ops) != 1 || ops[0].Kind != editOpInsert {
		t.Fatalf("want 1 insert op, got %+v", ops)
	}
	if ops[0].Insert.Descr != "new entry" {
		t.Errorf("insert.Descr = %q, want %q", ops[0].Insert.Descr, "new entry")
	}
}

// TestDiffEditBlockUnknownAddressErrors confirms an edited line referencing
// an address absent from the original rows fails loudly instead of being
// silently treated as an insert.
func TestDiffEditBlockUnknownAddressErrors(t *testing.T) {
	edited := []editLine{{Address: "earth:999", Action: "add", Epoch: 1000, Value: 3600}}
	if _, err := diffEditBlock(nil, edited); err == nil {
		t.Fatal("diffEditBlock with an unknown address: want error, got nil")
	}
}

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

// TestEditCommandStubEditorRoundTrip runs the full `work edit` command with
// $EDITOR stubbed to a tiny shell script that rewrites one entry's
// description in place, confirming the end-to-end wiring (render -> launch
// $EDITOR -> parse -> diff -> apply) works without a real interactive
// editor. The heavier diff/apply logic itself is covered in isolation by
// the tests above; this only exercises the plumbing between them. The stub
// script assumes a POSIX shell, matching this project's Linux-only
// environment (see ~/.claude/CLAUDE.md's Fedora Linux dev machine).
func TestEditCommandStubEditorRoundTrip(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work", "--descr", "before edit"); err != nil {
		t.Fatalf("work add: %v", err)
	}

	script := filepath.Join(t.TempDir(), "fake-editor.sh")
	// The stub "editor" just replaces "before edit" with "after edit" in
	// place, standing in for a human doing the same thing interactively.
	content := "#!/bin/sh\nsed -i 's/before edit/after edit/' \"$1\"\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write stub editor: %v", err)
	}
	t.Setenv("EDITOR", script)

	out, err := runWork(t, store, "edit")
	if err != nil {
		t.Fatalf("work edit: %v", err)
	}
	if !strings.Contains(out, "modify") {
		t.Errorf("expected a modify to be reported, got %q", out)
	}

	entries := readEntries(t, store, currentHost(t))
	if len(entries) != 1 || entries[0].Descr != "after edit" {
		t.Fatalf("want descr \"after edit\", got %+v", entries)
	}
}

// TestEditCommandNoEditorFails confirms a missing $EDITOR fails clearly
// instead of silently doing nothing or guessing a default editor.
func TestEditCommandNoEditorFails(t *testing.T) {
	store := newScratchStore(t)
	if _, err := runWork(t, store, "add", "1h", "work"); err != nil {
		t.Fatalf("work add: %v", err)
	}
	t.Setenv("EDITOR", "")

	if _, err := runWork(t, store, "edit"); err == nil {
		t.Fatal("work edit with no $EDITOR: want error, got nil")
	}
}
