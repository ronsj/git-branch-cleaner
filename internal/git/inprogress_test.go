package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

func TestLoadBranchesDetectsRebaseInProgress(t *testing.T) {
	testrepo.New(t)
	testrepo.Git(t, "switch", "-q", "-c", "feature")
	testrepo.CommitFile(t, ".", "one")
	// Stop the rebase at its first commit, like pausing to fix a conflict.
	t.Setenv("GIT_SEQUENCE_EDITOR", "sed -i.bak s/^pick/edit/")
	testrepo.Git(t, "rebase", "-q", "-i", "HEAD~1")

	// From a subdirectory, git reports the git directory as a relative path.
	sub := filepath.Join("src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)

	feature := loadBranch(t, "feature")
	if feature.InProgress != "rebasing" || !feature.Protected("main") {
		t.Errorf("feature = %+v, want protected and marked rebasing", feature)
	}
	// Ask git directly: the app skips protected branches before git sees them.
	if _, err := git("branch", "-D", "--", "feature"); err == nil {
		t.Error("expected git to refuse deleting a branch that's being rebased")
	}
}

func TestLoadBranchesDetectsBisectInOtherWorktree(t *testing.T) {
	testrepo.New(t, "feature")
	wt := filepath.Join(t.TempDir(), "bisect-wt")
	testrepo.Git(t, "worktree", "add", "-q", wt, "feature")
	for _, message := range []string{"one", "two", "three", "four"} {
		testrepo.CommitFile(t, wt, message)
	}
	testrepo.Git(t, "-C", wt, "bisect", "start")
	testrepo.Git(t, "-C", wt, "bisect", "bad")
	testrepo.Git(t, "-C", wt, "bisect", "good", "HEAD~4")

	feature := loadBranch(t, "feature")
	// This is why the state files are needed: bisect detached the worktree's
	// HEAD, so git no longer reports the branch as checked out there.
	if feature.Worktree != "" {
		t.Fatalf("test setup: expected bisect to detach HEAD, but Worktree = %q", feature.Worktree)
	}
	if feature.InProgress != "bisecting" || !feature.Protected("main") {
		t.Errorf("feature = %+v, want protected and marked bisecting", feature)
	}
	// Ask git directly: the app skips protected branches before git sees them.
	if _, err := git("branch", "-D", "--", "feature"); err == nil {
		t.Error("expected git to refuse deleting a branch that's being bisected")
	}
}
