package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Client abstracts GitHub API operations.
type Client interface {
	// ClosedPRBranches returns branch names for all non-OPEN PRs in the repo.
	ClosedPRBranches() ([]string, error)
}

// GHClient implements Client by shelling out to `gh`.
type GHClient struct{}

func NewGHClient() *GHClient {
	return &GHClient{}
}

type prEntry struct {
	HeadRefName string `json:"headRefName"`
	State       string `json:"state"`
}

func (c *GHClient) ClosedPRBranches() ([]string, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--state", "all",
		"--json", "headRefName,state",
		"--limit", "500",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	return ParseClosedPRBranches(out)
}

// ParseClosedPRBranches extracts branch names for non-OPEN PRs from gh JSON output.
func ParseClosedPRBranches(data []byte) ([]string, error) {
	var entries []prEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing gh output: %w", err)
	}
	var branches []string
	for _, e := range entries {
		if strings.ToUpper(e.State) != "OPEN" {
			branches = append(branches, e.HeadRefName)
		}
	}
	return branches, nil
}
