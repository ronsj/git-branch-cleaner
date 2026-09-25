# branch-cleaner

A terminal UI for finding and deleting stale local git branches, built with
[Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Bubbles](https://github.com/charmbracelet/bubbles), and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

```
Branch Cleaner  base: main

  [x] chore/deps          3 weeks ago    merged        Alex Kim         Bump dependencies
> [ ] experiment/new-nav  2 weeks ago                  Priya Natarajan  Try a sidebar layout for th…
  [x] feature/login       6 days ago     merged        Alex Kim         Add login form with email v…
  [x] fix/header-typo     2 days ago     gone          Sam Lee          Fix typo in header
   -  main                5 minutes ago  current base  Ronald San Jose  Merge chore/deps

3 selected
space toggle • a select merged/gone • enter delete • / filter • ? more • q quit
```

## Requirements

- Go 1.27.1 or newer
- git

## Install

From the repo root:

```sh
go install .
```

This installs `branch-cleaner` into `$(go env GOPATH)/bin`, or into `$GOBIN`
if you've set it. Add that directory
to your `PATH` if it isn't already there. You can also build a binary in place
with `go build -o branch-cleaner .`.

## Uninstall

Delete the installed binary:

```sh
rm "$(go env GOPATH)/bin/branch-cleaner"
```

If you've set `GOBIN`, `go install` put the binary there instead:

```sh
rm "$(go env GOBIN)/branch-cleaner"
```

If you created the [demo repo](#try-it-on-a-demo-repo), remove it and its
fake remote:

```sh
rm -rf /tmp/branch-cleaner-demo /tmp/branch-cleaner-demo-remote.git
```

## Usage

Run it from anywhere inside a git repository:

```sh
cd path/to/your/repo
branch-cleaner
```

To see what would be deleted without deleting anything, add `--dry-run`:

```sh
branch-cleaner --dry-run
```

The whole UI works the same, with a **DRY RUN** badge in the title. Confirming
a delete lists each branch and the commit it points to instead of deleting it.
Run `branch-cleaner -h` to see all flags.

Branches are listed oldest first, so the stalest ones are at the top. Each row
shows the branch's last commit: when it was made, who wrote it, and its
message. Rows that don't fit your terminal are cut off with `…`.

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
| `merged` | Every commit on the branch is already in the base branch. Safe to delete. |
| `gone` | The branch tracked a remote branch that has since been deleted, typically after a PR was merged. Squash-merged branches show up this way, because git doesn't see their commits in the base branch. |
| `current` | The branch you have checked out. It can't be selected. |
| `base` | The branch everything is compared against. It can't be selected. |

The base branch is the remote's default branch (`origin/HEAD`) if it exists
locally, otherwise `main`, then `master`, then whatever is checked out.

The `gone` label depends on your local copy of the remote. Run
`git fetch --prune` first so branches deleted on the remote are detected.

## Recovering a deleted branch

Branches are deleted with `git branch -D`. If any selected branch isn't merged
into the base branch, the confirmation screen warns you before anything is
deleted.

When you quit, branch-cleaner prints a restore command for each branch it
deleted:

```
Deleted branch fix/header-typo (was c6d677e).
  restore: git branch fix/header-typo c6d677e
```

Run the restore command to bring a branch back. If you've lost the output,
`git reflog` still has the commits for a while.

## Try it on a demo repo

To try branch-cleaner without touching real work, use `--dry-run` or create
a throwaway repo with merged, gone, and unmerged branches:

```sh
go build -o branch-cleaner .
scripts/demo-repo.sh                  # defaults to /tmp/branch-cleaner-demo
cd /tmp/branch-cleaner-demo && ~/path/to/branch-cleaner
```

Run the script again at any time to reset the demo repo.

## Development

```sh
go test ./...
go vet ./...
```

| File | Contents |
| --- | --- |
| `main.go` | Starts the program and prints restore commands on exit |
| `model.go` | Bubble Tea model: state, `Update`, and `View` |
| `git.go` | Runs git commands and parses their output |
| `keys.go` | Key bindings and help text |
| `styles.go` | Lip Gloss styles |
