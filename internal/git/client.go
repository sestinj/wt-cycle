package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Client abstracts git operations for testability.
type Client interface {
	// FetchOriginMain runs git fetch -q origin main.
	FetchOriginMain() error
	// MergedBranches returns branches matching pattern merged into origin/main.
	MergedBranches(pattern string) ([]string, error)
	// WorktreeListPorcelain returns raw `git worktree list --porcelain` output.
	WorktreeListPorcelain() (string, error)
	// ForEachRef returns ref short names matching the given patterns.
	ForEachRef(patterns ...string) ([]string, error)
	// IsClean returns true if the worktree at path has no modifications or untracked files.
	IsClean(path string) (bool, error)
	// CurrentBranch returns the current branch name, or "" if detached.
	CurrentBranch() (string, error)
	// RepoRoot returns the root directory of the repo.
	RepoRoot() (string, error)
	// Run executes an arbitrary git command and returns stdout.
	Run(args ...string) (string, error)
}

// ExecClient implements Client by shelling out to git.
type ExecClient struct{}

func NewExecClient() *ExecClient {
	return &ExecClient{}
}

func (c *ExecClient) FetchOriginMain() error {
	cmd := exec.Command("git", "fetch", "-q", "origin", "main")
	cmd.Stderr = nil
	return cmd.Run()
}

func (c *ExecClient) MergedBranches(pattern string) ([]string, error) {
	out, err := c.Run("branch", "--merged", "origin/main", "--list", pattern, "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	return nonEmpty(strings.Split(out, "\n")), nil
}

func (c *ExecClient) WorktreeListPorcelain() (string, error) {
	return c.Run("worktree", "list", "--porcelain")
}

func (c *ExecClient) ForEachRef(patterns ...string) ([]string, error) {
	args := append([]string{"for-each-ref", "--format=%(refname:short)"}, patterns...)
	out, err := c.Run(args...)
	if err != nil {
		return nil, err
	}
	return nonEmpty(strings.Split(out, "\n")), nil
}

func (c *ExecClient) IsClean(path string) (bool, error) {
	// Check for staged and unstaged changes
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", path, err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

func (c *ExecClient) CurrentBranch() (string, error) {
	out, err := c.Run("branch", "--show-current")
	if err != nil {
		return "", nil // detached HEAD
	}
	return strings.TrimSpace(out), nil
}

func (c *ExecClient) RepoRoot() (string, error) {
	out, err := c.Run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c *ExecClient) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func nonEmpty(ss []string) []string {
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			result = append(result, s)
		}
	}
	return result
}
