# wt-cycle

Git worktree lifecycle manager. Create, recycle, and clean numbered `wt-N` worktrees.

## Install

```bash
make install  # builds and copies to ~/.local/bin/
```

## Usage

```bash
# Create or recycle a worktree (prints path to stdout)
wt-cycle next

# List worktrees with status
wt-cycle list

# Remove all recyclable worktrees
wt-cycle clean
```

### Flags

- `--verbose` / `-v` — verbose output to stderr
- `--no-cache` — bypass the GitHub API cache (5 min TTL)
- `--json` — JSON output (for `list`)

## Shell Integration

The `cc` fish function wraps `wt-cycle next`:

```fish
function cc
    set -l path (wt-cycle next)
    cd $path
    claude --dangerously-skip-permissions
end
```

## How It Works

A worktree is **recyclable** if:
1. Its branch matches `wt-N`
2. It's merged into `origin/main` OR its PR is closed/merged
3. Its directory exists with a clean working tree
4. It's not the current branch

`wt-cycle next` either recycles the first available worktree or creates a new one, delegating to [worktrunk](https://github.com/sestinj/worktrunk) (`wt switch`) for the actual worktree operations.
