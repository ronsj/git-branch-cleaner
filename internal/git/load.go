package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Positions of the fields in each for-each-ref record.
const (
	fieldName = iota
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
		if branchExists(name) {
			return name
		}
	}
	current, _ := git("branch", "--show-current")
	return current
}

// LoadBranches lists local branches and marks which are merged into the base
// branch: baseOverride if set (--base), otherwise a guess (see baseBranch).
// The UI decides the order.
func LoadBranches(baseOverride string) (base string, branches []Branch, err error) {
	out, err := git("for-each-ref", "--format="+branchFormat, "refs/heads/")
	if err != nil {
		return "", nil, err
	}
	branches = parseBranches(out)

	if baseOverride == "" {
		base = baseBranch()
	} else if !branchExists(baseOverride) {
		return "", nil, fmt.Errorf("base branch %q doesn't exist", baseOverride)
	} else {
		base = baseOverride
	}
	// With no base (a detached HEAD and no main or master), nothing counts as
	// merged, but rebases and bisects still need to be found: a rebase is
	// exactly what detaches HEAD.
	if base != "" {
		isMerged, err := mergedInto(base)
		if err != nil {
			return "", nil, err
		}
		for i := range branches {
			branches[i].Merged = isMerged[branches[i].Name]
		}
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

// branchExists reports whether a local branch exists. Using the full ref name
// keeps a tag with the same name from matching.
func branchExists(name string) bool {
	_, err := git("rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}
