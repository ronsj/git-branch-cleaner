package git

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Positions of the fields in each for-each-ref record.
const (
	fieldName = iota
	fieldUnixDate
	fieldHead
	fieldUpstream
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
	fieldUpstream:      "%(upstream)",
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
			Upstream:   fields[fieldUpstream],
			Gone:       fields[fieldUpstreamTrack] == "[gone]",
			Worktree:   fields[fieldWorktree],
			Author:     printable(fields[fieldAuthor]),
			Subject:    printable(fields[fieldSubject]),
		})
	}
	return branches
}

// printable drops control characters from s, since a terminal acts on them
// instead of showing them: a commit's author or subject could otherwise move
// the cursor, restyle the screen, or add a link. Tabs become spaces. (Git
// already does this to its own error messages.)
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
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
// branch, directly or by a rebase or squash merge (see rebaseMergedInto and
// squashMergedInto): baseOverride if set (--base), otherwise a guess (see
// baseBranch).
// baseOverride can name a local branch or a remote-tracking one, such as
// origin/main, which is often ahead of the local main.
// The UI decides the order.
func LoadBranches(baseOverride string) (base string, branches []Branch, err error) {
	out, err := git("for-each-ref", "--format="+branchFormat, "refs/heads/")
	if err != nil {
		return "", nil, err
	}
	branches = parseBranches(out)

	var baseRef string
	if baseOverride == "" {
		base = baseBranch()
		baseRef = "refs/heads/" + base
	} else if baseRef = resolveBase(baseOverride); baseRef == "" {
		return "", nil, fmt.Errorf("base branch %q doesn't exist, locally or as a remote-tracking branch", baseOverride)
	} else {
		base = baseOverride
	}
	// With no base (a detached HEAD and no main or master), nothing counts as
	// merged, but rebases and bisects still need to be found: a rebase is
	// exactly what detaches HEAD.
	if base != "" {
		isMerged, err := mergedInto(baseRef)
		if err != nil {
			return "", nil, err
		}
		for i := range branches {
			b := &branches[i]
			b.Merged = isMerged[b.Name]
			if b.Merged || b.IsBase(base) {
				continue
			}
			if b.Merged, err = rebaseMergedInto(baseRef, b.Name); err != nil {
				return "", nil, err
			}
			if b.Merged {
				continue
			}
			if b.Merged, err = squashMergedInto(baseRef, b.Name); err != nil {
				return "", nil, err
			}
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

// resolveBase returns the full ref for a --base name: the local branch if
// there is one, otherwise the remote-tracking branch, otherwise "".
func resolveBase(name string) string {
	for _, ref := range []string{"refs/heads/" + name, "refs/remotes/" + name} {
		if _, err := git("rev-parse", "--verify", "--quiet", ref); err == nil {
			return ref
		}
	}
	return ""
}

// mergedInto returns the set of local branches fully merged into baseRef.
func mergedInto(baseRef string) (map[string]bool, error) {
	out, err := git("branch", "--merged", baseRef, "--format=%(refname:lstrip=2)")
	if err != nil {
		return nil, err
	}
	merged := make(map[string]bool)
	for name := range strings.SplitSeq(out, "\n") {
		merged[strings.TrimSpace(name)] = true
	}
	return merged, nil
}

// rebaseMergedInto reports whether every commit on branch has an identical
// copy (same patch-id) in baseRef, as after a pull request is rebase-merged:
// the copies get new SHAs, so git branch --merged misses them. A squash merge
// combines the commits into one, so it still isn't detected.
//
// rev-list marks each commit on the branch side "=" if the base has an
// equivalent and "+" if not. Merge commits are always "+", so a branch with
// merges of its own never counts: a merge can carry changes no patch-id
// covers.
func rebaseMergedInto(baseRef, branch string) (bool, error) {
	out, err := git("rev-list", "--cherry-mark", "--right-only", baseRef+"...refs/heads/"+branch, "--")
	if err != nil {
		return false, err
	}
	if out == "" {
		return false, nil
	}
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "=") {
			return false, nil
		}
	}
	return true, nil
}

// squashMergedInto reports whether one commit in baseRef makes the same change
// as the whole branch, as after a pull request is squash-merged. It compares
// patch-ids: the branch's diff since it left the base against each commit the
// base has gained since then, limited to the files the branch changed. Diffs
// of the same change match even if the base moved on elsewhere in the
// meantime, but not if it changed lines next to the branch's.
//
// A match means the base has every change the branch made, even if the
// matching commit also changed other files. Unlike rebaseMergedInto, this
// looks at the branch's end result, so a branch with merge commits can still
// match: anything a merge added is in the diff.
func squashMergedInto(baseRef, branch string) (bool, error) {
	branchRef := "refs/heads/" + branch
	// The same options for every diff, so the same change always looks the
	// same. Without renames, a moved file is listed under both its names.
	// --full-index gives binary files whole blob ids for patch-id to tell
	// changes apart by.
	diffOptions := []string{"--no-ext-diff", "--no-color", "--no-renames", "--full-index"}

	// Three dots: the diff from the merge base, i.e. what the branch changed.
	changes := baseRef + "..." + branchRef
	paths, err := git(slices.Concat([]string{"diff", "--name-only", "-z"}, diffOptions, []string{changes, "--"})...)
	if err != nil || paths == "" {
		return false, err
	}
	diff, err := git(slices.Concat([]string{"diff"}, diffOptions, []string{changes, "--"})...)
	if err != nil {
		return false, err
	}
	// One diff, so one patch-id.
	want, err := patchIDs(diff)
	if err != nil || len(want) != 1 {
		return false, err
	}

	// --stdin takes the paths after a "--" line, one per line, so there's no
	// limit on how many and no quoting: GIT_LITERAL_PATHSPECS stops a name
	// like "*.go" from matching as a pattern. (A path with a newline in it
	// gets split and just finds nothing.)
	stdin := "--\n" + strings.ReplaceAll(strings.TrimSuffix(paths, "\x00"), "\x00", "\n") + "\n"
	log, err := run(stdin, []string{"GIT_LITERAL_PATHSPECS=1"}, nil,
		slices.Concat([]string{"log", "-p", "--no-merges"}, diffOptions, []string{"--stdin", branchRef + ".." + baseRef})...)
	if err != nil || log == "" {
		return false, err
	}
	have, err := patchIDs(log)
	return slices.Contains(have, want[0]), err
}

// patchIDs returns the stable patch-id of each patch in diffs: the output of
// git diff or git log -p.
func patchIDs(diffs string) ([]string, error) {
	out, err := run(diffs+"\n", nil, nil, "patch-id", "--stable")
	if err != nil {
		return nil, err
	}
	var ids []string
	for line := range strings.SplitSeq(out, "\n") {
		if id, _, ok := strings.Cut(line, " "); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
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
