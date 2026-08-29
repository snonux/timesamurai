// Package worktime models work-time log entries, tag accounting rules, the
// per-host JSONL store (db.<host>.jsonl), and the append-only undo log
// (undo.<host>.jsonl) used to revert insert/modify/delete mutations.
//
// entries.go composes the Store and undo log into the mutation API
// (Start/Stop/Add/Sub/UseBuffer/Modify/Delete): every mutation validates
// its entry, writes it, and records an undo entry, and Modify/Delete
// address existing entries as "<host>:<id>" (ParseAddress), with a bare id
// meaning the current host.
package worktime
