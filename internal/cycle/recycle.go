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
func FindRecyclable(d *Deps) ([]Recyclable, error) {
	// Get current branch to exclude
	currentBranch, err := d.Git.CurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("getting current branch: %w", err)
	}

	// Run fetch and GitHub lookup in parallel
	var fetchErr error
	var closedBranches []string
	var ghErr error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		fetchErr = d.Git.FetchOriginMain()
	}()

	go func() {
		defer wg.Done()
		closedBranches, ghErr = cachedClosedBranches(d)
	}()

	wg.Wait()

	if fetchErr != nil {
		d.Logf("warning: git fetch failed: %v", fetchErr)
		// Continue — merged branch detection still works with stale data
	}

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
		return nil, nil
	}

	// Get worktree list and map branches to paths
	wtOutput, err := d.Git.WorktreeListPorcelain()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}
	worktrees := git.ParseWorktreeList(wtOutput)
	byBranch := git.WorktreesByBranch(worktrees)

	// Filter candidates
	var result []Recyclable
	for branch := range candidateSet {
		if branch == currentBranch {
			if d.Verbose {
				d.Logf("skip %s: current branch", branch)
			}
			continue
		}

		wt, ok := byBranch[branch]
		if !ok {
			if d.Verbose {
				d.Logf("skip %s: no worktree", branch)
			}
			continue
		}

		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			if d.Verbose {
				d.Logf("skip %s: directory missing", branch)
			}
			continue
		}

		clean, err := d.Git.IsClean(wt.Path)
		if err != nil {
			if d.Verbose {
				d.Logf("skip %s: clean check failed: %v", branch, err)
			}
			continue
		}
		if !clean {
			if d.Verbose {
				d.Logf("skip %s: dirty working tree", branch)
			}
			continue
		}

		result = append(result, Recyclable{Branch: branch, Path: wt.Path})
	}

	return result, nil
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
