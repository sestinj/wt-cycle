package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Recycling path ---

func TestDoNext_Recycle_HappyPath(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	var chdirPath string
	e.chdir = func(path string) error {
		chdirPath = path
		return nil
	}

	var wtArgs []string
	e.runWt = func(args ...string) error {
		wtArgs = args
		return nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// Verify chdir was called with the target worktree path
	if chdirPath != dir {
		t.Errorf("chdir called with %q, want %q", chdirPath, dir)
	}

	// Verify runWt was called with correct args
	assertArgs(t, wtArgs, "switch", "wt-1")

	// Verify path printed to stdout
	got := strings.TrimSpace(stdout.String())
	if got != dir {
		t.Errorf("stdout = %q, want %q", got, dir)
	}
}

// TestDoNext_Recycle_ChdirBeforeGitRun is the regression test for the
// critical bug: git commands must run AFTER chdir to the target worktree,
// not before. Without the fix, git commands would run in the wrong worktree.
func TestDoNext_Recycle_ChdirBeforeGitRun(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
	}

	e, _ := testEnv(t, g, &mockGH{})

	var ops []string
	e.runWt = func(args ...string) error {
		ops = append(ops, "runWt")
		return nil
	}
	e.chdir = func(path string) error {
		ops = append(ops, "chdir:"+path)
		return nil
	}
	g.runFn = func(args []string) (string, error) {
		ops = append(ops, "git:"+args[0])
		return "", nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// Expected order: runWt, chdir, git:checkout (detach), git:branch (-D), git:checkout (-b)
	expected := []string{"runWt", "chdir:" + dir, "git:checkout", "git:branch", "git:checkout"}
	if len(ops) != len(expected) {
		t.Fatalf("ops = %v, want %v", ops, expected)
	}
	for i, op := range ops {
		if op != expected[i] {
			t.Errorf("ops[%d] = %q, want %q; full ops: %v", i, op, expected[i], ops)
		}
	}
}

func TestDoNext_Recycle_CorrectGitArgs(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
	}

	e, _ := testEnv(t, g, &mockGH{})
	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// NextNum([1]) = 2, so new branch is wt-2
	if len(g.runCalls) != 3 {
		t.Fatalf("expected 3 git Run calls, got %d: %v", len(g.runCalls), g.runCalls)
	}
	assertArgs(t, g.runCalls[0], "checkout", "-q", "origin/main")
	assertArgs(t, g.runCalls[1], "branch", "-D", "wt-1")
	assertArgs(t, g.runCalls[2], "checkout", "-q", "-b", "wt-2")
}

func TestDoNext_Recycle_RunWtFails(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
	}

	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		return fmt.Errorf("wt not found")
	}

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "wt switch wt-1") {
		t.Errorf("error = %q, want it to mention 'wt switch wt-1'", err)
	}

	// No git commands should have been executed
	if len(g.runCalls) != 0 {
		t.Errorf("expected no git Run calls, got %v", g.runCalls)
	}
}

func TestDoNext_Recycle_ChdirFails(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
	}

	e, _ := testEnv(t, g, &mockGH{})
	e.chdir = func(path string) error {
		return fmt.Errorf("no such directory")
	}

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "chdir") {
		t.Errorf("error = %q, want it to mention 'chdir'", err)
	}

	// No git commands should have been executed after chdir failure
	if len(g.runCalls) != 0 {
		t.Errorf("expected no git Run calls after chdir failure, got %v", g.runCalls)
	}
}

func TestDoNext_Recycle_CheckoutMainFails(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
		runFn: func(args []string) (string, error) {
			if len(args) >= 3 && args[0] == "checkout" && args[2] == "origin/main" {
				return "", fmt.Errorf("checkout failed: ref not found")
			}
			return "", nil
		},
	}

	e, _ := testEnv(t, g, &mockGH{})

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "checkout origin/main") {
		t.Errorf("error = %q, want it to mention 'checkout origin/main'", err)
	}
}

func TestDoNext_Recycle_BranchDeleteNonFatal(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
		runFn: func(args []string) (string, error) {
			if args[0] == "branch" && args[1] == "-D" {
				return "", fmt.Errorf("cannot delete branch")
			}
			return "", nil
		},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	// Should succeed despite branch -D failure
	if err := e.doNext(); err != nil {
		t.Fatalf("expected no error (branch delete is non-fatal), got: %v", err)
	}

	// All 3 git commands should still have been called
	if len(g.runCalls) != 3 {
		t.Fatalf("expected 3 git Run calls, got %d: %v", len(g.runCalls), g.runCalls)
	}

	// Path should still be printed
	if strings.TrimSpace(stdout.String()) != dir {
		t.Errorf("stdout = %q, want %q", stdout.String(), dir)
	}
}

func TestDoNext_Recycle_NewBranchFails(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain:   fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\n", dir),
		cleanPaths:    map[string]bool{dir: true},
		repoRoot:      dir,
		refs:          []string{"wt-1"},
		runFn: func(args []string) (string, error) {
			if args[0] == "checkout" && len(args) >= 4 && args[2] == "-b" {
				return "", fmt.Errorf("branch already exists")
			}
			return "", nil
		},
	}

	e, _ := testEnv(t, g, &mockGH{})

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "checkout -b") {
		t.Errorf("error = %q, want it to mention 'checkout -b'", err)
	}
}

