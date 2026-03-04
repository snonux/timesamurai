package worktime

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestLoginLogoutValidation(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	loginEntry, err := Login(dbDir, host, "work", time.Unix(100, 0), "start")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginEntry.Action != "login" || loginEntry.What != "work" {
		t.Fatalf("unexpected login entry: %+v", loginEntry)
	}

	if _, err := Login(dbDir, host, "work", time.Unix(110, 0), "start again"); err == nil {
		t.Fatal("Login() error = nil, want already logged in error")
	} else if !errors.Is(err, ErrAlreadyLoggedIn) {
		t.Fatalf("Login() error = %v, want ErrAlreadyLoggedIn", err)
	}

	logoutEntry, err := Logout(dbDir, host, "work", time.Unix(120, 0), "stop")
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if logoutEntry.Action != "logout" || logoutEntry.What != "work" {
		t.Fatalf("unexpected logout entry: %+v", logoutEntry)
	}

	if _, err := Logout(dbDir, host, "work", time.Unix(130, 0), "stop again"); err == nil {
		t.Fatal("Logout() error = nil, want not logged in error")
	} else if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("Logout() error = %v, want ErrNotLoggedIn", err)
	}
}

func TestLoginValidationIsCategoryScoped(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	if _, err := Login(dbDir, host, "work", time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Login(work) error = %v", err)
	}
	if _, err := Login(dbDir, host, "lunch", time.Unix(110, 0), ""); err != nil {
		t.Fatalf("Login(lunch) error = %v", err)
	}
}

func TestActiveCategories(t *testing.T) {
	entries := []Entry{
		{Action: "login", What: "work"},
		{Action: "login", What: "lunch"},
		{Action: "logout", What: "lunch"},
		{Action: "login", What: ""},
	}

	got := ActiveCategories(entries)
	want := []string{"work"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveCategories() = %v, want %v", got, want)
	}
}

func TestAddSubAndUseBuffer(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	added, err := Add(dbDir, host, "", 30*time.Minute, time.Unix(100, 0), "manual add")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if added.Action != "add" || added.What != "work" || added.Value != 1800 {
		t.Fatalf("unexpected Add() entry: %+v", added)
	}

	subbed, err := Sub(dbDir, host, "work", 15*time.Minute, time.Unix(200, 0), "manual sub")
	if err != nil {
		t.Fatalf("Sub() error = %v", err)
	}
	if subbed.Value != -900 {
		t.Fatalf("Sub() value = %d, want -900", subbed.Value)
	}

	bufferEntries, err := UseBuffer(dbDir, host, 10*time.Minute, time.Unix(300, 0), "buffer transfer")
	if err != nil {
		t.Fatalf("UseBuffer() error = %v", err)
	}
	if len(bufferEntries) != 2 {
		t.Fatalf("UseBuffer() len = %d, want 2", len(bufferEntries))
	}
	if bufferEntries[0].What != "selfdevelopment" || bufferEntries[0].Value != -600 {
		t.Fatalf("unexpected buffer remove entry: %+v", bufferEntries[0])
	}
	if bufferEntries[1].What != "work" || bufferEntries[1].Value != 600 {
		t.Fatalf("unexpected buffer add entry: %+v", bufferEntries[1])
	}

	db, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() error = %v", err)
	}

	entries := db.Entries[host]
	if len(entries) != 4 {
		t.Fatalf("entries len = %d, want 4", len(entries))
	}
	if entries[0].Epoch != 100 || entries[1].Epoch != 200 || entries[2].Epoch != 300 || entries[3].Epoch != 300 {
		t.Fatalf("entries not sorted by epoch: %+v", entries)
	}
}

func TestDurationValidation(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "add zero duration",
			run: func() error {
				_, err := Add(dbDir, host, "work", 0, time.Unix(100, 0), "")
				return err
			},
		},
		{
			name: "add negative duration",
			run: func() error {
				_, err := Add(dbDir, host, "work", -time.Minute, time.Unix(100, 0), "")
				return err
			},
		},
		{
			name: "sub zero duration",
			run: func() error {
				_, err := Sub(dbDir, host, "work", 0, time.Unix(100, 0), "")
				return err
			},
		},
		{
			name: "use buffer zero duration",
			run: func() error {
				_, err := UseBuffer(dbDir, host, 0, time.Unix(100, 0), "")
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatalf("%s: error = nil, want validation error", test.name)
			}
		})
	}
}

