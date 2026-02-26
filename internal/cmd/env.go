package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/sestinj/wt-cycle/internal/cache"
	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
	ghpkg "github.com/sestinj/wt-cycle/internal/github"
)

// env bundles dependencies for command execution.
// Production commands use newEnv(); tests construct directly with mocks.
type env struct {
	repoRoot string
	deps     *cycle.Deps
	runWt    func(args ...string) error
	chdir    func(path string) error
	stdout   io.Writer
	jsonOut  bool
}

func newEnv(gitClient gitpkg.Client, repoRoot string) *env {
	logf := func(format string, a ...interface{}) {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
	return &env{
		repoRoot: repoRoot,
		deps: &cycle.Deps{
			Git:     gitClient,
			GitHub:  ghpkg.NewGHClient(),
			Cache:   cache.New(repoRoot),
			NoCache: noCache,
			Verbose: verbose,
			Logf:    logf,
		},
		runWt: func(args ...string) error {
			c := exec.Command("wt", args...)
			c.Stdout = os.Stderr
			c.Stderr = os.Stderr
			c.Stdin = os.Stdin
			return c.Run()
		},
		chdir:   os.Chdir,
		stdout:  os.Stdout,
		jsonOut: jsonOut,
	}
}
