package worktime_test

import (
	"testing"
	"time"

	"codeberg.org/snonux/timesamurai/internal/worktime"
)

func TestAddAndLoadAllPublicAPI(t *testing.T) {
	dbDir := t.TempDir()
	host := "host-a"

	_, err := worktime.Add(dbDir, host, "work", 30*time.Minute, time.Unix(100, 0), "public api")
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	entries, err := worktime.LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Action != "add" || entries[0].What != "work" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}
