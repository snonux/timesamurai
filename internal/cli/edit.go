package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/timefmt"
	"github.com/snonux/timesamurai/internal/worktime"
)

// This file implements `work edit [range]`: render entries in range (or the
// full history when no range is given, matching `work list`'s own default)
// as a plain-text block, open it in $EDITOR, and turn whatever comes back
// into individual worktime.Modify/Delete/insert calls -- one mutation per
// changed line, so every change lands in the undo log separately instead of
// as one opaque batch operation.
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
// order) is what turns a text edit into individual, undo-logged store
// mutations.
type editOp struct {
	Kind    editOpKind
	Address string              // set for Modify/Delete
	Patch   worktime.EntryPatch // set for Modify
	Insert  editLine            // set for Insert
}

// newEditCmd builds `work edit [range]`.
func newEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit [range]",
		Short: "Edit entries in $EDITOR as a text block",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(cmd, positionalRange(args))
		},
	}
	// 571: same [range] completer list.go/report.go wire up.
	cmd.ValidArgsFunction = completeRanges
	return cmd
}

// runEdit loads the rows in range, opens them in $EDITOR, diffs the result
// against the original rendering, and applies each difference as its own
// mutation.
func runEdit(cmd *cobra.Command, rangeArg string) error {
	rt, err := newRuntime(cmd)
	if err != nil {
		return err
	}
	rows, err := editRows(rt, rangeArg)
	if err != nil {
		return err
	}

	edited, err := launchEditor(cmd, renderEditBlock(rows))
	if err != nil {
		return err
	}

	ops, err := parseAndDiffEdit(rows, edited)
	if err != nil {
		return err
	}

	applied, applyErr := applyEditOps(cmdContext(cmd), rt.store, rt.cfg.Accounting, rt.host, ops)
	printEditSummary(cmd, ops, applied, rt.verbose)
	return applyErr
}

// editRows resolves rangeArg (list.go's shared buildFilter: "" means the
// full history) against rt's already-open store, without opening a second
// runtime/store the way calling list.go's queryRows here would.
func editRows(rt *runtime, rangeArg string) ([]worktime.Row, error) {
	filter, err := buildFilter(rangeArg, filterFlagValues{})
	if err != nil {
		return nil, err
	}
	return worktime.Query(worktime.CollectEntries(rt.store), filter)
}

// parseAndDiffEdit turns the raw edited text into editOps, wrapping a parse
// failure so it reads distinctly from a diff failure (an edited line
// referencing an address that no longer exists).
func parseAndDiffEdit(rows []worktime.Row, edited string) ([]editOp, error) {
	lines, err := parseEditBlock(edited)
	if err != nil {
		return nil, fmt.Errorf("parse edited block: %w", err)
	}
	return diffEditBlock(rows, lines)
}

// printEditSummary reports each applied op via printEntryResult, the same
// "<verb> <host>:<id>"/verbose shape every other mutating verb uses.
func printEditSummary(cmd *cobra.Command, ops []editOp, applied []worktime.Entry, verbose bool) {
	if len(ops) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no changes")
		return
	}
	for i, entry := range applied {
		printEntryResult(cmd, editOpVerb(ops[i].Kind), entry, verbose)
	}
}

