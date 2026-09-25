#!/usr/bin/env bash
# Creates a throwaway git repo with a mix of branches to try branch-cleaner on.
# Usage: scripts/demo-repo.sh [dir]   (default: /tmp/branch-cleaner-demo)
set -euo pipefail

dir="${1:-/tmp/branch-cleaner-demo}"
root="$(cd "$(dirname "$0")/.." && pwd)"
rm -rf "$dir" "$dir-remote.git"
git init -q --bare "$dir-remote.git"
git init -q -b main "$dir"
cd "$dir"
git remote add origin "$dir-remote.git"

commit() { echo "$1" >> log.txt; git add log.txt; git commit -qm "$1"; }

commit "initial"
git push -qu origin main
git remote set-head origin main

# Merged into main.
git switch -qc feature/login
commit "add login"
git switch -q main
git merge -q --no-ff feature/login -m "Merge feature/login"

# Pushed, then deleted on the remote (like a squash-merged PR) -> "gone".
git switch -qc fix/header-typo
commit "fix typo"
git push -qu origin fix/header-typo
git push -q origin --delete fix/header-typo

# Unmerged work in progress.
git switch -qc experiment/new-nav main
commit "try new nav"

git switch -qc chore/deps main
commit "bump deps"
git switch -q main
git merge -q --no-ff chore/deps -m "Merge chore/deps"

git fetch -q --prune
echo "Demo repo ready: cd $dir && $root/branch-cleaner"
