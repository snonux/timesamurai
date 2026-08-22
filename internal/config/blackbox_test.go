package config_test

import (
	"testing"

	"github.com/snonux/timesamurai/internal/config"
)

func TestDefaultPublicAPI(t *testing.T) {
	cfg := config.Default()
	if cfg.WeekWorkHours <= 0 {
		t.Fatalf("WeekWorkHours = %v, want positive value", cfg.WeekWorkHours)
	}
	if cfg.WorktimeDBDir == "" {
		t.Fatal("WorktimeDBDir is empty")
	}
}
