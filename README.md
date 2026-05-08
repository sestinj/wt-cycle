<p align="center">
  <a href="https://continue.dev">
    <img src=".github/assets/continue-banner.png" width="800" alt="Continue" />
  </a>
</p>

<h1 align="center">🔁 wt-cycle</h1>

<p align="center">Git worktree lifecycle manager. Create, recycle, and clean numbered <code>wt-N</code> worktrees.</p>

<p align="center"><em>An autonomous codebase built by the <a href="https://continue.dev/blueprint">Continue Software Factory</a></em></p>

---

## 🤔 Why?

When working with multiple Claude Code sessions in parallel, each session needs its own worktree. `wt-cycle` automates the lifecycle of numbered `wt-N` worktrees -- creating new ones when needed, recycling merged ones to avoid clutter, and cleaning up stale ones in bulk.

## 📦 Install

**From release (recommended):**

```bash
curl -fsSL https://raw.githubusercontent.com/sestinj/wt-cycle/main/install.sh | sh
```

**From source:**

```bash
make install  # builds and copies to ~/.local/bin/
```

## 🚀 Usage

```bash
# Create or recycle a worktree (prints path to stdout)
wt-cycle next

# List worktrees with status
wt-cycle list

# Remove all recyclable worktrees
wt-cycle clean
```

### Flags

- `--verbose` / `-v` -- verbose output to stderr
- `--no-cache` -- bypass the GitHub API cache (5 min TTL)
- `--json` -- JSON output (for `list`)

## 🐚 Shell Integration

The `cc` fish function wraps `wt-cycle next`:

```fish
function cc
    set -l path (wt-cycle next)
    cd $path
    claude --dangerously-skip-permissions
end
```

## ⚙️ How It Works

A worktree is **recyclable** if:
1. Its branch matches `wt-N`
2. It's merged into `origin/main` OR its PR is closed/merged
3. Its directory exists with a clean working tree
4. It's not the current branch

`wt-cycle next` either recycles the first available worktree or creates a new one, delegating to [worktrunk](https://github.com/sestinj/worktrunk) (`wt switch`) for the actual worktree operations.

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and contribution guidelines.

## 📄 License

Apache-2.0 -- see [LICENSE](LICENSE) for details.
Copyright (c) 2025 Continue Dev, Inc.
