# git-branch-cleaner

A terminal UI for finding and deleting stale local git branches, built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles), and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

```
Git Branch Cleaner  base: main · oldest first

  [x] chore/deps          merged
      3 weeks ago    Alex Kim         Bump dependencies
> [ ] experiment/new-nav
      2 weeks ago    Priya Natarajan  Try a sidebar layout for the nav
  [x] feature/login       merged
      6 days ago     Alex Kim         Add login form with email validation
  [x] fix/header-typo     gone
      2 days ago     Sam Lee          Fix typo in header
   -  main                current base
      5 minutes ago  Ronald San Jose  Merge chore/deps

3 selected
space toggle • a select merged/gone • enter delete • / filter • ? more • q quit
```

## Requirements

- Go 1.27.1 or newer
- git 2.23 or newer

## Install

```sh
go install github.com/ronsj/git-branch-cleaner@latest
```

Or, from a clone of this repo, `go install .`

Either way, this installs `git-branch-cleaner` into `$(go env GOPATH)/bin`, or
into `$GOBIN` if you've set it. Add that directory to your `PATH` if it isn't
already there. You can also build a binary in place with `go build .`.

## Uninstall

Delete the installed binary:

```sh
rm "$(go env GOPATH)/bin/git-branch-cleaner"
```

If you've set `GOBIN`, `go install` put the binary there instead:

```sh
rm "$(go env GOBIN)/git-branch-cleaner"
```

If you created the [demo repo](#try-it-on-a-demo-repo), remove its folder:

```sh
rm -rf /tmp/git-branch-cleaner-demo
```

## Usage

Run it from anywhere inside a git repository:

```sh
cd path/to/your/repo
git-branch-cleaner
```

Git runs any `git-<name>` program on your `PATH` as a subcommand, so
`git branch-cleaner` works too. For help, use `-h`: git turns
`git branch-cleaner --help` into a request for a man page, which this tool
doesn't install.

Branches are listed oldest first, so the stalest ones are at the top; press
`s` to switch to newest first or by name. Under each branch's name and status
is its last commit: when it was made, who wrote it, and its message. Lines
that don't fit your terminal are cut off with `…`.

### Flags

| Flag | Effect |
| --- | --- |
| `--dry-run` | Show what would be deleted without deleting anything |
| `--older-than N` | Hide branches whose last commit is less than `N` days old |
| `--base branch` | Compare against `branch` instead of detecting the base branch. It can be a remote-tracking branch, such as `origin/main` |
| `--version` | Print the version and exit |

With `--dry-run`, the whole UI works the same, with a **DRY RUN** badge in the
title. Confirming a delete lists each branch and the commit it points to
instead of deleting it.

`--older-than` is handy for skipping work that's still in progress:

```sh
git-branch-cleaner --older-than 30
```

The header shows how many branches it's hiding (for example
`2 newer than 30 days hidden`). Flags can be combined, and
`git-branch-cleaner -h` lists them all.

### Keys

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move the cursor |
| `space` / `x` | Select or deselect the branch under the cursor |
| `a` | Select every branch marked `merged` or `gone` |
| `n` | Clear the selection |
| `/` | Filter branches by name |
| `esc` | Clear the filter |
| `enter` / `d` | Delete the selected branches (asks for confirmation) |
| `s` | Change sort order: oldest first, newest first, by name |
| `r` | Reload the branch list |
| `?` | Show all keys |
| `q` / `ctrl+c` | Quit |

On the confirmation screen, press `y` to delete or `n` / `esc` to go back.

### Filtering

Press `/` and type to show only branches whose names contain that text
(case-insensitive). While you're typing, letter keys go into the filter, so use
`↑` / `↓` to move. Press `enter` to keep the filter and get your shortcuts
back, or `esc` to clear it.

With a filter applied, `a` only selects matching branches. Branches you
selected before filtering stay selected; the footer shows how many are hidden
(for example `3 selected (1 hidden by filter)`), and the confirmation screen
lists every selected branch, hidden or not.

### Labels

