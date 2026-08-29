// Package worktime models work-time log entries, tag accounting rules, the
// per-host JSONL store (db.<host>.jsonl), and the append-only undo log
// (undo.<host>.jsonl) used to revert insert/modify/delete mutations.
package worktime
