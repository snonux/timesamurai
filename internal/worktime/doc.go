// Package worktime models work-time log entries, tag accounting rules, the
// per-host JSONL store (db.<host>.jsonl), and the append-only undo log
// (undo.<host>.jsonl) used to revert insert/modify/delete mutations.
//
// entries.go composes the Store and undo log into the mutation API
// (Start/Stop/Add/Sub/UseBuffer/Modify/Delete): every mutation validates
// its entry, writes it, and records an undo entry, and Modify/Delete
// address existing entries as "<host>:<id>" (ParseAddress), with a bare id
// meaning the current host.
//
// This package holds only the core, always-needed domain logic: model,
// store, undo, entries (mutation), query, and report. The worktime.rb-era
// concerns — the legacy Ruby-JSON codec, one-shot migration from that
// format, and export back to it — live in the sibling package
// internal/worktime/legacy instead (task e81). Those exist only for the
// dual-tool coexistence window and have a different reason to change (the
// legacy format's quirks, not this package's runtime domain rules), so
// SRP calls for keeping them out of this package; legacy imports this one
// for Store/Entry/action constants/validation, never the other way around.
package worktime
