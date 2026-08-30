package worktime

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/config"
)

// This file is the JSONL-era home for what the pre-rewrite package's
// comprehensive_test.go checked: that a whole day's worth of mutations,
// written through the public entry points (Start/Stop/Add) rather than
// constructed by hand, round-trips through the Store and produces the same
// report a human would expect from worktime.rb. The individual pieces
// (Store persistence, entries.go's validation, report.go's byte-for-byte
// rendering) each have their own focused unit tests elsewhere in this
// package; this test exists solely to catch a wiring mistake between them
// that no single-file test could see — e.g. Store.Entries returning entries
// in an order BuildReport doesn't expect, or a tag written by Add not being
// classified the way report.go's entryCategory reads it back.
//
// The fixture and expected numbers are deliberately the same ones
// pre-rewrite's comprehensive_test.go used (8h login/logout work session,
// 1h lunch the same day, 8h off the next day, default 40h week target):
// reusing them shows the ported package reproduces the same accounting
// result the old positional-index implementation did, not just a
// self-consistent new one.
func TestIntegration_StartStopAddThroughStoreToReport(t *testing.T) {
	store, ctx := openStore(t)
	cfg := config.Default().Accounting
	host := "fixture-host"

	day1Login := time.Date(2026, 1, 5, 9, 0, 0, 0, time.Local)
	day1Lunch := time.Date(2026, 1, 5, 12, 0, 0, 0, time.Local)
	day1Logout := time.Date(2026, 1, 5, 17, 0, 0, 0, time.Local)
	day2Off := time.Date(2026, 1, 6, 10, 0, 0, 0, time.Local)

	if _, err := Start(ctx, store, cfg, host, nil, day1Login, "start"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := Add(ctx, store, cfg, host, []string{"lunch"}, time.Hour, day1Lunch, "lunch"); err != nil {
		t.Fatalf("Add(lunch): %v", err)
	}
	if _, err := Stop(ctx, store, cfg, host, nil, day1Logout, "stop"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := Add(ctx, store, cfg, host, []string{"off"}, 8*time.Hour, day2Off, "off"); err != nil {
		t.Fatalf("Add(off): %v", err)
	}

	// Round-trip: what Start/Stop/Add wrote must come back out of the store.
	stored := store.Entries(host)
	if len(stored) != 4 {
		t.Fatalf("store.Entries(host) len = %d, want 4: %+v", len(stored), stored)
	}

	merged := CollectEntries(store)
	if len(merged) != 4 {
		t.Fatalf("CollectEntries len = %d, want 4", len(merged))
	}

	weeks, err := BuildReport(merged, cfg, io.Discard)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if len(weeks) != 1 {
		t.Fatalf("weeks len = %d, want 1", len(weeks))
	}

	week := weeks[0]
	// work: 8h logged - 1h lunch (minusfor) = 7h.
	if got, want := week.Values[WorkTag], int64(7*secondsPerHour); got != want {
		t.Fatalf("week work = %d, want %d", got, want)
	}
	// off is plusfor: target = 40h - 8h = 32h; balance = work(7h) - target(32h) = -25h.
	if got, want := week.Values["balance"], int64(-25*secondsPerHour); got != want {
		t.Fatalf("week balance = %d, want %d", got, want)
	}

	rendered := FormatReport(weeks, false)
	for _, want := range []string{"work:7.00h", "off:8.00h", "balance:-25.00h"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered report missing %q:\n%s", want, rendered)
		}
	}
}
