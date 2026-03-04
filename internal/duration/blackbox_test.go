package duration_test

import (
	"testing"
	"time"

	"codeberg.org/snonux/timesamurai/internal/duration"
)

func TestParsePublicAPI(t *testing.T) {
	got, err := duration.Parse("90m")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got != 90*time.Minute {
		t.Fatalf("Parse() = %v, want %v", got, 90*time.Minute)
	}
}
