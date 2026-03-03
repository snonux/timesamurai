package timer

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

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

func GetStatus() (string, error) {
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	if state.Running {
		elapsed := (state.ElapsedTime + time.Since(state.StartTime)).Round(time.Second)
		return fmt.Sprintf("Status: Running\nElapsed Time: %s", elapsed), nil
	}

	elapsed := state.ElapsedTime.Round(time.Second)
	return fmt.Sprintf("Status: Stopped\nElapsed Time: %s", elapsed), nil
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
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	elapsed := state.ElapsedTime
	if state.Running {
		elapsed += time.Since(state.StartTime)
	}

	return fmt.Sprintf("%f", elapsed.Seconds()), nil
}

func GetRawMinutesStatus() (string, error) {
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	elapsed := state.ElapsedTime
	if state.Running {
		elapsed += time.Since(state.StartTime)
	}

	return fmt.Sprintf("%d", int(elapsed.Minutes())), nil
}

func GetPromptStatus() (string, error) {
	state, err := LoadState()
	if err != nil {
		return "", fmt.Errorf("error loading state: %w", err)
	}

	elapsed := state.ElapsedTime
	if state.Running {
		elapsed += time.Since(state.StartTime)
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

	// Build and execute the task command
	taskDescription := fmt.Sprintf("%dmin %s", minutes, description)
	cmd := exec.Command("task", "add", "+track", taskDescription)

	// Execute the command and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Command failed, return error with output
		return "", fmt.Errorf("task command failed: %s\nOutput: %s", err, string(output))
	}

	// Command succeeded, reset the timer
	if _, err := ResetTimer(); err != nil {
		return "", fmt.Errorf("tracked time successfully but failed to reset timer: %w", err)
	}

	return fmt.Sprintf("Tracked %d minutes: %s\nTimer reset.", minutes, description), nil
}
