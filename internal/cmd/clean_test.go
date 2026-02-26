package cmd

import (
	"fmt"
	"strings"
	"testing"
)

func TestDoClean_HappyPath(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(
			"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n"+
				"worktree %s\nHEAD def\nbranch refs/heads/wt-2\n\n",
			dir1, dir2,
		),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
		repoRoot:   dir1,
	}

	var wtCalls [][]string
	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		wtCalls = append(wtCalls, args)
		return nil
	}

	if err := e.doClean(); err != nil {
		t.Fatal(err)
	}

	// Should call wt remove for each recyclable
	if len(wtCalls) != 2 {
		t.Fatalf("expected 2 wt remove calls, got %d: %v", len(wtCalls), wtCalls)
	}
	for _, call := range wtCalls {
		if len(call) < 2 || call[0] != "remove" || call[1] != "-y" {
			t.Errorf("expected [remove -y <branch>], got %v", call)
		}
	}

	// Should call git branch -D for each
	if len(g.runCalls) != 2 {
		t.Fatalf("expected 2 branch -D calls, got %d: %v", len(g.runCalls), g.runCalls)
	}
	for _, call := range g.runCalls {
		assertArgs(t, call[:2], "branch", "-D")
	}
}

func TestDoClean_NoRecyclable(t *testing.T) {
	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      t.TempDir(),
	}

	wtCalled := false
	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		wtCalled = true
		return nil
	}

	if err := e.doClean(); err != nil {
		t.Fatal(err)
	}

	if wtCalled {
		t.Error("wt should not be called when nothing to clean")
	}
	if len(g.runCalls) != 0 {
		t.Errorf("expected no git Run calls, got %v", g.runCalls)
	}
}

func TestDoClean_WtRemoveFails_Continues(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(
			"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n"+
				"worktree %s\nHEAD def\nbranch refs/heads/wt-2\n\n",
			dir1, dir2,
		),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
		repoRoot:   dir1,
	}

	callCount := 0
	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("remove failed for first worktree")
		}
		return nil
	}

	// Should not return error — wt remove failures are non-fatal
	if err := e.doClean(); err != nil {
		t.Fatal(err)
	}

	// Both worktrees should have been attempted
	if callCount != 2 {
		t.Errorf("expected 2 wt remove attempts, got %d", callCount)
	}

	// Only the second worktree's branch should be deleted
	// (first worktree's remove failed, so branch -D is skipped)
	if len(g.runCalls) != 1 {
		t.Fatalf("expected 1 branch -D call (skipped failed wt), got %d: %v", len(g.runCalls), g.runCalls)
	}
}

func TestDoClean_BranchDeleteFails_Continues(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(
			"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n"+
				"worktree %s\nHEAD def\nbranch refs/heads/wt-2\n\n",
			dir1, dir2,
		),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
		repoRoot:   dir1,
		runFn: func(args []string) (string, error) {
			// First branch -D fails
			if args[0] == "branch" && args[2] == "wt-1" {
				return "", fmt.Errorf("cannot delete branch")
			}
			return "", nil
		},
	}

	e, _ := testEnv(t, g, &mockGH{})

	// Should not return error — branch delete failures are non-fatal
	if err := e.doClean(); err != nil {
		t.Fatal(err)
	}

	// Both branch -D calls should have been attempted
	if len(g.runCalls) != 2 {
		t.Fatalf("expected 2 branch -D calls, got %d: %v", len(g.runCalls), g.runCalls)
	}
}

func TestDoClean_CorrectWtArgs(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-42"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-42\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
	}

	var wtCalls [][]string
	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		wtCalls = append(wtCalls, args)
		return nil
	}

	if err := e.doClean(); err != nil {
		t.Fatal(err)
	}

	if len(wtCalls) != 1 {
		t.Fatalf("expected 1 wt call, got %d", len(wtCalls))
	}
	assertArgs(t, wtCalls[0], "remove", "-y", "wt-42")

	if len(g.runCalls) != 1 {
		t.Fatalf("expected 1 git Run call, got %d", len(g.runCalls))
	}
	assertArgs(t, g.runCalls[0], "branch", "-D", "wt-42")
}

func TestDoClean_FindRecyclableError(t *testing.T) {
	g := &mockGit{
		currentBranch: "main",
		mergedErr:     fmt.Errorf("git error"),
		repoRoot:      t.TempDir(),
	}

	e, _ := testEnv(t, g, &mockGH{})

	err := e.doClean()
	if err == nil {
		t.Fatal("expected error from FindRecyclable")
	}
	if !strings.Contains(err.Error(), "merged") {
		t.Errorf("error = %q, want it to mention 'merged'", err)
	}
}
