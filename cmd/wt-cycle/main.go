package main

import (
	"os"

	"github.com/sestinj/wt-cycle/internal/cmd"
)

var version = "dev" // overridden by ldflags at build time

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
