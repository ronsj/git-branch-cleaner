package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Branch is one local git branch plus the metadata shown in the UI.
type Branch struct {
	Name       string
	LastCommit string // relative date, e.g. "3 weeks ago"
	Current    bool   // checked out right now
	Gone       bool   // upstream was deleted on the remote (often a squash-merged PR)
	Merged     bool   // fully merged into the base branch
}

// Protected reports whether the UI should refuse to delete this branch.
func (b Branch) Protected(base string) bool {
	return b.Current || b.Name == base
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

// The fields are tab-separated (%09) so branch names never collide with the separator.
const branchFormat = "%(refname:short)%09%(committerdate:relative)%09%(HEAD)%09%(upstream:track)"

// parseBranches turns `git for-each-ref --format=branchFormat` output into Branches.
func parseBranches(out string) []Branch {
	var branches []Branch
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		branches = append(branches, Branch{
			Name:       fields[0],
			LastCommit: fields[1],
			Current:    fields[2] == "*",
			Gone:       fields[3] == "[gone]",
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

// loadBranches lists local branches, oldest first, and marks which are merged into base.
func loadBranches() (base string, branches []Branch, err error) {
	out, err := git("for-each-ref", "--sort=committerdate", "--format="+branchFormat, "refs/heads/")
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
	return base, branches, nil
}

// restoreSHA extracts the commit from git's "Deleted branch foo (was abc1234)." message.
func restoreSHA(output string) string {
	i := strings.LastIndex(output, "(was ")
	if i < 0 {
		return ""
	}
	return strings.TrimSuffix(output[i+len("(was "):], ").")
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
