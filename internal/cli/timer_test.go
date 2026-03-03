package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	timrTimer "codeberg.org/snonux/timr/internal/timer"
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

func setupTimerState(t *testing.T) {
	t.Helper()

	tempDir := t.TempDir()
	timrTimer.SetStateFilePathOverride(filepath.Join(tempDir, ".timr_state"))
	t.Cleanup(func() {
		timrTimer.SetStateFilePathOverride("")
	})
}
