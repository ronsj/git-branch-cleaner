package git

import (
	"errors"
	"fmt"
	"slices"
)

// DeleteResult records the outcome of deleting (or, in a dry run,
// previewing) one branch.
type DeleteResult struct {
	Name   string
	SHA    string // commit the branch pointed to; `git branch Name SHA` restores it
	DryRun bool
	Err    error
}

// String describes the outcome in a sentence, for the UI and the summary
// printed on exit.
func (r DeleteResult) String() string {
	short := r.SHA[:min(len(r.SHA), 7)]
	switch {
	case r.Err != nil && r.DryRun:
		return fmt.Sprintf("Wouldn't delete %s: %v", r.Name, r.Err)
	case r.Err != nil:
		return fmt.Sprintf("Didn't delete %s: %v", r.Name, r.Err)
	case r.DryRun:
		return fmt.Sprintf("Would delete branch %s (at %s).", r.Name, short)
	default:
		return fmt.Sprintf("Deleted branch %s (was %s).", r.Name, short)
	}
}

// PreviewDeletes reports what DeleteBranches would do, without deleting
// anything: it makes the same checks, so it skips the same branches.
func PreviewDeletes(branches []Branch, base string) []DeleteResult {
	ready, skipped, err := recheck(branches, base)
	if err != nil {
		return failAll(branches, err)
	}
	results := make([]DeleteResult, 0, len(branches))
	for _, b := range branches {
		if err, ok := skipped[b.Name]; ok {
			results = append(results, DeleteResult{Name: b.Name, DryRun: true, Err: err})
			continue
		}
		results = append(results, DeleteResult{Name: b.Name, SHA: ready[b.Name], DryRun: true})
	}
	return results
}

// recheck compares the selected branches, as they were loaded (possibly
// minutes ago), with how they are now. It returns the commit of each branch
// that's still safe to delete, and the reason each other one isn't: it no
// longer exists, it points at a different commit, it was merged into base
// and no longer is, or it has become protected (checked out, or in use by a
// rebase or bisect).
func recheck(branches []Branch, base string) (ready map[string]string, skipped map[string]error, err error) {
	_, current, err := LoadBranches(base)
	if err != nil {
		return nil, nil, err
	}
	now := make(map[string]Branch, len(current))
	for _, b := range current {
		now[b.Name] = b
	}

	ready = make(map[string]string)
	skipped = make(map[string]error)
	for _, b := range branches {
		c, exists := now[b.Name]
		switch {
		case !exists:
			skipped[b.Name] = errors.New("it no longer exists")
		case c.SHA != b.SHA:
			skipped[b.Name] = errors.New("it changed since you selected it")
		case b.Merged && !c.Merged:
			skipped[b.Name] = fmt.Errorf("it's no longer merged into %s", base)
		case c.Current:
			skipped[b.Name] = errors.New("it's checked out")
		case c.InOtherWorktree():
			skipped[b.Name] = errors.New("it's checked out in another worktree")
		case c.InProgress != "":
			skipped[b.Name] = fmt.Errorf("it's in use (%s)", c.InProgress)
		default:
			ready[b.Name] = c.SHA
		}
	}
	return ready, skipped, nil
}

// deleteBatchSize caps how many names go into one `git branch -D` call,
// keeping the command line well under the OS limit.
const deleteBatchSize = 200

// DeleteBranches force-deletes the branches. Force (-D) is deliberate: -d
// checks against HEAD rather than the base branch, and the UI has already
// warned about unmerged branches on the confirm screen.
//
// That warning was based on the branches as loaded, possibly minutes ago, so
// recheck skips any branch that has changed since.
//
// For speed, it deletes in batches (one git process per branch is ~25x
// slower), then reads the branches again to see what's gone. Commits come
// from git directly rather than from its "Deleted branch x (was abc1234)."
// message, which is translated in some languages.
func DeleteBranches(branches []Branch, base string) []DeleteResult {
	ready, skipped, err := recheck(branches, base)
	if err != nil {
		return failAll(branches, err)
	}
	var names []string
	for _, b := range branches {
		if _, ok := ready[b.Name]; ok {
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

	results := make([]DeleteResult, 0, len(branches))
	for _, b := range branches {
		if err, ok := skipped[b.Name]; ok {
			results = append(results, DeleteResult{Name: b.Name, Err: err})
			continue
		}
		var err error
		if _, remains := after[b.Name]; remains {
			// Git refused anyway. Try once more on its own to get git's
			// reason for this branch.
			_, err = git("branch", "-D", "--", b.Name)
		}
		results = append(results, DeleteResult{Name: b.Name, SHA: ready[b.Name], Err: err})
	}
	return results
}

func failAll(branches []Branch, err error) []DeleteResult {
	results := make([]DeleteResult, 0, len(branches))
	for _, b := range branches {
		results = append(results, DeleteResult{Name: b.Name, Err: err})
	}
	return results
}
