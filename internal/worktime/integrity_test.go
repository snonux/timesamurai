package worktime

import (
	"testing"
	"time"
)

func TestCheckEntriesIntegrityDetectsIssues(t *testing.T) {
	entries := []Entry{
		{Action: "login", What: "work", Epoch: 100, Source: "host-a"},
		{Action: "login", What: "work", Epoch: 110, Source: "host-a"},  // double login
		{Action: "logout", What: "work", Epoch: 120, Source: "host-a"}, // matches first login
		{Action: "logout", What: "work", Epoch: 130, Source: "host-a"}, // logout without login
		{Action: "login", What: "off", Epoch: 200, Source: "host-b"},   // open session warning
		{Action: "login", What: "bank", Epoch: 300, Source: "host-c"},
		{Action: "logout", What: "bank", Epoch: 300 + int64((16*time.Hour)/time.Second), Source: "host-c"}, // too long
	}

	issues := CheckEntriesIntegrity(entries, DefaultMaxSessionSpan)
	if len(issues) != 3 {
		t.Fatalf("issues len = %d, want 3", len(issues))
	}

	kinds := map[string]bool{}
	for _, issue := range issues {
		kinds[issue.Kind] = true
	}
	for _, kind := range []string{
		"double-login",
		"logout-without-login",
		"session-too-long",
	} {
		if !kinds[kind] {
			t.Fatalf("missing expected issue kind %q", kind)
		}
	}
}

func TestOpenSessionsReturnsActiveLogins(t *testing.T) {
	entries := []Entry{
		{Action: "login", What: "work", Epoch: 100, Source: "host-a"},
		{Action: "logout", What: "work", Epoch: 200, Source: "host-a"},
		{Action: "login", What: "off", Epoch: 300, Source: "host-b"},
		{Action: "login", What: "bank", Epoch: 400, Source: "host-c"},
	}

	sessions := OpenSessions(entries)
	if len(sessions) != 2 {
		t.Fatalf("sessions len = %d, want 2", len(sessions))
	}

	if sessions[0].Category != "bank" || sessions[0].Login.Source != "host-c" {
		t.Fatalf("unexpected first session: %+v", sessions[0])
	}
	if sessions[1].Category != "off" || sessions[1].Login.Source != "host-b" {
		t.Fatalf("unexpected second session: %+v", sessions[1])
	}
}

func TestCheckEntriesIntegrityPassesValidFlow(t *testing.T) {
	entries := []Entry{
		{Action: "login", What: "work", Epoch: 100, Source: "host-a"},
		{Action: "logout", What: "work", Epoch: 200, Source: "host-a"},
		{Action: "add", What: "off", Epoch: 300, Source: "host-a", Value: 8 * 3600},
	}

	issues := CheckEntriesIntegrity(entries, DefaultMaxSessionSpan)
	if len(issues) != 0 {
		t.Fatalf("issues len = %d, want 0 (%v)", len(issues), issues)
	}
}

func TestCheckIntegrityValidation(t *testing.T) {
	if _, err := CheckIntegrity(t.TempDir(), 0); err == nil {
		t.Fatal("CheckIntegrity() with non-positive max span should fail")
	}
}
