package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// This file implements the edit-block text protocol `work edit` (edit.go)
// speaks with $EDITOR: rendering entries to the text block, parsing an
// edited block back into structured lines, and diffing the parsed lines
// against the original rows to decide what changed. Applying the resulting
// ops via worktime mutations lives in edit_apply.go; the $EDITOR/scratch-file
// plumbing lives in edit_editor.go.
//
// # Text-block format
//
// One entry per line, six tab-separated fields in this fixed order:
//
//		<address>\t<action>\t<at>\t<value>\t<tags>\t<descr>
//
//	  - address  "<host>:<id>" for an existing entry, or empty for a line the
//	    editing session added -- an empty address is exactly how a brand-new
//	    entry is recognized on the way back in, so it must stay empty rather
//	    than, say, "new".
//	  - action   login, logout, or add (worktime's fixed action set).
//	  - at       "YYYY-MM-DD HH:MM:SS", a format internal/timefmt.ParseTime
//	    already accepts, so the rendered text parses back byte-for-byte; a
//	    human editing the file can also type any other ParseTime shorthand
//	    ("today", "-2h", a bare epoch) and have it work.
//	  - value    the entry's signed value in seconds, rendered as a plain
//	    integer (0 for login/logout), accepted back through
//	    internal/timefmt.ParseDuration, which also takes "1h"/"-30m" shorthand
//	    if a human prefers to type it that way.
//	  - tags     comma-joined, no spaces around the commas.
//	  - descr    free text, everything after the fifth tab to end of line; any
//	    literal tab or newline already in a description is rendered as a
//	    single space (descriptions are free text a human typed, not meant to
//	    carry structural characters, so this is a lossy but harmless
//	    substitution on the way out, never on the way back in).
//
// Lines that are blank or start with "#" are ignored, so the rendered block
// can carry a header comment. Deleting a line deletes that entry; changing a
// field on an existing line modifies it; adding a line with no address
// inserts a new entry on the current host.
const editBlockHeader = "# address\taction\tat\tvalue\ttags\tdescr"

// editLine is one parsed row of the text block, either bound to an existing
// entry (Address set) or describing a line the editing session added
// (Address empty).
type editLine struct {
	Address string
	Action  string
	Epoch   int64
	Value   int64
	Tags    []string
	Descr   string
}

// editOpKind names what diffEditBlock decided to do with one line.
type editOpKind int

const (
	editOpModify editOpKind = iota
	editOpDelete
	editOpInsert
)

// editOp is one mutation diffEditBlock derived from comparing the edited
// block against the original rendering. Applying a slice of these (in
// order, via applyEditOps in edit_apply.go) is what turns a text edit into
// individual, undo-logged store mutations.
type editOp struct {
	Kind    editOpKind
	Address string              // set for Modify/Delete
	Patch   worktime.EntryPatch // set for Modify
	Insert  editLine            // set for Insert
}

// renderEditBlock renders rows as the text-block format documented above.
func renderEditBlock(rows []worktime.Row) string {
	var b strings.Builder
	b.WriteString(editBlockHeader)
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString(renderEditLine(row.Address, row.Entry))
		b.WriteByte('\n')
	}
	return b.String()
}

func renderEditLine(address string, e worktime.Entry) string {
	at := time.Unix(e.Epoch, 0).Format("2006-01-02 15:04:05")
	tags := strings.Join(e.Tags, ",")
	descr := sanitizeEditField(e.Descr)
	return fmt.Sprintf("%s\t%s\t%s\t%d\t%s\t%s", address, e.Action, at, e.Value, tags, descr)
}

// sanitizeEditField replaces tab/newline/carriage-return with a space so a
// stray structural character in a stored description can never be confused
// with the block's own field or line separators.
func sanitizeEditField(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
}

// parseEditBlock parses the text an editor session produced back into
// editLines, skipping blank lines and "#" comments.
func parseEditBlock(text string) ([]editLine, error) {
	var lines []editLine
	for i, raw := range strings.Split(text, "\n") {
		trimmed := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(trimmed) == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}
		line, err := parseEditLine(trimmed)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func parseEditLine(raw string) (editLine, error) {
	fields := strings.SplitN(raw, "\t", 6)
	if len(fields) != 6 {
		return editLine{}, fmt.Errorf("want 6 tab-separated fields, got %d", len(fields))
	}
	at, err := timefmt.ParseTime(fields[2])
	if err != nil {
		return editLine{}, fmt.Errorf("at: %w", err)
	}
	value, err := timefmt.ParseDuration(fields[3])
	if err != nil {
		return editLine{}, fmt.Errorf("value: %w", err)
	}
	return editLine{
		Address: strings.TrimSpace(fields[0]),
		Action:  strings.TrimSpace(fields[1]),
		Epoch:   at.Unix(),
		Value:   int64(value.Seconds()),
		Tags:    splitEditTags(fields[4]),
		Descr:   fields[5],
	}, nil
}

func splitEditTags(field string) []string {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil
	}
	parts := strings.Split(field, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// diffEditBlock compares edited against original (the rows the block was
// rendered from) and returns the ops needed to bring the store in line:
// a modify per changed existing line, a delete per address missing from
// edited, and an insert per address-less line. An edited line whose address
// doesn't match any original row is an error -- most likely a typo in a
// hand-edited address, which should fail loudly rather than silently
// becoming an unrelated insert.
func diffEditBlock(original []worktime.Row, edited []editLine) ([]editOp, error) {
	byAddr := make(map[string]worktime.Entry, len(original))
	for _, row := range original {
		byAddr[row.Address] = row.Entry
	}

	var ops []editOp
	seen := make(map[string]bool, len(edited))
	for _, line := range edited {
		if line.Address == "" {
			ops = append(ops, editOp{Kind: editOpInsert, Insert: line})
			continue
		}
		entry, ok := byAddr[line.Address]
		if !ok {
			return nil, fmt.Errorf("%w: %s", worktime.ErrEntryNotFound, line.Address)
		}
		seen[line.Address] = true
		if patch, changed := diffEntryFields(entry, line); changed {
			ops = append(ops, editOp{Kind: editOpModify, Address: line.Address, Patch: patch})
		}
	}
	return append(ops, deletedOps(original, seen)...), nil
}

// deletedOps returns one editOpDelete per original row whose address never
// showed up in the edited block, in original (epoch) order, so deletion
// order is deterministic across runs.
func deletedOps(original []worktime.Row, seen map[string]bool) []editOp {
	var ops []editOp
	for _, row := range original {
		if !seen[row.Address] {
			ops = append(ops, editOp{Kind: editOpDelete, Address: row.Address})
		}
	}
	return ops
}

// diffEntryFields compares entry (the stored value) against line (the
// edited value) field by field, returning a patch that touches only the
// fields that actually changed -- the same "only touch what changed"
// discipline `work modify` applies from its flags, applied here from a diff
// instead.
func diffEntryFields(entry worktime.Entry, line editLine) (worktime.EntryPatch, bool) {
	var patch worktime.EntryPatch
	changed := false
	if entry.Action != line.Action {
		action := line.Action
		patch.Action = &action
		changed = true
	}
	if entry.Epoch != line.Epoch {
		epoch := line.Epoch
		patch.Epoch = &epoch
		changed = true
	}
	if entry.Value != line.Value {
		value := line.Value
		patch.Value = &value
		changed = true
	}
	if !slices.Equal(entry.Tags, line.Tags) {
		tags := line.Tags
		patch.Tags = &tags
		changed = true
	}
	if entry.Descr != line.Descr {
		descr := line.Descr
		patch.Descr = &descr
		changed = true
	}
	return patch, changed
}
