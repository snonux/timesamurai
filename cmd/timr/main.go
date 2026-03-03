package main

import (
	"fmt"
	"os"

	"codeberg.org/snonux/timr/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
