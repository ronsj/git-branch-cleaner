package git

import (
	"os"
	"path/filepath"
	"strings"
)

// branchesInProgress finds branches that a rebase or bisect has taken over in
// any worktree, this one included, mapped to "rebasing" or "bisecting". While
// that's happening the worktree's HEAD is detached, so %(worktreepath) doesn't
// report the branch, but git still refuses to delete it. Git records the
// branch in state files inside each worktree's git directory.
func branchesInProgress() (map[string]string, error) {
	common, err := git("rev-parse", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	// The path can be relative to the current directory.
	common, err = filepath.Abs(common)
	if err != nil {
		return nil, err
	}

	// The main worktree uses the common directory itself; linked worktrees
	// each get a directory under worktrees/.
	gitDirs := []string{common}
	linked, _ := filepath.Glob(filepath.Join(common, "worktrees", "*"))
	gitDirs = append(gitDirs, linked...)

	inProgress := make(map[string]string)
	for _, dir := range gitDirs {
		for _, file := range []string{"rebase-merge/head-name", "rebase-apply/head-name"} {
			if name, ok := strings.CutPrefix(readTrimmed(filepath.Join(dir, file)), "refs/heads/"); ok {
				inProgress[name] = "rebasing"
			}
		}
		// Holds the branch bisect started from (or a commit, if HEAD was detached).
		if name := readTrimmed(filepath.Join(dir, "BISECT_START")); name != "" {
			inProgress[name] = "bisecting"
		}
	}
	return inProgress, nil
}

// readTrimmed returns a file's contents without surrounding whitespace, or ""
// if it can't be read. A missing file is the normal case: nothing in progress.
// Any other read error is treated the same way, since git still refuses to
// delete the branch; the user just sees that error instead of a label.
func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
