package cli

import (
	"strings"
	"testing"
	"time"
)

func TestAddCreditsDurationWithDefaultTag(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "2h"); err != nil {
		t.Fatalf("work add 2h: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "add" || entries[0].Value != 7200 {
		t.Errorf("entry = %+v, want add value=7200", entries[0])
	}
	if got := strings.Join(entries[0].Tags, ","); got != "work" {
		t.Errorf("tags = %q, want default \"work\"", got)
	}
}

func TestAddParsesTagsAfterDuration(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "add", "30m", "selfdevelopment", "blogpost"); err != nil {
		t.Fatalf("work add 30m selfdevelopment blogpost: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Value != 1800 {
		t.Errorf("value = %d, want 1800", entries[0].Value)
	}
	want := []string{"selfdevelopment", "blogpost"}
	got := entries[0].Tags
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

func TestAddRejectsTwoAccountingTags(t *testing.T) {
	store := newScratchStore(t)

	// "off" and "bank" are both plusfor categories by default, so a second
	// accounting tag on one entry must be rejected (worktime.ValidateEntry /
	// AccountingTag), not silently accepted.
	if _, err := runWork(t, store, "add", "2h", "off", "bank"); err == nil {
		t.Fatal("add 2h off bank: want error (two accounting tags), got nil")
	}
}

func TestSubRecordsNegativeValue(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "sub", "1h", "lunch"); err != nil {
		t.Fatalf("work sub 1h lunch: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Value != -3600 {
		t.Errorf("value = %d, want -3600", entries[0].Value)
	}
}

func TestAddMissingDurationFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add"); err == nil {
		t.Fatal("add with no duration arg: want error, got nil")
	}
}

func TestAddInvalidDurationFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "add", "not-a-duration"); err == nil {
		t.Fatal("add not-a-duration: want error, got nil")
	}
}

func TestUseBufferMovesSelfdevelopmentIntoWork(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "usebuffer", "30m"); err != nil {
		t.Fatalf("work usebuffer 30m: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (withdraw+credit), got %d: %+v", len(entries), entries)
	}
	withdraw, credit := entries[0], entries[1]
	if withdraw.Value != -1800 || strings.Join(withdraw.Tags, ",") != "selfdevelopment" {
		t.Errorf("withdraw entry = %+v, want value=-1800 tags=[selfdevelopment]", withdraw)
	}
	if credit.Value != 1800 || strings.Join(credit.Tags, ",") != "work" {
		t.Errorf("credit entry = %+v, want value=1800 tags=[work]", credit)
	}
}

func TestUseBufferWrongArgCountFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "usebuffer"); err == nil {
		t.Fatal("usebuffer with no duration: want error, got nil")
	}
	if _, err := runWork(t, store, "usebuffer", "30m", "extra"); err == nil {
		t.Fatal("usebuffer with an extra positional arg: want error, got nil")
	}
}

func TestDayOffCreditsEightHoursAgainstOffTag(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "day-off"); err != nil {
		t.Fatalf("work day-off: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Value != 8*3600 {
		t.Errorf("value = %d, want %d (8h)", entries[0].Value, 8*3600)
	}
	if got := strings.Join(entries[0].Tags, ","); got != "off" {
		t.Errorf("tags = %q, want \"off\"", got)
	}

	// day-off always normalizes to the start of the day, regardless of the
	// time-of-day the command happened to run at.
	at := time.Unix(entries[0].Epoch, 0)
	if h, m, s := at.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("day-off epoch %v is not midnight", at)
	}
}

func TestDayOffAcceptsExtraLabelTags(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "day-off", "vacation"); err != nil {
		t.Fatalf("work day-off vacation: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	want := []string{"off", "vacation"}
	got := entries[0].Tags
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

func TestDayOffAtNormalizesToStartOfThatDay(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "day-off", "--at", "2026-03-15T14:30"); err != nil {
		t.Fatalf("work day-off --at: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	wantDay, err := time.ParseInLocation("2006-01-02", "2026-03-15", time.Local)
	if err != nil {
		t.Fatalf("time.ParseInLocation: %v", err)
	}
	if entries[0].Epoch != wantDay.Unix() {
		t.Errorf("epoch = %d, want %d (midnight of 2026-03-15)", entries[0].Epoch, wantDay.Unix())
	}
}
