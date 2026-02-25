package cycle

import (
	"fmt"
	"os"
	"testing"
)

// mockGit implements git.Client for testing.
type mockGit struct {
	currentBranch string
	merged        []string
	wtPorcelain   string
	refs          []string
	cleanPaths    map[string]bool // path -> isClean
	repoRoot      string
}

func (m *mockGit) FetchOriginMain() error                     { return nil }
func (m *mockGit) MergedBranches(_ string) ([]string, error)  { return m.merged, nil }
func (m *mockGit) WorktreeListPorcelain() (string, error)     { return m.wtPorcelain, nil }
func (m *mockGit) ForEachRef(_ ...string) ([]string, error)   { return m.refs, nil }
func (m *mockGit) CurrentBranch() (string, error)             { return m.currentBranch, nil }
func (m *mockGit) RepoRoot() (string, error)                  { return m.repoRoot, nil }
func (m *mockGit) Run(_ ...string) (string, error)            { return "", nil }
func (m *mockGit) IsClean(path string) (bool, error) {
	clean, ok := m.cleanPaths[path]
	if !ok {
		return false, fmt.Errorf("unknown path: %s", path)
	}
	return clean, nil
}

// mockGH implements github.Client for testing.
type mockGH struct {
	branches []string
	err      error
}

func (m *mockGH) ClosedPRBranches() ([]string, error) { return m.branches, m.err }

func nopLogf(string, ...interface{}) {}

func TestFindRecyclable_Basic(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "wt-5",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(`worktree %s
HEAD abc
branch refs/heads/wt-1

worktree %s
HEAD def
branch refs/heads/wt-2

worktree /nonexistent
HEAD 000
branch refs/heads/wt-3

`, dir1, dir2),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
	}

	gh := &mockGH{branches: []string{"wt-2", "wt-3"}}

	d := &Deps{Git: g, GitHub: gh, Logf: nopLogf}
	result, err := FindRecyclable(d)
	if err != nil {
		t.Fatal(err)
	}

	// Should get wt-1 and wt-2 (not wt-3: dir doesn't exist, not wt-5: current branch)
	if len(result.Recyclable) != 2 {
		t.Fatalf("expected 2 recyclable, got %d: %+v", len(result.Recyclable), result.Recyclable)
	}

	branches := map[string]bool{}
	for _, r := range result.Recyclable {
		branches[r.Branch] = true
	}
	if !branches["wt-1"] || !branches["wt-2"] {
		t.Errorf("expected wt-1 and wt-2, got %v", result.Recyclable)
	}
}

func TestFindRecyclable_SkipsDirty(t *testing.T) {
	dirClean := t.TempDir()
	dirDirty := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1", "wt-2"},
		wtPorcelain: fmt.Sprintf(`worktree %s
HEAD abc
branch refs/heads/wt-1

worktree %s
HEAD def
branch refs/heads/wt-2

`, dirClean, dirDirty),
		cleanPaths: map[string]bool{dirClean: true, dirDirty: false},
	}

	d := &Deps{Git: g, GitHub: &mockGH{}, Logf: nopLogf}
	result, err := FindRecyclable(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Recyclable) != 1 || result.Recyclable[0].Branch != "wt-1" {
		t.Fatalf("expected only wt-1, got %+v", result.Recyclable)
	}
}

func TestFindRecyclable_SkipsCurrentBranch(t *testing.T) {
	dir := t.TempDir()

	g := &mockGit{
		currentBranch: "wt-1",
		merged:        []string{"wt-1"},
		wtPorcelain: fmt.Sprintf(`worktree %s
HEAD abc
branch refs/heads/wt-1

`, dir),
		cleanPaths: map[string]bool{dir: true},
	}

	d := &Deps{Git: g, GitHub: &mockGH{}, Logf: nopLogf}
	result, err := FindRecyclable(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Recyclable) != 0 {
		t.Fatalf("expected 0 recyclable (current branch), got %+v", result.Recyclable)
	}
}

func TestFindRecyclable_MissingDir(t *testing.T) {
	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"},
		wtPorcelain: `worktree /tmp/nonexistent-wt-cycle-test
HEAD abc
branch refs/heads/wt-1

`,
		cleanPaths: map[string]bool{},
	}

	d := &Deps{Git: g, GitHub: &mockGH{}, Logf: nopLogf}
	result, err := FindRecyclable(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Recyclable) != 0 {
		t.Fatalf("expected 0 recyclable (missing dir), got %+v", result.Recyclable)
	}
}

func TestFindRecyclable_UnionMergedAndGH(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	g := &mockGit{
		currentBranch: "main",
		merged:        []string{"wt-1"}, // only wt-1 is locally merged
		wtPorcelain: fmt.Sprintf(`worktree %s
HEAD abc
branch refs/heads/wt-1

worktree %s
HEAD def
branch refs/heads/wt-2

`, dir1, dir2),
		cleanPaths: map[string]bool{dir1: true, dir2: true},
	}

	// wt-2 was squash-merged (PR closed), not in git merged list
	gh := &mockGH{branches: []string{"wt-2"}}

	d := &Deps{Git: g, GitHub: gh, Logf: nopLogf}
	result, err := FindRecyclable(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Recyclable) != 2 {
		t.Fatalf("expected 2 recyclable (union), got %d: %+v", len(result.Recyclable), result.Recyclable)
	}
}

func TestCollectExistingNums(t *testing.T) {
	// Create a fake directory structure
	tmpDir := t.TempDir()
	repoDir := tmpDir + "/myrepo.wt-5"
	os.MkdirAll(repoDir, 0755)
	// Create sibling dirs
	os.MkdirAll(tmpDir+"/myrepo.wt-1", 0755)
	os.MkdirAll(tmpDir+"/myrepo.wt-3", 0755)
	os.MkdirAll(tmpDir+"/other-repo.wt-99", 0755) // should be ignored

	g := &mockGit{
		repoRoot: repoDir,
		refs:     []string{"wt-1", "wt-5", "origin/wt-7"},
	}

	d := &Deps{Git: g, Logf: nopLogf}
	nums, err := CollectExistingNums(d)
	if err != nil {
		t.Fatal(err)
	}

	// From refs: wt-1 (1), wt-5 (5), origin/wt-7 -> wt-7 (7)
	// From dirs: myrepo.wt-1 (1), myrepo.wt-3 (3)
	// Unique: 1, 3, 5, 7
	numSet := map[int]bool{}
	for _, n := range nums {
		numSet[n] = true
	}

	for _, expected := range []int{1, 3, 5, 7} {
		if !numSet[expected] {
			t.Errorf("expected %d in nums, got %v", expected, nums)
		}
	}

	if numSet[99] {
		t.Error("should not include 99 (different repo)")
	}
}