| Label | Meaning |
| --- | --- |
| `merged` | Every commit on the branch is already in the base branch, or the same changes are, as after a rebase or squash merge. Safe to delete. Also shown on branches that can't be selected yet, such as `worktree merged`: remove that worktree and the branch is safe to delete. |
| `gone` | The branch tracked a remote branch that has since been deleted, typically after a PR was merged. This catches merged branches that `merged` misses (see below). |
| `current` | The branch you have checked out. It can't be selected. |
| `base` | The branch everything is compared against. With a remote-tracking base such as `origin/main`, it's the local branch that tracks it. It can't be selected. |
| `worktree` | Checked out in another [worktree](https://git-scm.com/docs/git-worktree). Git won't delete it, so it can't be selected. |
| `rebasing` / `bisecting` | A rebase or bisect in some worktree is using the branch. Git won't delete it until that finishes, so it can't be selected. |

The base branch is the remote's default branch (`origin/HEAD`) if it exists
locally, otherwise `main`, then `master`, then whatever is checked out. If your
repo's main line has another name, such as `develop`, pass it with
`--base develop`.

Branches merged on the remote, through a pull request say, only show as
`merged` once your local `main` has caught up. To skip pulling, compare
against the remote's copy instead: run `git fetch`, then
`git-branch-cleaner --base origin/main`.

Rebase and squash merges put new commits on `main` instead of yours, so git
doesn't see the branch as merged. git-branch-cleaner still marks it `merged`
when `main` has the same changes: an identical copy of every commit on the
branch (a rebase merge), or one commit making the branch's whole change (a
squash merge).

It can miss a merge that changed things along the way: when `main` had
changed lines right next to yours, say, or the pull request was edited as it
was merged. Those branches show as `gone` instead, once their remote branch
has been deleted (GitHub can do this automatically after a merge) and you've
fetched.

The `gone` label depends on your local copy of the remote. Run
`git fetch --prune` first so branches deleted on the remote are detected.

If `a` finds no `merged` or `gone` branches, it says so below the list.

## Recovering a deleted branch

Branches are deleted with `git branch -D`. If any selected branch isn't merged
into the base branch, the confirmation screen warns you before anything is
deleted.

When you quit, git-branch-cleaner prints a restore command for each branch it
deleted, and the reason for any it didn't:

```
Deleted branch fix/header-typo (was c6d677e).
  restore: git branch fix/header-typo c6d677e4b1f0a9d2e3c5b7a8f9e0d1c2b3a4f5e6
Didn't delete feature/login: it changed since you selected it
```

Run the restore command to bring a branch back. If you've lost the output,
`git reflog` still has the commits for a while.

## Try it on a demo repo

To try git-branch-cleaner without touching real work, use `--dry-run` or create
a throwaway repo with merged, gone, unmerged, and worktree branches:

```sh
go build .
scripts/demo-repo.sh                  # defaults to /tmp/git-branch-cleaner-demo
cd /tmp/git-branch-cleaner-demo/repo && ~/path/to/git-branch-cleaner
```

The folder holds the repo (`repo/`), a fake remote (`remote.git/`), and a
second worktree (`release/`). Run the script again at any time to reset it. To
protect your files, the script only replaces a folder it created itself (or an
empty one) and refuses any other path.

## Development

```sh
go test ./...
go vet ./...
```

CI (`.github/workflows/ci.yml`) also checks formatting with `gofmt` and runs
[staticcheck](https://staticcheck.dev) and
[govulncheck](https://go.dev/doc/tutorial/govulncheck).

| Path | Contents |
| --- | --- |
| `main.go` | Entry point; calls `cmd.Execute` |
| `cmd/root.go` | Flags, running the UI, and the summary printed on exit |
| `internal/git/` | Everything that runs git: loading branches (`load.go`), protection rules (`branch.go`), rebase and bisect detection (`inprogress.go`), deleting (`delete.go`), and restore commands (`restore.go`) |
| `internal/tui/` | The Bubble Tea UI: state (`model.go`), key handling (`update.go`), rendering (`view.go`, `list.go`, `confirm.go`), sorting, keys, and styles |
| `internal/testrepo/` | Throwaway git repos for tests |
| `scripts/demo-repo.sh` | Builds a demo repo to try the tool on |

Tests sit next to the code they test, and most git tests run against real
temporary repositories.
