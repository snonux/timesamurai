//go:build mage

package main

import (
	"github.com/magefile/mage/sh"
)

var Default = Build

func Build() error {
	return sh.RunV("go", "build", "-o", "timr", "./cmd/timr")
}

func Run() error {
	return sh.RunV("go", "run", "./cmd/timr")
}

func Test() error {
	return sh.RunV("go", "test", "./...")
}

func Lint() error {
	return sh.RunV("golangci-lint", "run", "./...")
}

func Install() error {
	return sh.RunV("go", "install", "./cmd/timr")
}
