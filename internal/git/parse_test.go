package git

import (
	"testing"
)

func TestParseWorktreeList(t *testing.T) {
	input := `worktree /Users/nate/gh/repo
HEAD abc123
branch refs/heads/main
bare

worktree /Users/nate/gh/repo.wt-1
HEAD def456
branch refs/heads/wt-1

worktree /Users/nate/gh/repo.wt-2
HEAD 789abc
branch refs/heads/wt-2

worktree /Users/nate/gh/repo.wt-detached
HEAD 000000
detached

`
	wts := ParseWorktreeList(input)
	if len(wts) != 4 {
		t.Fatalf("expected 4 worktrees, got %d", len(wts))
	}

	tests := []struct {
		idx    int
		path   string
		branch string
		bare   bool
	}{
		{0, "/Users/nate/gh/repo", "main", true},
		{1, "/Users/nate/gh/repo.wt-1", "wt-1", false},
		{2, "/Users/nate/gh/repo.wt-2", "wt-2", false},
		{3, "/Users/nate/gh/repo.wt-detached", "", false},
	}

	for _, tt := range tests {
		wt := wts[tt.idx]
		if wt.Path != tt.path {
			t.Errorf("wt[%d].Path = %q, want %q", tt.idx, wt.Path, tt.path)
		}
		if wt.Branch != tt.branch {
			t.Errorf("wt[%d].Branch = %q, want %q", tt.idx, wt.Branch, tt.branch)
		}
		if wt.Bare != tt.bare {
			t.Errorf("wt[%d].Bare = %v, want %v", tt.idx, wt.Bare, tt.bare)
		}
	}
}

func TestParseWorktreeListNoTrailingNewline(t *testing.T) {
	input := `worktree /Users/nate/gh/repo
HEAD abc123
branch refs/heads/main`

	wts := ParseWorktreeList(input)
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Branch != "main" {
		t.Errorf("Branch = %q, want %q", wts[0].Branch, "main")
	}
}

func TestExtractWtNum(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"wt-1", 1},
		{"wt-42", 42},
		{"wt-0", 0},
		{"wt-999", 999},
		{"main", -1},
		{"wt-", -1},
		{"wt-abc", -1},
		{"wt-1-feature", -1},
		{"feature-wt-1", -1},
	}

	for _, tt := range tests {
		got := ExtractWtNum(tt.name)
		if got != tt.want {
			t.Errorf("ExtractWtNum(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestFilterWtBranches(t *testing.T) {
	input := []string{"main", "wt-1", "feature", "wt-42", "wt-abc", "wt-3"}
	got := FilterWtBranches(input)
	want := []string{"wt-1", "wt-42", "wt-3"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, g := range got {
		if g != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, g, want[i])
		}
	}
}

func TestWorktreesByBranch(t *testing.T) {
	wts := []Worktree{
		{Path: "/a", Branch: "wt-1"},
		{Path: "/b", Branch: "wt-2"},
		{Path: "/c", Branch: ""}, // detached
	}
	m := WorktreesByBranch(wts)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["wt-1"].Path != "/a" {
		t.Errorf("wt-1 path = %q, want /a", m["wt-1"].Path)
	}
	if m["wt-2"].Path != "/b" {
		t.Errorf("wt-2 path = %q, want /b", m["wt-2"].Path)
	}
}
