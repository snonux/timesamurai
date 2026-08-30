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

// TestReportTooManyArgsFails confirms report's positional argument is capped
// at one (cobra.MaximumNArgs(1)), matching list/search's "0 or 1 range" shape.
func TestReportTooManyArgsFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "report", "2026-01", "extra"); err == nil {
		t.Fatal("report with two positional args: want error, got nil")
	}
}
