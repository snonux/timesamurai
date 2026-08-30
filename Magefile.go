//go:build mage

// Timesamurai mage targets: build, run, test, vet, lint, install, completions, logo.
// Mirrors the Mage conventions used across other projects (e.g. hexai): a
// binaryName constant used everywhere the built binary's name is referenced,
// a Default target that depends on Build via mg.Deps, and Install building
// first (via mg.Deps(Build)) before copying the binary into GOPATH/bin.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// binaryName is the name of the built timesamurai binary. Defined once here
// so Build, Install, and Completions don't each hardcode the literal.
const binaryName = "timesamurai"

// Default is the default Mage target (runs when `mage` is invoked without a
// target). It depends on Build via mg.Deps so plain `mage` builds the binary.
func Default() {
	mg.Deps(Build)
}

// Build compiles the timesamurai binary into the repo root.
func Build() error {
	return sh.RunV("go", "build", "-o", binaryName, "./cmd/timesamurai")
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

// Install depends on Build, then copies the freshly built binary into
// GOPATH/bin (defaults to ~/go/bin when GOPATH is unset).
func Install() error {
	mg.Deps(Build)

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home: %w", err)
		}
		gopath = filepath.Join(home, "go")
	}

	bin := filepath.Join(gopath, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", bin, err)
	}
	return sh.RunV("cp", "-v", binaryName, filepath.Join(bin, binaryName))
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
	path := filepath.Join("completions", binaryName+".fish")
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

// logoSizes are the PNG renditions generated from logo.svg, matching the set
// the quicklog project ships so the two repos present alike.
var logoSizes = map[string]int{
	"logo.png":       600,
	"logo-small.png": 300,
	"icon.png":       64,
}

// Logo re-renders the PNG logo set from logo.svg.
//
// logo.svg is the only source; the PNGs are build output that happens to be
// committed, because README.md and forge pages cannot render a local SVG
// reliably. Regenerate rather than hand-editing a PNG. Requires ImageMagick
// built with librsvg (`magick -list format | grep RSVG`); the internal MSVG
// renderer does not handle the gradients.
func Logo() error {
	if _, err := exec.LookPath("magick"); err != nil {
		return fmt.Errorf("imagemagick 'magick' not found on PATH: %w", err)
	}
	names := make([]string, 0, len(logoSizes))
	for name := range logoSizes {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		size := strconv.Itoa(logoSizes[name])
		if err := sh.Run("magick", "logo.svg", "-background", "none",
			"-resize", size+"x"+size, name); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		fmt.Println("wrote", name)
	}
	return nil
}
