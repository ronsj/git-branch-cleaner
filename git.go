package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Branch is one local git branch plus the metadata shown in the UI.
type Branch struct {
	Name       string
	LastCommit string    // relative date for display, e.g. "3 weeks ago"
	CommitTime time.Time // exact date, for sorting and age checks
	Current    bool      // checked out right now
	Worktree   string    // path of the worktree that has it checked out, if any
	InProgress string    // "rebasing" or "bisecting" if a worktree is doing that to it
	Gone       bool      // upstream was deleted on the remote (often a squash-merged PR)
	Merged     bool      // fully merged into the base branch
	Author     string    // author of the last commit
	Subject    string    // first line of the last commit message
}

// InOtherWorktree reports whether the branch is checked out in a worktree
// other than this one. Git refuses to delete those.
func (b Branch) InOtherWorktree() bool {
	return b.Worktree != "" && !b.Current
}

// Protected reports whether the UI should refuse to delete this branch.
func (b Branch) Protected(base string) bool {
	return b.Current || b.Name == base || b.InOtherWorktree() || b.InProgress != ""
}

// deleteResult records the outcome of deleting one branch. Git prints
// "Deleted branch foo (was abc1234)." on success, which is kept so the
// branch can be restored later.
type deleteResult struct {
	Name   string
	Output string
	Err    error
}

// git runs a git subcommand in the current directory and returns its stdout.
func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	// Only strip the trailing newline: fields inside the output can
	// legitimately end in spaces.
	return strings.TrimRight(string(out), "\r\n"), nil
}

// The fields are tab-separated (%09) so branch names never collide with the
// separator. The commit subject goes last because it's free text: splitting
// into at most branchFields parts keeps any tabs inside it intact.
const branchFormat = "%(refname:short)%09%(committerdate:relative)%09%(committerdate:unix)%09%(HEAD)%09%(upstream:track)%09%(worktreepath)%09%(authorname)%09%(contents:subject)"

const branchFields = 8

// parseBranches turns `git for-each-ref --format=branchFormat` output into Branches.
func parseBranches(out string) []Branch {
	var branches []Branch
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", branchFields)
		if len(fields) < branchFields {
			continue
		}
		// A bad timestamp parses as 0, i.e. 1970, so the branch just looks
		// very old; not worth dropping the branch over.
		unix, _ := strconv.ParseInt(fields[2], 10, 64)
		branches = append(branches, Branch{
			Name:       fields[0],
			LastCommit: fields[1],
			CommitTime: time.Unix(unix, 0),
			Current:    fields[3] == "*",
			Gone:       fields[4] == "[gone]",
			Worktree:   fields[5],
			Author:     fields[6],
			Subject:    fields[7],
		})
	}
	return branches
}

// baseBranch guesses the repo's main line: origin's default branch if known,
// then main or master, then whatever is checked out.
func baseBranch() string {
	var candidates []string
	if ref, err := git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		candidates = append(candidates, strings.TrimPrefix(ref, "origin/"))
	}
	candidates = append(candidates, "main", "master")

	for _, name := range candidates {
		if _, err := git("rev-parse", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	current, _ := git("branch", "--show-current")
	return current
}

// loadBranches lists local branches and marks which are merged into base.
// The UI decides the order (see sortBranches).
func loadBranches() (base string, branches []Branch, err error) {
	out, err := git("for-each-ref", "--format="+branchFormat, "refs/heads/")
	if err != nil {
		return "", nil, err
	}
	branches = parseBranches(out)

	base = baseBranch()
	if base == "" {
		return "", branches, nil
	}

	merged, err := git("branch", "--merged", base, "--format=%(refname:short)")
	if err != nil {
		return "", nil, err
	}
	isMerged := make(map[string]bool)
	for name := range strings.SplitSeq(merged, "\n") {
		isMerged[strings.TrimSpace(name)] = true
	}
	for i := range branches {
		branches[i].Merged = isMerged[branches[i].Name]
	}

	inProgress, err := branchesInProgress()
	if err != nil {
		return "", nil, err
	}
	for i := range branches {
		branches[i].InProgress = inProgress[branches[i].Name]
	}
	return base, branches, nil
}

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

// restoreSHA extracts the commit from git's "Deleted branch foo (was abc1234)." message.
func restoreSHA(output string) string {
	i := strings.LastIndex(output, "(was ")
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(output[i+len("(was "):], ").")
}

// previewDeletes reports what deleteBranches would do, without deleting anything.
func previewDeletes(names []string) []deleteResult {
	results := make([]deleteResult, 0, len(names))
	for _, name := range names {
		sha, err := git("rev-parse", "--short", "refs/heads/"+name)
		var out string
		if err == nil {
			out = fmt.Sprintf("Would delete branch %s (at %s).", name, sha)
		}
		results = append(results, deleteResult{Name: name, Output: out, Err: err})
	}
	return results
}

// deleteBranches force-deletes each branch. Force (-D) is deliberate: -d checks
// against HEAD rather than the base branch, and the UI has already warned
// about unmerged branches on the confirm screen.
func deleteBranches(names []string) []deleteResult {
	results := make([]deleteResult, 0, len(names))
	for _, name := range names {
		out, err := git("branch", "-D", name)
		results = append(results, deleteResult{Name: name, Output: out, Err: err})
	}
	return results
}
