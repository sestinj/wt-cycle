package git

import (
	"regexp"
	"strconv"
	"strings"
)

// Worktree represents a parsed worktree entry from `git worktree list --porcelain`.
type Worktree struct {
	Path   string
	Branch string // short name, e.g. "wt-42" (empty if detached)
	Bare   bool
}

// ParseWorktreeList parses `git worktree list --porcelain` output into Worktree structs.
func ParseWorktreeList(output string) []Worktree {
	var worktrees []Worktree
	var current Worktree

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "bare":
			current.Bare = true
		case line == "":
			if current.Path != "" {
				worktrees = append(worktrees, current)
				current = Worktree{}
			}
		}
	}
	// Handle final entry if no trailing newline
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}
	return worktrees
}

var wtNumRe = regexp.MustCompile(`^wt-(\d+)$`)

// ExtractWtNum extracts the number N from a "wt-N" branch name. Returns -1 if not matching.
func ExtractWtNum(name string) int {
	m := wtNumRe.FindStringSubmatch(name)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return n
}

// FilterWtBranches filters a list of branch names to only wt-N branches.
func FilterWtBranches(branches []string) []string {
	var result []string
	for _, b := range branches {
		if ExtractWtNum(b) >= 0 {
			result = append(result, b)
		}
	}
	return result
}

// WorktreesByBranch builds a map from branch name to Worktree.
func WorktreesByBranch(worktrees []Worktree) map[string]Worktree {
	m := make(map[string]Worktree, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch != "" {
			m[wt.Branch] = wt
		}
	}
	return m
}
