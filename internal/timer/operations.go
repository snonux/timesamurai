package timer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Tracker abstracts timer-to-task tracking so TrackTime can be unit-tested.
type Tracker interface {
	Track(description string, minutes int) error
}

const trackCommandTimeout = 10 * time.Second

type taskwarriorTracker struct{}

func (taskwarriorTracker) Track(description string, minutes int) error {
	taskDescription := fmt.Sprintf("%dmin %s", minutes, description)
	ctx, cancel := context.WithTimeout(context.Background(), trackCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "task", "add", "+track", taskDescription)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("task command timed out after %s", trackCommandTimeout)
		}
		return fmt.Errorf("task command failed: %s\nOutput: %s", err, string(output))
	}

	return nil
}

func StartTimer(continued bool) (string, error) {
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	if state.Running {
		return "Timer is already running.", nil
	}

	state.Running = true
	state.StartTime = time.Now()
	if err := state.Save(); err != nil {
		return "", fmt.Errorf("error saving state: %w", err)
	}

	if continued {
		return "Timer continued.", nil
	}

	return "Timer started.", nil
}

func StopTimer() (string, error) {
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	if !state.Running {
		return "Timer is not running.", nil
	}

	state.Running = false
	state.ElapsedTime += time.Since(state.StartTime)
	if err := state.Save(); err != nil {
		return "", fmt.Errorf("error saving state: %w", err)
	}
	return "Timer stopped.", nil
}

func getElapsed() (state State, elapsed time.Duration, err error) {
	state, err = LoadState()
	if err != nil {
		return state, 0, fmt.Errorf("error loading state: %w", err)
	}

	elapsed = state.ElapsedTime
	if state.Running {
		elapsed += time.Since(state.StartTime)
	}

	return state, elapsed, nil
}

func GetStatus() (string, error) {
	state, elapsed, err := getElapsed()
	if err != nil {
		return "", err
	}

	if state.Running {
		return fmt.Sprintf("Status: Running\nElapsed Time: %s", elapsed.Round(time.Second)), nil
	}

	return fmt.Sprintf("Status: Stopped\nElapsed Time: %s", elapsed.Round(time.Second)), nil
}

func ResetTimer() (string, error) {
	stateFile, err := GetStateFile()
	if err != nil {
		return "", fmt.Errorf("error getting state file path: %w", err)
	}
	if err := os.Remove(stateFile); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("error resetting timer: %w", err)
		}
	}
	state := State{}
	if err := state.Save(); err != nil {
		return "", fmt.Errorf("error saving state: %w", err)
	}
	return "Timer reset.", nil
}

func GetRawStatus() (string, error) {
	_, elapsed, err := getElapsed()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%f", elapsed.Seconds()), nil
}

func GetRawMinutesStatus() (string, error) {
	_, elapsed, err := getElapsed()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d", int(elapsed.Minutes())), nil
}

func GetPromptStatus() (string, error) {
	state, elapsed, err := getElapsed()
	if err != nil {
		return "", err
	}

	if elapsed == 0 {
		return "", nil
	}

	icon := "⏸"
	if state.Running {
		icon = "▶"
	}

	return fmt.Sprintf("%s%s", icon, elapsed.Round(time.Second)), nil
}

func TrackTime(description string) (string, error) {
	return TrackTimeWithTracker(description, taskwarriorTracker{})
}

func TrackTimeWithTracker(description string, tracker Tracker) (string, error) {
	if tracker == nil {
		return "", errors.New("tracker is nil")
	}

	// Load current state
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	// Calculate total elapsed time
	elapsed := state.ElapsedTime
	if state.Running {
		elapsed += time.Since(state.StartTime)
	}

	// Convert to minutes
	minutes := int(elapsed.Minutes())

	// If timer was running, stop it
	if state.Running {
		state.Running = false
		state.ElapsedTime = elapsed
		if err := state.Save(); err != nil {
			return "", fmt.Errorf("error saving state after stopping: %w", err)
		}
	}

	if err := tracker.Track(description, minutes); err != nil {
		return "", err
	}

	// Command succeeded, reset the timer
	if _, err := ResetTimer(); err != nil {
		return "", fmt.Errorf("tracked time successfully but failed to reset timer: %w", err)
	}

	return fmt.Sprintf("Tracked %d minutes: %s\nTimer reset.", minutes, description), nil
}
