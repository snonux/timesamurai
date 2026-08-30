package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// This file applies the editOps edit_format.go's diffEditBlock produces to
// the worktime store, via the same Start/Stop/Add/Sub/Modify/Delete
// primitives every other mutating verb uses -- edit.go's runEdit is the only
// caller, wiring the ops it gets from parseAndDiffEdit into applyEditOps.

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
