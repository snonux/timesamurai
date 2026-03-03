package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	timrTimer "codeberg.org/snonux/timr/internal/timer"
	"codeberg.org/snonux/timr/internal/worktime"
)

func TestTimerStartAndStopCommands(t *testing.T) {
	setupTimerState(t)

	var startOut bytes.Buffer
	startCmd := NewRootCmd()
	startCmd.SetOut(&startOut)
	startCmd.SetErr(&startOut)
	startCmd.SetArgs([]string{"timer", "start"})
	if err := startCmd.Execute(); err != nil {
		t.Fatalf("timer start execute error = %v", err)
	}
	if !strings.Contains(startOut.String(), "Timer started.") {
		t.Fatalf("timer start output = %q", startOut.String())
	}

	var stopOut bytes.Buffer
	stopCmd := NewRootCmd()
	stopCmd.SetOut(&stopOut)
	stopCmd.SetErr(&stopOut)
	stopCmd.SetArgs([]string{"timer", "stop"})
	if err := stopCmd.Execute(); err != nil {
		t.Fatalf("timer stop execute error = %v", err)
	}
	if !strings.Contains(stopOut.String(), "Timer stopped.") {
		t.Fatalf("timer stop output = %q", stopOut.String())
	}
}

func TestTimerContinueAtZero(t *testing.T) {
	setupTimerState(t)

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"timer", "continue"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("timer continue execute error = %v", err)
	}

	if !strings.Contains(out.String(), "Timer is at 0, cannot continue.") {
		t.Fatalf("timer continue output = %q", out.String())
	}
}

func TestTimerStatusFlagConflict(t *testing.T) {
	setupTimerState(t)

	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"timer", "status", "--raw", "--raw-minutes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("timer status conflict error = nil, want error")
	}
	if !strings.Contains(err.Error(), "--raw") {
		t.Fatalf("timer status conflict error = %v", err)
	}
}

func TestTimerAutoWorktimeSync(t *testing.T) {
	setupTimerState(t)

	dbDir := t.TempDir()
	cfgPath := writeWorkConfigWithAuto(t, dbDir, "host-auto", true)

	out, err := runRootCommand("--config", cfgPath, "timer", "start")
	if err != nil {
		t.Fatalf("timer start error = %v (output: %q)", err, out)
	}
	out, err = runRootCommand("--config", cfgPath, "timer", "stop")
	if err != nil {
		t.Fatalf("timer stop error = %v (output: %q)", err, out)
	}

	entries, err := worktime.LoadAll(dbDir)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	var hasLogin bool
	var hasLogout bool
	for _, entry := range entries {
		if entry.What != "work" {
			continue
		}
		if entry.Action == "login" {
			hasLogin = true
		}
		if entry.Action == "logout" {
			hasLogout = true
		}
	}

	if !hasLogin || !hasLogout {
		t.Fatalf("auto worktime sync missing login/logout entries: %+v", entries)
	}
}

func TestTimerAutoWorktimeSyncIgnoresAlreadyLoggedIn(t *testing.T) {
	setupTimerState(t)

	dbDir := t.TempDir()
	if _, err := worktime.Login(dbDir, "host-auto", "work", time.Unix(100, 0), "seed"); err != nil {
		t.Fatalf("seed Login() error = %v", err)
	}

	cfgPath := writeWorkConfigWithAuto(t, dbDir, "host-auto", true)
	out, err := runRootCommand("--config", cfgPath, "timer", "start")
	if err != nil {
		t.Fatalf("timer start error = %v (output: %q)", err, out)
	}
}

func TestTimerAutoWorktimeSyncIgnoresNotLoggedInOnStop(t *testing.T) {
	setupTimerState(t)

	dbDir := t.TempDir()
	cfgPath := writeWorkConfigWithAuto(t, dbDir, "host-auto", true)
	out, err := runRootCommand("--config", cfgPath, "timer", "stop")
	if err != nil {
		t.Fatalf("timer stop error = %v (output: %q)", err, out)
	}
}

func setupTimerState(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	timrTimer.SetStateFilePathOverride(filepath.Join(tempDir, ".timr_state"))
	t.Cleanup(func() {
		timrTimer.SetStateFilePathOverride("")
	})
}