func editOpVerb(kind editOpKind) string {
	switch kind {
	case editOpDelete:
		return "delete"
	case editOpInsert:
		return "insert"
	default:
		return "modify"
	}
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

// applyEditOps issues one worktime mutation per op, in order, stopping at
// the first error. Each op is already its own durable, undo-logged
// mutation, so a partial batch here is not a partial write -- exactly the
// point of applying differences individually rather than as one opaque
// batch op.
func applyEditOps(ctx context.Context, store *worktime.Store, cfg config.AccountingConfig, host string, ops []editOp) ([]worktime.Entry, error) {
	applied := make([]worktime.Entry, 0, len(ops))
	for _, op := range ops {
		entry, err := applyEditOp(ctx, store, cfg, host, op)
		if err != nil {
			return applied, fmt.Errorf("apply edit at %q: %w", op.Address, err)
		}
		applied = append(applied, entry)
	}
	return applied, nil
}

func applyEditOp(ctx context.Context, store *worktime.Store, cfg config.AccountingConfig, host string, op editOp) (worktime.Entry, error) {
	switch op.Kind {
	case editOpDelete:
		return worktime.Delete(ctx, store, op.Address, host)
	case editOpModify:
		return worktime.Modify(ctx, store, cfg, op.Address, host, op.Patch)
	case editOpInsert:
		return insertEditLine(ctx, store, cfg, host, op.Insert)
	default:
		return worktime.Entry{}, fmt.Errorf("unsupported edit op %d", op.Kind)
	}
}

// insertEditLine dispatches a new (address-less) line to the worktime
// primitive matching its action: Start/Stop for login/logout (which also
// re-checks the login/logout state machine an edited-in session must still
// obey), Add/Sub for a credit or withdrawal depending on value's sign.
func insertEditLine(ctx context.Context, store *worktime.Store, cfg config.AccountingConfig, host string, line editLine) (worktime.Entry, error) {
	at := time.Unix(line.Epoch, 0)
	switch strings.ToLower(strings.TrimSpace(line.Action)) {
	case "login":
		return worktime.Start(ctx, store, cfg, host, line.Tags, at, line.Descr)
	case "logout":
		return worktime.Stop(ctx, store, cfg, host, line.Tags, at, line.Descr)
	case "add":
		return insertAddLine(ctx, store, cfg, host, at, line)
	default:
		return worktime.Entry{}, fmt.Errorf("unsupported action %q for a new entry", line.Action)
	}
}

// insertAddLine handles a new "add" line: worktime.Add/Sub both require a
// strictly positive duration and pick the sign themselves, so value's sign
// here selects which of the two to call.
func insertAddLine(ctx context.Context, store *worktime.Store, cfg config.AccountingConfig, host string, at time.Time, line editLine) (worktime.Entry, error) {
	if line.Value == 0 {
		return worktime.Entry{}, errors.New("a new add entry needs a nonzero value")
	}
	if line.Value > 0 {
		return worktime.Add(ctx, store, cfg, host, line.Tags, time.Duration(line.Value)*time.Second, at, line.Descr)
	}
	return worktime.Sub(ctx, store, cfg, host, line.Tags, time.Duration(-line.Value)*time.Second, at, line.Descr)
}

// launchEditor writes content to a scratch file, runs $EDITOR on it
// (inheriting cmd's stdin/stdout/stderr so an interactive editor works from
// the terminal), and returns the file's contents afterward. An unset
// $EDITOR fails clearly rather than falling back to a guessed default: a
// silently-chosen editor the user didn't ask for is worse than an explicit
// error naming the fix ("set $EDITOR").
func launchEditor(cmd *cobra.Command, content string) (string, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return "", errors.New("$EDITOR is not set; work edit needs an editor to open the entries in")
	}

	path, cleanup, err := writeEditScratchFile(content)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := runEditor(cmd, editor, path); err != nil {
		return "", err
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}
	return string(edited), nil
}

// writeEditScratchFile writes content to a fresh temp file and returns its
// path plus a cleanup func that removes it, so launchEditor can defer
// cleanup regardless of how runEditor or the later read turns out.
func writeEditScratchFile(content string) (string, func(), error) {
	f, err := os.CreateTemp("", "timesamurai-edit-*.txt")
	if err != nil {
		return "", nil, fmt.Errorf("create scratch file: %w", err)
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, fmt.Errorf("write scratch file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close scratch file: %w", err)
	}
	return path, cleanup, nil
}

// runEditor runs $EDITOR (which may itself carry arguments, e.g. "code
// --wait") against path, wiring cmd's stdio through so an interactive
// terminal editor behaves normally and a test's fake editor script can read
// cmd's injected stdin if it wants to.
func runEditor(cmd *cobra.Command, editor, path string) error {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return errors.New("$EDITOR is not set; work edit needs an editor to open the entries in")
	}
	args := append(append([]string{}, parts[1:]...), path)
	editorCmd := exec.CommandContext(cmdContext(cmd), parts[0], args...) //nolint:gosec // $EDITOR is operator-controlled, same trust level as running any local tool
	editorCmd.Stdin = cmd.InOrStdin()
	editorCmd.Stdout = cmd.OutOrStdout()
	editorCmd.Stderr = cmd.ErrOrStderr()
	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("run $EDITOR (%s): %w", editor, err)
	}
	return nil
}