func TestEditAndDeleteEntry(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	if _, err := Add(dbDir, host, "work", 5*time.Minute, time.Unix(100, 0), "first"); err != nil {
		t.Fatalf("Add(first) error = %v", err)
	}
	if _, err := Add(dbDir, host, "work", 6*time.Minute, time.Unix(200, 0), "second"); err != nil {
		t.Fatalf("Add(second) error = %v", err)
	}

	edited, err := EditEntry(dbDir, host, 0, Entry{
		Action: "ADD",
		What:   "off",
		Epoch:  100,
		Value:  120,
		Descr:  "updated",
	})
	if err != nil {
		t.Fatalf("EditEntry() error = %v", err)
	}
	if edited.Action != "add" || edited.What != "off" || edited.Source != host || edited.Value != 120 {
		t.Fatalf("unexpected edited entry: %+v", edited)
	}

	dbAfterEdit, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() after edit error = %v", err)
	}
	if dbAfterEdit.Entries[host][0].What != "off" {
		t.Fatalf("entry was not edited: %+v", dbAfterEdit.Entries[host][0])
	}

	removed, err := DeleteEntry(dbDir, host, 1)
	if err != nil {
		t.Fatalf("DeleteEntry() error = %v", err)
	}
	if removed.Descr != "second" {
		t.Fatalf("unexpected removed entry: %+v", removed)
	}

	dbAfterDelete, err := LoadHost(dbDir, host)
	if err != nil {
		t.Fatalf("LoadHost() after delete error = %v", err)
	}
	if len(dbAfterDelete.Entries[host]) != 1 {
		t.Fatalf("entries len after delete = %d, want 1", len(dbAfterDelete.Entries[host]))
	}

	if _, err := EditEntry(dbDir, host, 5, Entry{Action: "add", Epoch: 1}); err == nil {
		t.Fatal("EditEntry() accepted out-of-range index")
	}
	if _, err := DeleteEntry(dbDir, host, 5); err == nil {
		t.Fatal("DeleteEntry() accepted out-of-range index")
	}
}

func TestEditEntryValidation(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	if _, err := Add(dbDir, host, "work", time.Minute, time.Unix(100, 0), "seed"); err != nil {
		t.Fatalf("Add(seed) error = %v", err)
	}

	tests := []struct {
		name        string
		replacement Entry
	}{
		{
			name:        "unsupported action",
			replacement: Entry{Action: "bad", Epoch: 1},
		},
		{
			name:        "non-positive epoch",
			replacement: Entry{Action: "add", Epoch: 0},
		},
		{
			name:        "mismatched source",
			replacement: Entry{Action: "add", Epoch: 1, Source: "other-host"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if _, err := EditEntry(dbDir, host, 0, test.replacement); err == nil {
				t.Fatalf("EditEntry() accepted invalid replacement: %s", test.name)
			}
		})
	}
}

func TestGlobalLoginValidationAcrossHosts(t *testing.T) {
	dbDir := t.TempDir()

	if _, err := Login(dbDir, "host-a", "work", time.Unix(100, 0), ""); err != nil {
		t.Fatalf("Login(host-a) error = %v", err)
	}

	if _, err := Login(dbDir, "host-b", "work", time.Unix(110, 0), ""); err == nil {
		t.Fatal("Login(host-b) should fail while work is already logged in")
	}

	if _, err := Logout(dbDir, "host-b", "work", time.Unix(120, 0), ""); err != nil {
		t.Fatalf("Logout(host-b) error = %v", err)
	}

	if _, err := Logout(dbDir, "host-a", "work", time.Unix(130, 0), ""); err == nil {
		t.Fatal("Logout(host-a) should fail because work is already logged out")
	}
}
