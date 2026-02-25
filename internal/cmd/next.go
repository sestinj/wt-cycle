package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/sestinj/wt-cycle/internal/cache"
	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
	ghpkg "github.com/sestinj/wt-cycle/internal/github"
	"github.com/sestinj/wt-cycle/internal/lock"
	"github.com/spf13/cobra"
)

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Create or recycle a worktree",
	Long:  "Finds a recyclable worktree (merged/closed PR, clean) or creates a new one. Prints the worktree path to stdout.",
	RunE:  runNext,
}

func init() {
	rootCmd.AddCommand(nextCmd)
}

func runNext(cmd *cobra.Command, args []string) error {
	gitClient := gitpkg.NewExecClient()

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Acquire lock
	lk := lock.New(repoRoot)
	if err := lk.Acquire(lock.DefaultTimeout); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer lk.Release()

	logf := func(format string, args ...interface{}) {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}

	d := &cycle.Deps{
		Git:     gitClient,
		GitHub:  ghpkg.NewGHClient(),
		Cache:   cache.New(repoRoot),
		NoCache: noCache,
		Verbose: verbose,
		Logf:    logf,
	}

	// Find recyclable worktrees
	result, err := cycle.FindRecyclable(d)
	if err != nil {
		return err
	}

	// Compute next wt-N number
	existingNums, err := cycle.CollectExistingNums(d)
	if err != nil {
		return fmt.Errorf("collecting existing numbers: %w", err)
	}
	nextNum := cycle.NextNum(existingNums)
	newBranch := fmt.Sprintf("wt-%d", nextNum)

	if len(result.Recyclable) > 0 {
		target := result.Recyclable[0]
		logf("♻️  Recycling %s", target.Branch)

		// Switch to the recyclable worktree
		if err := runWt("switch", target.Branch); err != nil {
			return fmt.Errorf("wt switch %s: %w", target.Branch, err)
		}

		// Detach HEAD, delete old branch, create new
		logf("🔄 Updating to latest main and creating branch %s", newBranch)
		if _, err := gitClient.Run("checkout", "-q", "origin/main"); err != nil {
			return fmt.Errorf("checkout origin/main: %w", err)
		}
		if _, err := gitClient.Run("branch", "-D", target.Branch); err != nil {
			logf("warning: could not delete branch %s: %v", target.Branch, err)
		}
		if _, err := gitClient.Run("checkout", "-q", "-b", newBranch); err != nil {
			return fmt.Errorf("checkout -b %s: %w", newBranch, err)
		}

		// Print the worktree path
		fmt.Println(target.Path)
	} else {
		logf("✨ Creating %s", newBranch)

		// Create new worktree via worktrunk
		if err := runWt("switch", "-c", newBranch, "--base", "origin/main"); err != nil {
			return fmt.Errorf("wt switch -c %s: %w", newBranch, err)
		}

		// Get the new working directory (wt switch changes cwd via shell eval)
		// We need to figure out the path. Worktrunk creates at <parent>/<base>.wt-N
		newRoot, err := gitClient.RepoRoot()
		if err != nil {
			// Fall back to computing the path
			return fmt.Errorf("could not determine new worktree path: %w", err)
		}
		fmt.Println(newRoot)
	}

	return nil
}

func runWt(args ...string) error {
	c := exec.Command("wt", args...)
	c.Stdout = os.Stderr // wt output goes to stderr
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	return c.Run()
}
