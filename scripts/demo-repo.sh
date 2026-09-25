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

# at <days-ago>: backdate the next commit or merge by that many days.
at() {
  local when="$(( $(date +%s) - $1 * 86400 )) +0000"
  export GIT_AUTHOR_DATE="$when" GIT_COMMITTER_DATE="$when"
}

# commit <days-ago> <message> [author]: author defaults to your git config.
commit() {
  at "$1"
  echo "$2" >> log.txt
  git add log.txt
  if [ -n "${3:-}" ]; then
    git commit -qm "$2" --author "$3 <${3// /.}@example.com>"
  else
    git commit -qm "$2"
  fi
}

# merge <days-ago> <branch>: merge branch into main.
merge() {
  at "$1"
  git switch -q main
  git merge -q --no-ff "$2" -m "Merge $2"
}

commit 500 "initial"
git push -qu origin main
git remote set-head origin main

# Abandoned long ago, never merged.
git switch -qc spike/graphql-api
commit 430 "Spike: expose branches over GraphQL" "Jordan Park"

# Merged into main.
git switch -qc feature/login main
commit 75 "Add login form with email validation" "Alex Kim"
merge 70 feature/login

git switch -qc chore/deps main
commit 45 "Bump dependencies" "Alex Kim"
merge 44 chore/deps

# Pushed, then deleted on the remote (like a squash-merged PR) -> "gone".
git switch -qc fix/header-typo main
commit 20 "Fix typo in header" "Sam Lee"
git push -qu origin fix/header-typo
git push -q origin --delete fix/header-typo

# Recent, unmerged work in progress.
git switch -qc experiment/new-nav main
commit 3 "Try a sidebar layout for the new navigation menu" "Priya Natarajan"

git switch -q main

git fetch -q --prune
echo "Demo repo ready: cd $dir && $root/branch-cleaner"
