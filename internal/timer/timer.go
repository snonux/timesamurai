package timer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stateFile = ".timesamurai_state"
)

// State stores persisted timer progress.
type State struct {
	StartTime   time.Time
	ElapsedTime time.Duration
	Running     bool
}

func resolveStateFilePath(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return path, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "timesamurai", stateFile), nil
}

// GetStateFile returns the default state file path.
func GetStateFile() (string, error) {
	return resolveStateFilePath("")
}

// LoadState loads timer state from the default state file.
func LoadState() (State, error) {
	return LoadStateAt("")
}

// LoadStateAt loads timer state from the provided path or default path when empty.
func LoadStateAt(path string) (State, error) {
	var state State
	stateFilePath, err := resolveStateFilePath(path)
	if err != nil {
		return state, err
	}

	data, err := os.ReadFile(stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return state, err
	}

	err = json.Unmarshal(data, &state)
	return state, err
}

// Save writes timer state to the default state file.
func (s *State) Save() error {
	return s.SaveAt("")
}

// SaveAt writes timer state to the provided path or default path when empty.
func (s *State) SaveAt(path string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	stateFilePath, err := resolveStateFilePath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(stateFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(stateFilePath, data, 0644)
}
