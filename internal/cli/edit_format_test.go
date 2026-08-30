package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/snonux/timesamurai/internal/worktime"
)

// sampleRow builds one worktime.Row for edit_format.go's pure-function
// tests, avoiding a real store for tests that never touch disk.
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
