//go:build mage

package main

import (
	"github.com/magefile/mage/sh"
)

var Default = Build

func Build() error {
	return sh.RunV("go", "build", "-o", "timesamurai", "./cmd/timesamurai")
}

func Run() error {
	return sh.RunV("go", "run", "./cmd/timesamurai")
}

func Test() error {
	return sh.RunV("go", "test", "-race", "./...")
}

func Lint() error {
	return sh.RunV("golangci-lint", "run", "./...")
}

func Install() error {
	return sh.RunV("go", "install", "./cmd/timesamurai")
}
