package git

import (
	"time"
)

// Branch is one local git branch plus the metadata shown in the UI.
type Branch struct {
	Name       string
	SHA        string    // full commit the branch points to
	CommitTime time.Time // exact date, for sorting and age checks
	Current    bool      // checked out right now
	Worktree   string    // path of the worktree that has it checked out, if any
	InProgress string    // "rebasing" or "bisecting" if a worktree is doing that to it
	Upstream   string    // full ref of the branch it tracks, if any
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

// IsBase reports whether this is the base branch or, when the base is a
// remote-tracking branch like origin/main, the local branch that tracks it.
func (b Branch) IsBase(base string) bool {
	return base != "" && (b.Name == base || b.Upstream == "refs/remotes/"+base)
}

// Protected reports whether the UI should refuse to delete this branch.
func (b Branch) Protected(base string) bool {
	return b.Current || b.IsBase(base) || b.InOtherWorktree() || b.InProgress != ""
}
