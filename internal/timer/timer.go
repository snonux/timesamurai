package timer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	stateFile = ".timr_state"
)

// StateFilePathOverride is used by tests to override the state file path.
var StateFilePathOverride string

func SetStateFilePathOverride(path string) {
	StateFilePathOverride = path
}

type State struct {
	StartTime    time.Time
	ElapsedTime  time.Duration
	Running      bool
}

func GetStateFile() (string, error) {
	if StateFilePathOverride != "" {
		return StateFilePathOverride, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "timr", stateFile), nil
}

func LoadState() (State, error) {
	var state State
	stateFilePath, err := GetStateFile()
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

func (s *State) Save() error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	stateFilePath, err := GetStateFile()
	if err != nil {
		return err
	}

	dir := filepath.Dir(stateFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(stateFilePath, data, 0644)
}
