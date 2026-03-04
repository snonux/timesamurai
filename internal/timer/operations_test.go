package timer

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

// setup sets up a temporary state file for testing.
func setup(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("HOME", tempDir)
}

type mockTracker struct {
	description string
	minutes     int
	err         error
	calls       int
}

func (m *mockTracker) Track(description string, minutes int) error {
	m.description = description
	m.minutes = minutes
	m.calls++
	return m.err
}

func TestStartTimer(t *testing.T) {
	setup(t)

	// Start the timer
	msg, err := StartTimer(false)
	if err != nil {
		t.Fatalf("StartTimer(false) error = %v", err)
	}
	if msg != "Timer started." {
		t.Errorf("StartTimer(false) msg = %v, want %v", msg, "Timer started.")
	}

	// Check the state
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if !state.Running {
		t.Error("state.Running = false, want true")
	}

	// Try to start again
	msg, err = StartTimer(false)
	if err != nil {
		t.Fatalf("StartTimer(false) error = %v", err)
	}
	if msg != "Timer is already running." {
		t.Errorf("StartTimer(false) msg = %v, want %v", msg, "Timer is already running.")
	}
}

func TestStopTimer(t *testing.T) {
	setup(t)

	// Stop before starting
	msg, err := StopTimer()
	if err != nil {
		t.Fatalf("StopTimer() error = %v", err)
	}
	if msg != "Timer is not running." {
		t.Errorf("StopTimer() msg = %v, want %v", msg, "Timer is not running.")
	}

	// Start and then stop the timer
	_, _ = StartTimer(false)
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() after StartTimer error = %v", err)
	}
	state.StartTime = time.Now().Add(-10 * time.Millisecond)
	if err := state.Save(); err != nil {
		t.Fatalf("Save() after StartTimer error = %v", err)
	}
	msg, err = StopTimer()
	if err != nil {
		t.Fatalf("StopTimer() error = %v", err)
	}
	if msg != "Timer stopped." {
		t.Errorf("StopTimer() msg = %v, want %v", msg, "Timer stopped.")
	}

	// Check the state
	state, err = LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Running {
		t.Error("state.Running = true, want false")
	}
	if state.ElapsedTime == 0 {
		t.Error("state.ElapsedTime = 0, want > 0")
	}
}

func TestGetStatus(t *testing.T) {
	setup(t)

	// Status when stopped
	msg, err := GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	want := "Status: Stopped\nElapsed Time: 0s"
	if msg != want {
		t.Errorf("GetStatus() msg = %q, want %q", msg, want)
	}

	// Status when running
	_, _ = StartTimer(false)
	msg, err = GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	want = "Status: Running\nElapsed Time: 0s"
	if msg != want {
		t.Errorf("GetStatus() msg = %q, want %q", msg, want)
	}
}

func TestGetRawStatus(t *testing.T) {
	setup(t)

	// Raw status when stopped
	msg, err := GetRawStatus()
	if err != nil {
		t.Fatalf("GetRawStatus() error = %v", err)
	}
	want := "0.000000"
	if msg != want {
		t.Errorf("GetRawStatus() msg = %q, want %q", msg, want)
	}

	// Raw status when running
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	state.Running = true
	state.StartTime = time.Now().Add(-2 * time.Second) // Set start time 2 seconds ago
	state.ElapsedTime = 0                              // Reset elapsed time for this specific test
	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	msg, err = GetRawStatus()
	if err != nil {
		t.Fatalf("GetRawStatus() error = %v", err)
	}
	// we can't know the exact float, so we check if it's close to 2
	val, err := strconv.ParseFloat(msg, 64)
	if err != nil {
		t.Fatalf("could not parse float: %v", err)
	}
	if val < 1.9 || val > 2.1 {
		t.Errorf("GetRawStatus() msg = %q, want around 2.0", msg)
	}
}

func TestGetRawMinutesStatus(t *testing.T) {
	setup(t)

	// Raw minutes status when stopped
	msg, err := GetRawMinutesStatus()
	if err != nil {
		t.Fatalf("GetRawMinutesStatus() error = %v", err)
	}
	want := "0"
	if msg != want {
		t.Errorf("GetRawMinutesStatus() msg = %q, want %q", msg, want)
	}

	// Raw minutes status when running (simulating 2 minutes)
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	state.Running = true
	state.StartTime = time.Now().Add(-2 * time.Minute) // Set start time 2 minutes ago
	state.ElapsedTime = 0                              // Reset elapsed time for this specific test
	if err := state.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	msg, err = GetRawMinutesStatus()
	if err != nil {
		t.Fatalf("GetRawMinutesStatus() error = %v", err)
	}
	want = "2"
	if msg != want {
		t.Errorf("GetRawMinutesStatus() msg = %q, want %q", msg, want)
	}
}

func TestResetTimer(t *testing.T) {
	setup(t)

	// Start timer to create a state file
	_, _ = StartTimer(false)

	// Reset the timer
	msg, err := ResetTimer()
	if err != nil {
		t.Fatalf("ResetTimer() error = %v", err)
	}
	if msg != "Timer reset." {
		t.Errorf("ResetTimer() msg = %v, want %v", msg, "Timer reset.")
	}

	// Check the state
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state.Running {
		t.Error("state.Running = true, want false")
	}
	if state.ElapsedTime != 0 {
		t.Errorf("state.ElapsedTime = %v, want 0", state.ElapsedTime)
	}
}

func TestTrackTime(t *testing.T) {
	setup(t)

	t.Run("TrackWithRunningTimer", func(t *testing.T) {
		setup(t)
		tracker := &mockTracker{}

		// Start timer and let it run for a bit
		state, _ := LoadState()
		state.Running = true
		state.StartTime = time.Now().Add(-5 * time.Minute)
		state.ElapsedTime = 0
		state.Save()

		msg, err := TrackTimeWithTracker("test description", tracker)
		if err != nil {
			t.Fatalf("TrackTimeWithTracker() error = %v", err)
		}
		if msg == "" {
			t.Fatal("TrackTimeWithTracker() msg is empty")
		}
		if tracker.calls != 1 {
			t.Fatalf("tracker calls = %d, want 1", tracker.calls)
		}
		if tracker.minutes < 4 || tracker.minutes > 6 {
			t.Fatalf("tracker minutes = %d, want around 5", tracker.minutes)
		}

		state, _ = LoadState()
		if state.Running || state.ElapsedTime != 0 {
			t.Fatalf("state after tracking = %+v, want reset stopped state", state)
		}
	})

	t.Run("TrackFailureKeepsElapsedState", func(t *testing.T) {
		setup(t)
		tracker := &mockTracker{err: errTestTracker}

		// Set up a stopped timer with some elapsed time
		state, _ := LoadState()
		state.Running = false
		state.ElapsedTime = 10 * time.Minute
		state.Save()

		_, err := TrackTimeWithTracker("another test", tracker)
		if err == nil {
			t.Fatal("TrackTimeWithTracker() error = nil, want tracker error")
		}

		state, _ = LoadState()
		if state.Running {
			t.Fatal("state.Running = true, want false after failed tracking")
		}
		if state.ElapsedTime != 10*time.Minute {
			t.Fatalf("state.ElapsedTime = %v, want %v", state.ElapsedTime, 10*time.Minute)
		}
	})

	t.Run("NilTracker", func(t *testing.T) {
		setup(t)
		_, err := TrackTimeWithTracker("zero time test", nil)
		if err == nil {
			t.Fatal("TrackTimeWithTracker() error = nil, want error")
		}
	})
}

var errTestTracker = errors.New("tracker failure")
