package cmd

import (
	"fmt"
	"os"

	"github.com/sestinj/wt-cycle/internal/cache"
	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
	ghpkg "github.com/sestinj/wt-cycle/internal/github"
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

	result, err := cycle.FindRecyclable(d)
	if err != nil {
		return err
	}

	if len(result.Recyclable) == 0 {
		logf("✨ No worktrees to clean")
		return nil
	}

	var branches []string
	for _, r := range result.Recyclable {
		branches = append(branches, r.Branch)
	}
	logf("🧹 Cleaning: %v", branches)

	for _, r := range result.Recyclable {
		if err := runWt("remove", "-y", r.Branch); err != nil {
			logf("warning: failed to remove worktree %s: %v", r.Branch, err)
			continue
		}
		if _, err := gitClient.Run("branch", "-D", r.Branch); err != nil {
			logf("warning: failed to delete branch %s: %v", r.Branch, err)
		}
	}

	logf("✅ Done")
	return nil
}
