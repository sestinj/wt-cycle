package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
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

	e := newEnv(gitClient, repoRoot)
	return e.doNext()
}

func (e *env) doNext() error {
	// Find recyclable worktrees
	result, err := cycle.FindRecyclable(e.deps)
	if err != nil {
		return err
	}

	// Compute next wt-N number
	existingNums, err := cycle.CollectExistingNums(e.deps)
	if err != nil {
		return fmt.Errorf("collecting existing numbers: %w", err)
	}
	nextNum := cycle.NextNum(existingNums)
	newBranch := fmt.Sprintf("wt-%d", nextNum)

	if len(result.Recyclable) > 0 {
		return e.recycleWorktree(result.Recyclable[0], newBranch)
	}
	return e.createWorktree(newBranch)
}

func (e *env) recycleWorktree(target cycle.Recyclable, newBranch string) error {
	e.deps.Logf("♻️  Recycling %s", target.Branch)

	// Switch to the recyclable worktree
	if err := e.runWt("switch", target.Branch); err != nil {
		return fmt.Errorf("wt switch %s: %w", target.Branch, err)
	}

	// wt switch runs as a subprocess and cannot change the parent
	// process's cwd. Explicitly chdir so subsequent git commands
	// target the correct worktree.
	if err := e.chdir(target.Path); err != nil {
		return fmt.Errorf("chdir to %s: %w", target.Path, err)
	}

	// Detach HEAD, delete old branch, create new
	e.deps.Logf("🔄 Updating to latest main and creating branch %s", newBranch)
	if _, err := e.deps.Git.Run("checkout", "-q", "origin/main"); err != nil {
		return fmt.Errorf("checkout origin/main: %w", err)
	}
	if _, err := e.deps.Git.Run("branch", "-D", target.Branch); err != nil {
		e.deps.Logf("warning: could not delete branch %s: %v", target.Branch, err)
	}
	if _, err := e.deps.Git.Run("checkout", "-q", "-b", newBranch); err != nil {
		return fmt.Errorf("checkout -b %s: %w", newBranch, err)
	}

	// Print the worktree path
	fmt.Fprintln(e.stdout, target.Path)
	return nil
}

func (e *env) createWorktree(newBranch string) error {
	e.deps.Logf("✨ Creating %s", newBranch)

	// Create new worktree via worktrunk
	if err := e.runWt("switch", "-c", newBranch, "--base", "origin/main"); err != nil {
		return fmt.Errorf("wt switch -c %s: %w", newBranch, err)
	}

	// wt switch runs as a subprocess and cannot change the parent
	// process's cwd. Compute the worktree path using the same
	// convention as worktrunk: <parent>/<base-repo-name>.<branch>
	repoParent := filepath.Dir(e.repoRoot)
	baseName := filepath.Base(e.repoRoot)
	if idx := strings.Index(baseName, ".wt-"); idx != -1 {
		baseName = baseName[:idx]
	}
	newPath := filepath.Join(repoParent, baseName+"."+newBranch)
	if err := e.chdir(newPath); err != nil {
		return fmt.Errorf("chdir to new worktree %s: %w", newPath, err)
	}
	fmt.Fprintln(e.stdout, newPath)
	return nil
}
