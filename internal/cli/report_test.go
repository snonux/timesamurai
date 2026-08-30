package cli

import (
	"strings"
	"testing"
)

// TestReportNoRangeIncludesEntireHistory is the golden-parity guardrail: with
// NO positional range, `work report` must dump the entire history (task 271's
// report.txt fixture is a full dump from the start of time), not some
// implicit default window like "this week". Two entries five months apart
// both showing up proves no implicit window was applied.
func TestReportNoRangeIncludesEntireHistory(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "2h", "work", "--at", "2026-01-05T09:00"); err != nil {
		t.Fatalf("work add january: %v", err)
	}
	if _, err := runWork(t, store, "add", "3h", "work", "--at", "2026-06-15T09:00"); err != nil {
		t.Fatalf("work add june: %v", err)
	}

	out, err := runWork(t, store, "report")
	if err != nil {
		t.Fatalf("work report: %v", err)
	}
	if !strings.Contains(out, "20260105") {
		t.Errorf("report with no range should include the january entry, got:\n%s", out)
	}
	if !strings.Contains(out, "20260615") {
		t.Errorf("report with no range should include the june entry, got:\n%s", out)
	}
}

// TestReportWithRangeFiltersEntries confirms an explicit [range] narrows the
// entries fed into BuildReport, unlike the no-args full-history path above.
func TestReportWithRangeFiltersEntries(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "2h", "work", "--at", "2026-01-05T09:00"); err != nil {
		t.Fatalf("work add january: %v", err)
	}
	if _, err := runWork(t, store, "add", "3h", "work", "--at", "2026-06-15T09:00"); err != nil {
		t.Fatalf("work add june: %v", err)
	}

	out, err := runWork(t, store, "report", "2026-01")
	if err != nil {
		t.Fatalf("work report 2026-01: %v", err)
	}
	if !strings.Contains(out, "20260105") {
		t.Errorf("report 2026-01 should include the january entry, got:\n%s", out)
	}
	if strings.Contains(out, "20260615") {
		t.Errorf("report 2026-01 should exclude the june entry, got:\n%s", out)
	}
}

// TestReportInvalidRangeFails confirms a bad [range] argument surfaces an
// error rather than silently falling back to the full-history path.
func TestReportInvalidRangeFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "report", "not-a-range"); err == nil {
		t.Fatal("report not-a-range: want error, got nil")
	}
}

// TestReportEmptyStoreProducesEmptyOutput confirms an empty store is not an
// error case: BuildReport returns no weeks and FormatReport renders nothing.
func TestReportEmptyStoreProducesEmptyOutput(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "report")
	if err != nil {
		t.Fatalf("work report on empty store: %v", err)
	}
	if out != "" {
		t.Errorf("report on empty store = %q, want empty string", out)
	}
}

// TestReportRangedCreditsPortionOfSessionCrossingBoundary is task 281's
// regression guardrail: a session that logs in before a ranged report's
// Since boundary and logs out inside it must not abort the report. Before
// the fix, worktime.Query's time filter dropped the pre-boundary login but
// kept the in-range logout, and BuildReport's applyAction hard-errored with
// "logout without login". The session here spans the 2025-12-31/2026-01-01
// boundary (22:00 -> 02:00, 4h total) so `work report 2026-01` exercises the
// exact straddling case, deterministically (no dependency on the real
// wall-clock "today").
func TestReportRangedCreditsPortionOfSessionCrossingBoundary(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "start", "work", "--at", "2025-12-31T22:00"); err != nil {
		t.Fatalf("work start: %v", err)
	}
	if _, err := runWork(t, store, "stop", "work", "--at", "2026-01-01T02:00"); err != nil {
		t.Fatalf("work stop: %v", err)
	}

	out, err := runWork(t, store, "report", "2026-01")
	if err != nil {
		t.Fatalf("work report 2026-01: %v", err)
	}
	// Only the in-range slice (2026-01-01T00:00 through 02:00 = 2h) must be
	// credited, not the session's full 4h span -- crediting the whole span
	// would double-count hours that belong to December.
	if !strings.Contains(out, "work:2.00h") {
		t.Errorf("expected work:2.00h (in-range portion only), got:\n%s", out)
	}
	// The synthetic boundary login must not leak an out-of-range December
	// day line into a January-only report.
	if strings.Contains(out, "20251231") {
		t.Errorf("out-of-range day 20251231 must not appear in a Jan-only report, got:\n%s", out)
	}
}

// TestReportFullHistoryStillCreditsEntireStraddlingSession confirms the fix
// is scoped to the ranged path: a no-args (full-history) report over the
// same straddling session from the test above must still credit the whole
// 4h span, since nothing is being filtered out there.
func TestReportFullHistoryStillCreditsEntireStraddlingSession(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "start", "work", "--at", "2025-12-31T22:00"); err != nil {
		t.Fatalf("work start: %v", err)
	}
	if _, err := runWork(t, store, "stop", "work", "--at", "2026-01-01T02:00"); err != nil {
		t.Fatalf("work stop: %v", err)
	}

	out, err := runWork(t, store, "report")
	if err != nil {
		t.Fatalf("work report: %v", err)
	}
	if !strings.Contains(out, "work:4.00h") {
		t.Errorf("expected work:4.00h (full session, no range filtering), got:\n%s", out)
	}
}

// TestReportTooManyArgsFails confirms report's positional argument is capped
// at one (cobra.MaximumNArgs(1)), matching list/search's "0 or 1 range" shape.
func TestReportTooManyArgsFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "report", "2026-01", "extra"); err == nil {
		t.Fatal("report with two positional args: want error, got nil")
	}
}
