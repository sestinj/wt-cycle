package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDoList_Table_HappyPath(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	porcelain := fmt.Sprintf(
		"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n"+
			"worktree %s\nHEAD def\nbranch refs/heads/wt-2\n\n"+
			"worktree %s\nHEAD ghi\nbranch refs/heads/wt-3\n\n",
		dir1, dir2, dir3,
	)

	g := &mockGit{
		currentBranch: "wt-3",
		merged:        []string{"wt-1"},
		wtPorcelain:   porcelain,
		cleanPaths:    map[string]bool{dir1: true, dir2: false, dir3: true},
		repoRoot:      dir1,
	}

	e, stdout := testEnv(t, g, &mockGH{})

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	out := stdout.String()

	// Verify header
	if !strings.Contains(out, "BRANCH") {
		t.Error("missing BRANCH header")
	}
	if !strings.Contains(out, "STATUS") {
		t.Error("missing STATUS header")
	}

	// wt-1 should be recyclable (merged + clean)
	if !strings.Contains(out, "recyclable") {
		t.Error("expected 'recyclable' in output for wt-1")
	}

	// wt-3 should be current
	if !strings.Contains(out, "current") {
		t.Error("expected 'current' in output for wt-3")
	}

	// All 3 branches should appear
	if !strings.Contains(out, "wt-1") || !strings.Contains(out, "wt-2") || !strings.Contains(out, "wt-3") {
		t.Errorf("expected all branches in output, got:\n%s", out)
	}
}

func TestDoList_JSON(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
	}

	e, stdout := testEnv(t, g, &mockGH{})
	e.jsonOut = true

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	var statuses []wtStatus
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Branch != "wt-1" {
		t.Errorf("branch = %q, want wt-1", statuses[0].Branch)
	}
	if !statuses[0].Recyclable {
		t.Error("expected recyclable = true")
	}
	if statuses[0].Path != dir {
		t.Errorf("path = %q, want %q", statuses[0].Path, dir)
	}
}

func TestDoList_Empty(t *testing.T) {
	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      t.TempDir(),
	}

	e, stdout := testEnv(t, g, &mockGH{})

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout.String(), "No wt-N worktrees found") {
		t.Errorf("expected empty message, got: %q", stdout.String())
	}
}

func TestDoList_FiltersNonWtBranches(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Include a non-wt branch (main) and a wt branch
	porcelain := fmt.Sprintf(
		"worktree %s\nHEAD abc\nbranch refs/heads/main\n\n"+
			"worktree %s\nHEAD def\nbranch refs/heads/wt-1\n\n",
		dir1, dir2,
	)

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   porcelain,
		cleanPaths:    map[string]bool{dir2: true},
		repoRoot:      dir1,
	}

	e, stdout := testEnv(t, g, &mockGH{})
	e.jsonOut = true

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	var statuses []wtStatus
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatal(err)
	}

	// Only wt-1 should be in the list, not "main"
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status (wt-1 only), got %d: %+v", len(statuses), statuses)
	}
	if statuses[0].Branch != "wt-1" {
		t.Errorf("branch = %q, want wt-1", statuses[0].Branch)
	}
}

func TestDoList_StatusLabels(t *testing.T) {
	dirCurrent := t.TempDir()
	dirRecyclable := t.TempDir()
	dirActive := t.TempDir()

	porcelain := fmt.Sprintf(
		"worktree %s\nHEAD a\nbranch refs/heads/wt-1\n\n"+
			"worktree %s\nHEAD b\nbranch refs/heads/wt-2\n\n"+
			"worktree %s\nHEAD c\nbranch refs/heads/wt-3\n\n",
		dirCurrent, dirRecyclable, dirActive,
	)

	g := &mockGit{
		currentBranch: "wt-1",
		merged:        []string{"wt-2"},
		wtPorcelain:   porcelain,
		cleanPaths:    map[string]bool{dirRecyclable: true, dirActive: true},
		repoRoot:      dirCurrent,
	}

	e, stdout := testEnv(t, g, &mockGH{})
	e.jsonOut = true

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	var statuses []wtStatus
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatal(err)
	}

	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}

	byBranch := make(map[string]wtStatus)
	for _, s := range statuses {
		byBranch[s.Branch] = s
	}

	// wt-1: current branch
	if s := byBranch["wt-1"]; !s.Current {
		t.Errorf("wt-1: expected current=true")
	}

	// wt-2: merged → recyclable
	if s := byBranch["wt-2"]; !s.Recyclable {
		t.Errorf("wt-2: expected recyclable=true")
	}

	// wt-3: not merged, not current → active
	if s := byBranch["wt-3"]; s.Reason != "active" {
		t.Errorf("wt-3: expected reason='active', got %q", s.Reason)
	}
}

func TestDoList_WorktreeListError(t *testing.T) {
	g := &mockGit{
		currentBranch:  "main",
		wtPorcelainErr: fmt.Errorf("git worktree list failed"),
		repoRoot:       t.TempDir(),
	}

	e, _ := testEnv(t, g, &mockGH{})

	err := e.doList()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing worktrees") {
		t.Errorf("error = %q, want it to mention 'listing worktrees'", err)
	}
}

func TestDoList_SkippedReasons(t *testing.T) {
	dirDirty := t.TempDir()

	porcelain := fmt.Sprintf(
		"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n",
		dirDirty,
	)

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   porcelain,
		cleanPaths:    map[string]bool{dirDirty: false}, // dirty
		repoRoot:      dirDirty,
	}

	e, stdout := testEnv(t, g, &mockGH{})
	e.jsonOut = true

	if err := e.doList(); err != nil {
		t.Fatal(err)
	}

	var statuses []wtStatus
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatal(err)
	}

	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].Reason != "dirty" {
		t.Errorf("reason = %q, want 'dirty'", statuses[0].Reason)
	}
	if statuses[0].Recyclable {
		t.Error("expected recyclable=false for dirty worktree")
	}
}
