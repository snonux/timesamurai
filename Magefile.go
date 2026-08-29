//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/sh"
)

var Default = Build

// Build compiles the timesamurai binary into the repo root.
func Build() error {
	return sh.RunV("go", "build", "-o", "timesamurai", "./cmd/timesamurai")
}

// Run executes timesamurai via go run.
func Run() error {
	return sh.RunV("go", "run", "./cmd/timesamurai")
}

// Test runs the full race-enabled test suite.
func Test() error {
	return sh.RunV("go", "test", "-race", "./...")
}

// Lint runs golangci-lint.
func Lint() error {
	return sh.RunV("golangci-lint", "run", "./...")
}

// Install installs timesamurai into GOPATH/bin.
func Install() error {
	return sh.RunV("go", "install", "./cmd/timesamurai")
}

// Vet runs go vet, then fails if gofmt -l or errcheck report issues.
func Vet() error {
	if err := sh.RunV("go", "vet", "./..."); err != nil {
		return err
	}
	if err := checkGofmt(); err != nil {
		return err
	}
	return runErrcheck()
}

// Completions regenerates completions/timesamurai.fish from the Cobra completion command.
func Completions() error {
	if err := os.MkdirAll("completions", 0o755); err != nil {
		return fmt.Errorf("mkdir completions: %w", err)
	}
	out, err := sh.Output("go", "run", "./cmd/timesamurai", "completion", "fish")
	if err != nil {
		return fmt.Errorf("generate fish completions: %w", err)
	}
	// sh.Output trims one trailing newline; restore it for POSIX text files.
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	path := filepath.Join("completions", "timesamurai.fish")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Println("wrote", path)
	return nil
}

func checkGofmt() error {
	out, err := sh.Output("gofmt", "-l", ".")
	if err != nil {
		return fmt.Errorf("gofmt -l: %w", err)
	}
	files := strings.TrimSpace(out)
	if files == "" {
		return nil
	}
	return fmt.Errorf("gofmt -l found unformatted files:\n%s", files)
}

func runErrcheck() error {
	bin := filepath.Join(os.Getenv("HOME"), "go", "bin", "errcheck")
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("errcheck not found at %s (install: go install github.com/kisielk/errcheck@latest)", bin)
	}
	return sh.RunV(bin, "./...")
}
