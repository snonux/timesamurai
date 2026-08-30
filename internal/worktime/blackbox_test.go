package worktime_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/snonux/timesamurai/internal/config"
	"github.com/snonux/timesamurai/internal/worktime"
)

// This is the JSONL-era home for what the pre-rewrite package's
// blackbox_test.go checked: that the package's *exported* surface alone —
// with no access to unexported helpers, sort orders, or internal
// representations — is enough for an outside caller to record time and
// build a report. Living in package worktime_test (an external test
// package) is the point: any assertion here that only compiles because of
// an unexported name would mean the public API isn't actually sufficient,
// which is exactly what this test exists to catch.
func TestBlackbox_PublicAPICanRecordAndReport(t *testing.T) {
	ctx := context.Background()
	store, err := worktime.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("worktime.Open: %v", err)
	}

	cfg := config.Default().Accounting
	host := "host-a"
	at := time.Date(2026, 1, 5, 9, 0, 0, 0, time.Local)

	if _, err := worktime.Start(ctx, store, cfg, host, nil, at, "start"); err != nil {
		t.Fatalf("worktime.Start: %v", err)
	}
	if _, err := worktime.Stop(ctx, store, cfg, host, nil, at.Add(time.Hour), "stop"); err != nil {
		t.Fatalf("worktime.Stop: %v", err)
	}
	if _, err := worktime.Add(ctx, store, cfg, host, []string{"lunch"}, 30*time.Minute, at, "lunch"); err != nil {
		t.Fatalf("worktime.Add: %v", err)
	}

	entries := store.Entries(host)
	if len(entries) != 3 {
		t.Fatalf("store.Entries(host) len = %d, want 3", len(entries))
	}

	collected := worktime.CollectEntries(store)
	if len(collected) != 3 {
		t.Fatalf("worktime.CollectEntries len = %d, want 3", len(collected))
	}

	weeks, err := worktime.BuildReport(collected, cfg, io.Discard)
	if err != nil {
		t.Fatalf("worktime.BuildReport: %v", err)
	}

	rendered := worktime.FormatReport(weeks, false)
	if !strings.Contains(rendered, "work:0.50h") {
		t.Fatalf("rendered report missing expected work value: %q", rendered)
	}
	if !strings.Contains(rendered, "lunch:0.50h") {
		t.Fatalf("rendered report missing expected lunch value: %q", rendered)
	}
}
