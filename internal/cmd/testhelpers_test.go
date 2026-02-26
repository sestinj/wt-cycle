package cmd

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"github.com/sestinj/wt-cycle/internal/cycle"
)

// mockGit implements git.Client for command-level testing.
type mockGit struct {
	currentBranch    string
	currentBranchErr error
	merged           []string
	mergedErr        error
	wtPorcelain      string
	wtPorcelainErr   error
	refs             []string
	refsErr          error
	cleanPaths       map[string]bool // path -> isClean
	repoRoot         string
	repoRootErr      error

	mu       sync.Mutex
	runCalls [][]string
	runFn    func(args []string) (string, error)
}

func (m *mockGit) FetchOriginMain() error { return nil }
func (m *mockGit) MergedBranches(_ string) ([]string, error) {
	return m.merged, m.mergedErr
}
func (m *mockGit) WorktreeListPorcelain() (string, error) {
	return m.wtPorcelain, m.wtPorcelainErr
}
func (m *mockGit) ForEachRef(_ ...string) ([]string, error) {
	return m.refs, m.refsErr
}
func (m *mockGit) CurrentBranch() (string, error) {
	return m.currentBranch, m.currentBranchErr
}
func (m *mockGit) RepoRoot() (string, error) {
	return m.repoRoot, m.repoRootErr
}
func (m *mockGit) IsClean(path string) (bool, error) {
	clean, ok := m.cleanPaths[path]
	if !ok {
		return false, fmt.Errorf("unknown path: %s", path)
	}
	return clean, nil
}
func (m *mockGit) Run(args ...string) (string, error) {
	m.mu.Lock()
	m.runCalls = append(m.runCalls, args)
	m.mu.Unlock()
	if m.runFn != nil {
		return m.runFn(args)
	}
	return "", nil
}

// mockGH implements github.Client for testing.
type mockGH struct {
	branches []string
	err      error
}

func (m *mockGH) ClosedPRBranches() ([]string, error) { return m.branches, m.err }

func nopLogf(string, ...interface{}) {}

// testEnv creates an env suitable for testing with sensible defaults.
// Returns the env and a buffer capturing stdout.
func testEnv(t *testing.T, g *mockGit, gh *mockGH) (*env, *bytes.Buffer) {
	t.Helper()
	var stdout bytes.Buffer
	return &env{
		repoRoot: g.repoRoot,
		deps: &cycle.Deps{
			Git:    g,
			GitHub: gh,
			Logf:   nopLogf,
		},
		runWt: func(args ...string) error { return nil },
		chdir: func(path string) error { return nil },
		stdout: &stdout,
	}, &stdout
}

// assertArgs verifies that a slice of args matches expected values.
func assertArgs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("args = %v, want %v", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q; got %v", i, got[i], want[i], got)
			return
		}
	}
}
