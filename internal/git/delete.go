package git

import (
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

func (r DeleteResult) String() string {
	short := r.SHA[:min(len(r.SHA), 7)]
	if r.DryRun {
		return fmt.Sprintf("Would delete branch %s (at %s).", r.Name, short)
	}
	return fmt.Sprintf("Deleted branch %s (was %s).", r.Name, short)
}

// PreviewDeletes reports what DeleteBranches would do, without deleting
// anything. It uses the commits recorded when the branches were loaded.
func PreviewDeletes(branches []Branch) []DeleteResult {
	results := make([]DeleteResult, 0, len(branches))
	for _, b := range branches {
		results = append(results, DeleteResult{Name: b.Name, SHA: b.SHA, DryRun: true})
	}
	return results
}

// deleteBatchSize caps how many names go into one `git branch -D` call,
// keeping the command line well under the OS limit.
const deleteBatchSize = 200

// DeleteBranches force-deletes the branches. Force (-D) is deliberate: -d
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
func DeleteBranches(branches []Branch, base string) []DeleteResult {
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

	results := make([]DeleteResult, 0, len(branches))
	for _, b := range branches {
		if err, ok := skipped[b.Name]; ok {
			results = append(results, DeleteResult{Name: b.Name, Err: err})
			continue
		}
		sha := before[b.Name]
		var err error
		if _, remains := after[b.Name]; remains {
			// Git refused (in a worktree, say). Try once more on its own to
			// get git's reason for this branch.
			_, err = git("branch", "-D", "--", b.Name)
		}
		results = append(results, DeleteResult{Name: b.Name, SHA: sha, Err: err})
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
