package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/sestinj/wt-cycle/internal/cycle"
	gitpkg "github.com/sestinj/wt-cycle/internal/git"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Show worktree status table",
	Long:  "Lists all wt-N worktrees with their recyclability status.",
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

type wtStatus struct {
	Branch     string `json:"branch"`
	Path       string `json:"path"`
	Recyclable bool   `json:"recyclable"`
	Reason     string `json:"reason,omitempty"`
	Current    bool   `json:"current"`
}

func runList(cmd *cobra.Command, args []string) error {
	gitClient := gitpkg.NewExecClient()

	repoRoot, err := gitClient.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	e := newEnv(gitClient, repoRoot)
	return e.doList()
}

func (e *env) doList() error {
	// Get all worktrees
	wtOutput, err := e.deps.Git.WorktreeListPorcelain()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}
	allWts := gitpkg.ParseWorktreeList(wtOutput)

	// FindRecyclable does all the expensive work (including parallel IsClean)
	result, err := cycle.FindRecyclable(e.deps)
	if err != nil {
		return err
	}

	// Build lookup maps from FindResult
	recyclableSet := make(map[string]bool)
	for _, r := range result.Recyclable {
		recyclableSet[r.Branch] = true
	}
	skippedReason := make(map[string]string)
	for _, s := range result.Skipped {
		skippedReason[s.Branch] = s.Reason
	}

	currentBranch, _ := e.deps.Git.CurrentBranch()

	// Build status list for wt-N worktrees
	var statuses []wtStatus
	for _, wt := range allWts {
		if gitpkg.ExtractWtNum(wt.Branch) < 0 {
			continue
		}
		s := wtStatus{
			Branch:     wt.Branch,
			Path:       wt.Path,
			Current:    wt.Branch == currentBranch,
			Recyclable: recyclableSet[wt.Branch],
		}
		if reason, ok := skippedReason[wt.Branch]; ok {
			s.Reason = reason
		} else if !s.Recyclable {
			s.Reason = "active" // not a candidate (not merged/closed)
		}
		statuses = append(statuses, s)
	}

	if e.jsonOut {
		enc := json.NewEncoder(e.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(statuses)
	}

	if len(statuses) == 0 {
		fmt.Fprintln(e.stdout, "No wt-N worktrees found.")
		return nil
	}

	w := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "BRANCH\tPATH\tSTATUS\tREASON")
	for _, s := range statuses {
		status := "active"
		if s.Current {
			status = "current"
		} else if s.Recyclable {
			status = "recyclable"
		}
		reason := s.Reason
		if reason == "" {
			reason = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Branch, s.Path, status, reason)
	}
	w.Flush()

	return nil
}