func TestDoNext_Recycle_UsesFirstRecyclable(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(
			"worktree %s\nHEAD abc\nbranch refs/heads/wt-1\n\nworktree %s\nHEAD def\nbranch refs/heads/wt-2\n\n",
			dir1, dir2,
		),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
		repoRoot:   dir1,
		refs:       []string{"wt-1", "wt-2"},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	var chdirPath string
	e.chdir = func(path string) error {
		chdirPath = path
		return nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// Should use the first recyclable (order depends on FindRecyclable,
	// but chdir + stdout should match whichever was selected)
	got := strings.TrimSpace(stdout.String())
	if got != chdirPath {
		t.Errorf("stdout (%q) doesn't match chdir path (%q)", got, chdirPath)
	}
}

// --- Creation path ---

func TestDoNext_Create_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "myrepo")
	os.MkdirAll(repoRoot, 0755)

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      repoRoot,
		refs:          []string{},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	var chdirPath string
	e.chdir = func(path string) error {
		chdirPath = path
		return nil
	}

	var wtArgs []string
	e.runWt = func(args ...string) error {
		wtArgs = args
		return nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// NextNum([]) = 1, new branch = wt-1
	expectedPath := filepath.Join(tmpDir, "myrepo.wt-1")
	if chdirPath != expectedPath {
		t.Errorf("chdir = %q, want %q", chdirPath, expectedPath)
	}

	got := strings.TrimSpace(stdout.String())
	if got != expectedPath {
		t.Errorf("stdout = %q, want %q", got, expectedPath)
	}

	assertArgs(t, wtArgs, "switch", "-c", "wt-1", "--base", "origin/main")
}

func TestDoNext_Create_PathFromWorktree(t *testing.T) {
	tmpDir := t.TempDir()
	// Simulate running from an existing worktree: repoRoot has .wt-5 suffix
	repoRoot := filepath.Join(tmpDir, "myrepo.wt-5")
	os.MkdirAll(repoRoot, 0755)

	g := &mockGit{
		currentBranch: "wt-5",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      repoRoot,
		refs:          []string{"wt-5"},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	var chdirPath string
	e.chdir = func(path string) error {
		chdirPath = path
		return nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// Should strip .wt-5 from base name: myrepo.wt-5 → myrepo
	// NextNum([5]) = 1 (gap fill), so path = <parent>/myrepo.wt-1
	expectedPath := filepath.Join(tmpDir, "myrepo.wt-1")
	if chdirPath != expectedPath {
		t.Errorf("chdir = %q, want %q", chdirPath, expectedPath)
	}

	got := strings.TrimSpace(stdout.String())
	if got != expectedPath {
		t.Errorf("stdout = %q, want %q", got, expectedPath)
	}
}

func TestDoNext_Create_RunWtFails(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "myrepo")
	os.MkdirAll(repoRoot, 0755)

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      repoRoot,
		refs:          []string{},
	}

	e, _ := testEnv(t, g, &mockGH{})
	e.runWt = func(args ...string) error {
		return fmt.Errorf("wt failed")
	}

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "wt switch -c") {
		t.Errorf("error = %q, want it to mention 'wt switch -c'", err)
	}
}

func TestDoNext_Create_ChdirFails(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := filepath.Join(tmpDir, "myrepo")
	os.MkdirAll(repoRoot, 0755)

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      repoRoot,
		refs:          []string{},
	}

	e, _ := testEnv(t, g, &mockGH{})
	e.chdir = func(path string) error {
		return fmt.Errorf("directory does not exist")
	}

	err := e.doNext()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "chdir to new worktree") {
		t.Errorf("error = %q, want it to mention 'chdir to new worktree'", err)
	}
}

// --- General ---

func TestDoNext_NextNum_GapFilling(t *testing.T) {
	dir := t.TempDir()

	// Existing: wt-1, wt-3 → NextNum should fill gap with 2
	g := &mockGit{
		currentBranch: "main",
		merged:        []string{},
		wtPorcelain:   "",
		cleanPaths:    map[string]bool{},
		repoRoot:      dir,
		refs:          []string{"wt-1", "wt-3"},
	}

	e, stdout := testEnv(t, g, &mockGH{})

	var wtArgs []string
	e.runWt = func(args ...string) error {
		wtArgs = args
		return nil
	}

	if err := e.doNext(); err != nil {
		t.Fatal(err)
	}

	// NextNum([1,3]) = 2 (fills gap)
	if len(wtArgs) < 3 || wtArgs[2] != "wt-2" {
		t.Errorf("expected new branch wt-2, runWt args = %v", wtArgs)
	}

	out := strings.TrimSpace(stdout.String())
	if !strings.HasSuffix(out, ".wt-2") {
		t.Errorf("stdout = %q, expected path ending in .wt-2", out)
	}
}
