package main

import (
	"os"

	"github.com/sestinj/wt-cycle/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
