package cmd

import (
	"fmt"

	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all recyclable worktrees",
	Long:  "Finds all recyclable worktrees (merged/closed PR, clean working tree) and removes them.",
	RunE:  runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	gitClient := gitpkg.NewExecClient()

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	e := newEnv(gitClient, repoRoot)
	return e.doClean()
}

func (e *env) doClean() error {
	result, err := cycle.FindRecyclable(e.deps)
	if err != nil {
		return err
	}

	if len(result.Recyclable) == 0 {
		e.deps.Logf("✨ No worktrees to clean")
		return nil
	}

	var branches []string
	for _, r := range result.Recyclable {
		branches = append(branches, r.Branch)
	}
	e.deps.Logf("🧹 Cleaning: %v", branches)

	for _, r := range result.Recyclable {
		if err := e.runWt("remove", "-y", r.Branch); err != nil {
			e.deps.Logf("warning: failed to remove worktree %s: %v", r.Branch, err)
			continue
		}
		if _, err := e.deps.Git.Run("branch", "-D", r.Branch); err != nil {
			e.deps.Logf("warning: failed to delete branch %s: %v", r.Branch, err)
		}
	}

	e.deps.Logf("✅ Done")
	return nil
}
