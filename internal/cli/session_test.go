package cli

import (
	"strings"
	"testing"
	"time"
)

func TestStartWritesLoginEntryWithDefaultTag(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("work start: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Action != "login" {
		t.Errorf("action = %q, want login", entries[0].Action)
	}
	if got := strings.Join(entries[0].Tags, ","); got != "work" {
		t.Errorf("tags = %q, want default \"work\"", got)
	}
}

func TestStartParsesBareWordsAsTags(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "start", "pet", "vet"); err != nil {
		t.Fatalf("work start pet vet: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	want := []string{"pet", "vet"}
	got := entries[0].Tags
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("tags = %v, want %v", got, want)
	}
}

func TestStartThenStopClosesSession(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("work start: %v", err)
	}
	if _, err := runWork(t, store, "stop"); err != nil {
		t.Fatalf("work stop: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 2 || entries[0].Action != "login" || entries[1].Action != "logout" {
		t.Fatalf("want [login logout], got %+v", entries)
	}
}

func TestStartTwiceSameCategoryFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "start"); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, err := runWork(t, store, "start"); err == nil {
		t.Fatal("second start on the same open category: want error, got nil")
	}
}

func TestStopWithoutLoginFails(t *testing.T) {
	store := newScratchStore(t)

	if _, err := runWork(t, store, "stop"); err == nil {
		t.Fatal("stop with no open login: want error, got nil")
	}
}

func TestLoginLogoutAreHiddenAliasesOfStartStop(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	if _, err := runWork(t, store, "login"); err != nil {
		t.Fatalf("work login: %v", err)
	}
	if _, err := runWork(t, store, "logout"); err != nil {
		t.Fatalf("work logout: %v", err)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 2 || entries[0].Action != "login" || entries[1].Action != "logout" {
		t.Fatalf("login/logout should behave exactly like start/stop, got %+v", entries)
	}
}

func TestLoginLogoutHiddenFromHelp(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "--help")
	if err != nil {
		t.Fatalf("work --help: %v", err)
	}
	if strings.Contains(out, "login") || strings.Contains(out, "logout") {
		t.Errorf("--help output must not mention hidden login/logout aliases, got:\n%s", out)
	}
}

func TestStatusShowsOpenSessionThenClears(t *testing.T) {
	store := newScratchStore(t)

	out, err := runWork(t, store, "status")
	if err != nil {
		t.Fatalf("work status (no sessions): %v", err)
	}
	if !strings.Contains(out, "no open sessions") {
		t.Errorf("status with nothing open: got %q", out)
	}

	if _, err := runWork(t, store, "start", "pet"); err != nil {
		t.Fatalf("work start pet: %v", err)
	}
	out, err = runWork(t, store, "status")
	if err != nil {
		t.Fatalf("work status (open): %v", err)
	}
	if !strings.Contains(out, "pet") {
		t.Errorf("status should report the open \"pet\" category, got %q", out)
	}

	if _, err := runWork(t, store, "stop", "pet"); err != nil {
		t.Fatalf("work stop pet: %v", err)
	}
	out, err = runWork(t, store, "status")
	if err != nil {
		t.Fatalf("work status (closed): %v", err)
	}
	if !strings.Contains(out, "no open sessions") {
		t.Errorf("status after stop: got %q", out)
	}
}

func TestStartAtDescrVerboseFlags(t *testing.T) {
	store := newScratchStore(t)
	host := currentHost(t)

	out, err := runWork(t, store, "start", "--at", "2026-01-02T09:00", "--descr", "morning standup", "--verbose")
	if err != nil {
		t.Fatalf("work start --at --descr --verbose: %v", err)
	}
	if !strings.Contains(out, `descr="morning standup"`) {
		t.Errorf("--verbose output should include the description, got %q", out)
	}

	entries := readEntries(t, store, host)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Descr != "morning standup" {
		t.Errorf("descr = %q, want %q", entries[0].Descr, "morning standup")
	}
	want, err := time.ParseInLocation("2006-01-02T15:04", "2026-01-02T09:00", time.Local)
	if err != nil {
		t.Fatalf("time.ParseInLocation: %v", err)
	}
	if entries[0].Epoch != want.Unix() {
		t.Errorf("epoch = %d, want %d (--at 2026-01-02T09:00)", entries[0].Epoch, want.Unix())
	}
}
