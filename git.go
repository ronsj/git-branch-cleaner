package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Branch is one local git branch plus the metadata shown in the UI.
type Branch struct {
	Name       string
	SHA        string    // full commit the branch points to
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

// deleteResult records the outcome of deleting (or, in a dry run,
// previewing) one branch.
type deleteResult struct {
	Name   string
	SHA    string // commit the branch pointed to; `git branch Name SHA` restores it
	DryRun bool
	Err    error
}

func (r deleteResult) String() string {
	short := r.SHA[:min(len(r.SHA), 7)]
	if r.DryRun {
		return fmt.Sprintf("Would delete branch %s (at %s).", r.Name, short)
	}
	return fmt.Sprintf("Deleted branch %s (was %s).", r.Name, short)
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

// Positions of the fields in each for-each-ref record.
const (
	fieldName = iota
	fieldRelativeDate
	fieldUnixDate
	fieldHead
	fieldUpstreamTrack
	fieldWorktree
	fieldAuthor
	fieldSubject
	fieldSHA
	numFields
)

// branchFields are the for-each-ref fields read for each branch. The keyed
// entries tie each one to its position above, so the list and the parser
// can't get out of step.
//
// The name uses lstrip=2 (drop "refs/heads/") rather than :short, which
// shortens only as far as stays unambiguous: with a tag also named "main",
// :short gives "heads/main".
var branchFields = [numFields]string{
	fieldName:          "%(refname:lstrip=2)",
	fieldRelativeDate:  "%(committerdate:relative)",
	fieldUnixDate:      "%(committerdate:unix)",
	fieldHead:          "%(HEAD)",
	fieldUpstreamTrack: "%(upstream:track)",
	fieldWorktree:      "%(worktreepath)",
	fieldAuthor:        "%(authorname)",
	fieldSubject:       "%(contents:subject)",
	fieldSHA:           "%(objectname)",
}

// branchFormat separates fields with NUL (%00) and ends each record with one,
// just before the newline git adds. NUL is the one byte that can't appear in a
// branch name, commit message, or file path, so no value can break parsing; a
// worktree path, for example, can contain tabs or even newlines.
var branchFormat = strings.Join(branchFields[:], "%00") + "%00"

// parseBranches turns `git for-each-ref --format=branchFormat` output into Branches.
func parseBranches(out string) []Branch {
	// Records are separated by NUL + newline. Remove the last record's
	// terminator once, up front: trimming each record would also eat the
	// separator before an empty last field.
	out = strings.TrimSuffix(strings.TrimSuffix(out, "\n"), "\x00")

	var branches []Branch
	for record := range strings.SplitSeq(out, "\x00\n") {
		fields := strings.Split(record, "\x00")
		if len(fields) != numFields {
			continue
		}
		// A bad timestamp parses as 0, i.e. 1970, so the branch just looks
		// very old; not worth dropping the branch over.
		unix, _ := strconv.ParseInt(fields[fieldUnixDate], 10, 64)
		branches = append(branches, Branch{
			Name:       fields[fieldName],
			SHA:        fields[fieldSHA],
			LastCommit: fields[fieldRelativeDate],
			CommitTime: time.Unix(unix, 0),
			Current:    fields[fieldHead] == "*",
			Gone:       fields[fieldUpstreamTrack] == "[gone]",
			Worktree:   fields[fieldWorktree],
			Author:     fields[fieldAuthor],
			Subject:    fields[fieldSubject],
		})
	}
	return branches
}

// baseBranch guesses the repo's main line: origin's default branch if known,
// then main or master, then whatever is checked out.
func baseBranch() string {
	var candidates []string
	// Full ref names throughout: short names can be ambiguous (see branchFields).
	if ref, err := git("symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil {
		candidates = append(candidates, strings.TrimPrefix(ref, "refs/remotes/origin/"))
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

	isMerged, err := mergedInto(base)
	if err != nil {
		return "", nil, err
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

// mergedInto returns the set of local branches fully merged into base.
func mergedInto(base string) (map[string]bool, error) {
	out, err := git("branch", "--merged", "refs/heads/"+base, "--format=%(refname:lstrip=2)")
	if err != nil {
		return nil, err
	}
	merged := make(map[string]bool)
	for name := range strings.SplitSeq(out, "\n") {
		merged[strings.TrimSpace(name)] = true
	}
	return merged, nil
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

// branchSHAs returns the commit every local branch points to, in one git call.
func branchSHAs() (map[string]string, error) {
	out, err := git("for-each-ref", "--format=%(refname:lstrip=2)%00%(objectname)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	shas := make(map[string]string)
	// Branch names can't contain newlines or NUL, so lines split cleanly.
	for line := range strings.SplitSeq(out, "\n") {
		if name, sha, ok := strings.Cut(line, "\x00"); ok {
			shas[name] = sha
		}
	}
	return shas, nil
}

// previewDeletes reports what deleteBranches would do, without deleting
// anything. It uses the commits recorded when the branches were loaded.
func previewDeletes(branches []Branch) []deleteResult {
	results := make([]deleteResult, 0, len(branches))
	for _, b := range branches {
		results = append(results, deleteResult{Name: b.Name, SHA: b.SHA, DryRun: true})
	}
	return results
}

// deleteBatchSize caps how many names go into one `git branch -D` call,
// keeping the command line well under the OS limit.
const deleteBatchSize = 200

// deleteBranches force-deletes the branches. Force (-D) is deliberate: -d
// checks against HEAD rather than the base branch, and the UI has already
// warned about unmerged branches on the confirm screen.
//
// That warning was based on the branches as loaded, possibly minutes ago, so
// a branch is skipped if it has changed since: if it now points at a
// different commit, or was merged into base and no longer is.
//
// For speed, it reads every branch's commit once, deletes in batches (one git
// process per branch is ~25x slower), then reads again to see what's gone.
// Commits come from git directly rather than from its "Deleted branch x (was
// abc1234)." message, which is translated in some languages.
func deleteBranches(branches []Branch, base string) []deleteResult {
	before, err := branchSHAs()
	if err != nil {
		return failAll(branches, err)
	}
	var merged map[string]bool
	if base != "" {
		if merged, err = mergedInto(base); err != nil {
			return failAll(branches, err)
		}
	}

	skipped := make(map[string]error)
	var names []string
	for _, b := range branches {
		sha, exists := before[b.Name]
		switch {
		case !exists:
			skipped[b.Name] = fmt.Errorf("branch %s no longer exists", b.Name)
		case sha != b.SHA:
			skipped[b.Name] = fmt.Errorf("skipped %s: it changed since you selected it", b.Name)
		case b.Merged && merged != nil && !merged[b.Name]:
			skipped[b.Name] = fmt.Errorf("skipped %s: it's no longer merged into %s", b.Name, base)
		default:
			names = append(names, b.Name)
		}
	}
	for batch := range slices.Chunk(names, deleteBatchSize) {
		// Errors are ignored here: git deletes what it can, and the check
		// below works out which branches are gone. "--" ends the options, so
		// a name like "-r" isn't read as a flag.
		git(append([]string{"branch", "-D", "--"}, batch...)...)
	}
	// If this read fails, every attempted branch counts as deleted (after is
	// nil): keeping its SHA keeps its restore command, and running that for a
	// branch that survived is harmless (git says it already exists).
	after, _ := branchSHAs()

	results := make([]deleteResult, 0, len(branches))
	for _, b := range branches {
		if err, ok := skipped[b.Name]; ok {
			results = append(results, deleteResult{Name: b.Name, Err: err})
			continue
		}
		sha := before[b.Name]
		var err error
		if _, remains := after[b.Name]; remains {
			// Git refused (in a worktree, say). Try once more on its own to
			// get git's reason for this branch.
			_, err = git("branch", "-D", "--", b.Name)
		}
		results = append(results, deleteResult{Name: b.Name, SHA: sha, Err: err})
	}
	return results
}

func failAll(branches []Branch, err error) []deleteResult {
	results := make([]deleteResult, 0, len(branches))
	for _, b := range branches {
		results = append(results, deleteResult{Name: b.Name, Err: err})
	}
	return results
}
