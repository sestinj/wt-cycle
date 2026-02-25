package cycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sestinj/wt-cycle/internal/cache"
	"github.com/sestinj/wt-cycle/internal/git"
	"github.com/sestinj/wt-cycle/internal/github"
)

// Recyclable represents a worktree branch that can be safely recycled.
type Recyclable struct {
	Branch string
	Path   string
}

// Skipped represents a candidate that was not recyclable.
type Skipped struct {
	Branch string
	Path   string
	Reason string // "current", "no-worktree", "missing-dir", "dirty", "check-failed"
}

// FindResult holds both recyclable and skipped candidates.
type FindResult struct {
	Recyclable []Recyclable
	Skipped    []Skipped
}

// Deps bundles the dependencies for the cycle logic.
type Deps struct {
	Git    git.Client
	GitHub github.Client
	Cache  *cache.Cache

	NoCache bool
	Verbose bool
	Logf    func(format string, args ...interface{}) // writes to stderr
}

// FindRecyclable returns worktree branches that are safe to recycle.
// A branch is recyclable if:
// 1. It matches wt-N pattern
// 2. It's merged into origin/main OR its PR is closed/merged
// 3. Its worktree directory exists
// 4. Its worktree is clean (no uncommitted changes)
// 5. It's not the current branch
func FindRecyclable(d *Deps) (*FindResult, error) {
	// Get current branch to exclude
	currentBranch, err := d.Git.CurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("getting current branch: %w", err)
	}

	// Fire-and-forget fetch — use stale origin/main for this invocation.
	// The data is at most a few minutes old; next call will see the update.
	go func() {
		if err := d.Git.FetchOriginMain(); err != nil && d.Verbose {
			d.Logf("warning: background git fetch failed: %v", err)
		}
	}()

	// GitHub lookup (cached)
	var closedBranches []string
	var ghErr error
	closedBranches, ghErr = cachedClosedBranches(d)

	// Get merged branches (after fetch)
	merged, err := d.Git.MergedBranches("wt-*")
	if err != nil {
		return nil, fmt.Errorf("listing merged branches: %w", err)
	}
	mergedBranches := git.FilterWtBranches(merged)

	// Union merged + closed PR branches
	candidateSet := make(map[string]struct{})
	for _, b := range mergedBranches {
		candidateSet[b] = struct{}{}
	}
	if ghErr != nil {
		d.Logf("warning: GitHub API failed: %v", ghErr)
	} else {
		for _, b := range git.FilterWtBranches(closedBranches) {
			candidateSet[b] = struct{}{}
		}
	}

	if len(candidateSet) == 0 {
		return &FindResult{}, nil
	}

	// Get worktree list and map branches to paths
	wtOutput, err := d.Git.WorktreeListPorcelain()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	worktrees := git.ParseWorktreeList(wtOutput)
	byBranch := git.WorktreesByBranch(worktrees)

	// Pre-filter candidates that have existing worktree directories (cheap checks first)
	type candidate struct {
		branch string
		path   string
	}
	var toCheck []candidate
	var skipped []Skipped
	for branch := range candidateSet {
		if branch == currentBranch {
			if d.Verbose {
				d.Logf("skip %s: current branch", branch)
			}
			skipped = append(skipped, Skipped{Branch: branch, Reason: "current"})
			continue
		}

		wt, ok := byBranch[branch]
		if !ok {
			if d.Verbose {
				d.Logf("skip %s: no worktree", branch)
			}
			skipped = append(skipped, Skipped{Branch: branch, Reason: "no-worktree"})
			continue
		}

		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			if d.Verbose {
				d.Logf("skip %s: directory missing", branch)
			}
			skipped = append(skipped, Skipped{Branch: branch, Path: wt.Path, Reason: "missing-dir"})
			continue
		}

		toCheck = append(toCheck, candidate{branch: branch, path: wt.Path})
	}

	// Parallel IsClean checks — this is the expensive part (~120ms each)
	type cleanResult struct {
		candidate
		clean bool
		err   error
	}
	results := make([]cleanResult, len(toCheck))
	var cleanWg sync.WaitGroup
	sem := make(chan struct{}, 16) // bound concurrency
	for i, c := range toCheck {
		cleanWg.Add(1)
		go func(i int, c candidate) {
			defer cleanWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			clean, err := d.Git.IsClean(c.path)
			results[i] = cleanResult{candidate: c, clean: clean, err: err}
		}(i, c)
	}
	cleanWg.Wait()

	var recyclable []Recyclable
	for _, r := range results {
		if r.err != nil {
			if d.Verbose {
				d.Logf("skip %s: clean check failed: %v", r.branch, r.err)
			}
			skipped = append(skipped, Skipped{Branch: r.branch, Path: r.path, Reason: "check-failed"})
			continue
		}
		if !r.clean {
			if d.Verbose {
				d.Logf("skip %s: dirty working tree", r.branch)
			}
			skipped = append(skipped, Skipped{Branch: r.branch, Path: r.path, Reason: "dirty"})
			continue
		}
		recyclable = append(recyclable, Recyclable{Branch: r.branch, Path: r.path})
	}

	return &FindResult{Recyclable: recyclable, Skipped: skipped}, nil
}

// CollectExistingNums gathers all existing wt-N numbers from refs and worktree directories.
func CollectExistingNums(d *Deps) ([]int, error) {
	repoRoot, err := d.Git.RepoRoot()
	if err != nil {
		return nil, err
	}

	refs, err := d.Git.ForEachRef("refs/heads/wt-*", "refs/remotes/origin/wt-*")
	if err != nil {
		return nil, err
	}

	var nums []int
	for _, ref := range refs {
		ref = strings.TrimPrefix(ref, "origin/")
		if n := git.ExtractWtNum(ref); n >= 0 {
			nums = append(nums, n)
		}
	}

	// Scan sibling directories
	repoParent := filepath.Dir(repoRoot)
	baseName := filepath.Base(repoRoot)
	// Strip .wt-N suffix from base name to get the root repo name
	if idx := strings.Index(baseName, ".wt-"); idx != -1 {
		baseName = baseName[:idx]
	}

	entries, _ := os.ReadDir(repoParent)
	for _, e := range entries {
		name := e.Name()
		prefix := baseName + ".wt-"
		if strings.HasPrefix(name, prefix) {
			rest := strings.TrimPrefix(name, prefix)
			if n := git.ExtractWtNum("wt-" + rest); n >= 0 {
				nums = append(nums, n)
			}
		}
	}

	return nums, nil
}

func cachedClosedBranches(d *Deps) ([]string, error) {
	cacheKey := "pr-states"

	if !d.NoCache && d.Cache != nil {
		if data := d.Cache.Get(cacheKey); data != nil {
			var branches []string
			if err := json.Unmarshal(data, &branches); err == nil {
				if d.Verbose {
					d.Logf("using cached PR data (%d branches)", len(branches))
				}
				return branches, nil
			}
		}
	}

	branches, err := d.GitHub.ClosedPRBranches()
	if err != nil {
		return nil, err
	}

	if d.Cache != nil {
		data, _ := json.Marshal(branches)
		d.Cache.Set(cacheKey, data)
	}

	return branches, nil
}
