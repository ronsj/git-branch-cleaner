package git

import (
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

func TestDeleteResultString(t *testing.T) {
	sha := "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"
	tests := []struct {
		result DeleteResult
		want   string
	}{
		{DeleteResult{Name: "feature/x", SHA: sha}, "Deleted branch feature/x (was 1a2b3c4)."},
		{DeleteResult{Name: "feature/x", SHA: sha, DryRun: true}, "Would delete branch feature/x (at 1a2b3c4)."},
		{DeleteResult{Name: "feature/x", Err: errors.New("it changed since you selected it")}, "Didn't delete feature/x: it changed since you selected it"},
		{DeleteResult{Name: "feature/x", DryRun: true, Err: errors.New("it's checked out")}, "Wouldn't delete feature/x: it's checked out"},
	}
	for _, tt := range tests {
		if got := tt.result.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestPreviewDeletesKeepsBranches(t *testing.T) {
	testrepo.New(t, "old")

	old := loadBranch(t, "old")

	results := PreviewDeletes([]Branch{old}, "main")

	if results[0].Err != nil || results[0].SHA != testrepo.Git(t, "rev-parse", "refs/heads/old") {
		t.Errorf("preview of old = %+v, want its current commit", results[0])
	}
	if !strings.HasPrefix(results[0].String(), "Would delete branch old (at ") {
		t.Errorf("preview message = %q", results[0].String())
	}
	if !testrepo.BranchExists("old") {
		t.Fatal("a dry run must not delete the branch")
	}
}

func TestDeleteBranchesCanBeRestored(t *testing.T) {
	testrepo.New(t, "old")
	tip := testrepo.Git(t, "rev-parse", "refs/heads/old")

	results := DeleteBranches(branchesNamed(t, "old"), "main")
	if results[0].Err != nil {
		t.Fatal(results[0].Err)
	}
	if results[0].SHA != tip {
		t.Errorf("recorded SHA = %q, want the branch's full tip %q", results[0].SHA, tip)
	}
	if testrepo.BranchExists("old") {
		t.Fatal("branch should be deleted")
	}

	// The restore command printed on exit should bring it back.
	testrepo.Git(t, "branch", "old", results[0].SHA)
	if !testrepo.BranchExists("old") {
		t.Fatal("branch should be restored from the recorded SHA")
	}
}

func TestDeleteBranchNamedLikeAnOption(t *testing.T) {
	testrepo.New(t)
	// git branch refuses names starting with "-", but plumbing can create them.
	for _, name := range []string{"-r", "--all"} {
		testrepo.Git(t, "update-ref", "refs/heads/"+name, "HEAD")
	}

	for _, r := range PreviewDeletes(branchesNamed(t, "-r", "--all"), "main") {
		if r.Err != nil {
			t.Errorf("preview of %q: %v", r.Name, r.Err)
		}
	}
	for _, r := range DeleteBranches(branchesNamed(t, "-r", "--all"), "main") {
		if r.Err != nil {
			t.Errorf("delete of %q: %v", r.Name, r.Err)
		}
		if testrepo.BranchExists(r.Name) {
			t.Errorf("%q should be deleted", r.Name)
		}
	}
}

func TestDeleteBranchesInSeveralBatches(t *testing.T) {
	testrepo.New(t)
	var names []string
	var creates strings.Builder
	for i := range deleteBatchSize + 50 {
		name := fmt.Sprintf("old-%03d", i)
		names = append(names, name)
		fmt.Fprintf(&creates, "create refs/heads/%s HEAD\n", name)
	}
	cmd := exec.Command("git", "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(creates.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("creating branches: %v %s", err, out)
	}
	tip := testrepo.Git(t, "rev-parse", "HEAD")

	results := DeleteBranches(branchesNamed(t, names...), "main")
	remaining, err := branchSHAs()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if _, still := remaining[r.Name]; r.Err != nil || r.SHA != tip || still {
			t.Fatalf("result %+v: want deleted, with SHA %s", r, tip)
		}
	}
}

func TestDeleteBranchesReportsEachFailure(t *testing.T) {
	testrepo.New(t, "a", "busy", "b")
	testrepo.Git(t, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "busy")
	branches := branchesNamed(t, "a", "busy", "b")
	testrepo.Git(t, "branch", "-D", "b") // deleted by someone else after loading

	results := DeleteBranches(branches, "main")
	if results[0].Err != nil || testrepo.BranchExists("a") {
		t.Errorf("a: %+v, want deleted", results[0])
	}
	if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "worktree") {
		t.Errorf("busy: err = %v, want git's reason (checked out in a worktree)", results[1].Err)
	}
	if results[2].Err == nil || !strings.Contains(results[2].Err.Error(), "no longer exists") {
		t.Errorf("b: err = %v, want 'no longer exists'", results[2].Err)
	}
}

func TestDeleteSkipsBranchThatChangedSinceLoading(t *testing.T) {
	testrepo.New(t, "feature")
	loaded := branchesNamed(t, "feature")
	// Someone commits to the branch after the list was loaded.
	testrepo.Git(t, "switch", "-q", "feature")
	testrepo.CommitFile(t, ".", "new work")
	testrepo.Git(t, "switch", "-q", "main")

	results := DeleteBranches(loaded, "main")
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "changed since") {
		t.Errorf("err = %v, want a 'changed since' refusal", results[0].Err)
	}
	if !testrepo.BranchExists("feature") {
		t.Fatal("a branch with commits the user hasn't seen must not be deleted")
	}
}

func TestDeleteSkipsBranchNoLongerMerged(t *testing.T) {
	testrepo.New(t)
	testrepo.CommitFile(t, ".", "second")
	testrepo.Git(t, "branch", "done") // merged: points at main's tip
	loaded := branchesNamed(t, "done")
	if !loaded[0].Merged {
		t.Fatal("test setup: done should start out merged")
	}
	testrepo.Git(t, "reset", "-q", "--hard", "HEAD~1") // main drops the commit

	results := DeleteBranches(loaded, "main")
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "no longer merged") {
		t.Errorf("err = %v, want a 'no longer merged' refusal", results[0].Err)
	}
	if !testrepo.BranchExists("done") {
		t.Fatal("a branch that stopped being merged must not be deleted without a warning")
	}
}

// A dry run must predict the real run: after the same changes since loading,
// both should act on, and skip, the same branches.
func TestPreviewMatchesDelete(t *testing.T) {
	testrepo.New(t)
	testrepo.CommitFile(t, ".", "second")
	testrepo.Git(t, "branch", "ok", "HEAD~1")
	testrepo.Git(t, "branch", "moved")
	testrepo.Git(t, "branch", "lost-merge")
	testrepo.Git(t, "branch", "busy", "HEAD~1")
	testrepo.Git(t, "branch", "gone", "HEAD~1")
	names := []string{"ok", "moved", "lost-merge", "busy", "gone"}
	loaded := branchesNamed(t, names...)

	// What can happen between loading the list and confirming:
	testrepo.Git(t, "switch", "-q", "moved")
	testrepo.CommitFile(t, ".", "new work on moved") // new commit
	testrepo.Git(t, "switch", "-q", "main")
	testrepo.Git(t, "reset", "-q", "--hard", "HEAD~1") // lost-merge is no longer merged
	testrepo.Git(t, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "busy")
	testrepo.Git(t, "branch", "-D", "gone")

	preview := PreviewDeletes(loaded, "main")
	for _, name := range []string{"ok", "moved", "lost-merge", "busy"} {
		if !testrepo.BranchExists(name) {
			t.Fatalf("the dry run deleted %s", name)
		}
	}
	deleted := DeleteBranches(loaded, "main")

	for i, name := range names {
		wouldDelete, didDelete := preview[i].Err == nil, deleted[i].Err == nil
		if wouldDelete != didDelete {
			t.Errorf("%s: dry run would delete = %v (%v), real run deleted = %v (%v)",
				name, wouldDelete, preview[i].Err, didDelete, deleted[i].Err)
		}
		if want := name == "ok"; didDelete != want {
			t.Errorf("%s: deleted = %v, want %v (%v)", name, didDelete, want, deleted[i].Err)
		}
	}
}

// forceDelete reads the commit from git's own message: it's the one that was
// actually deleted, even if the branch moved after recheck looked at it.
func TestForceDeleteRecordsDeletedCommits(t *testing.T) {
	testrepo.New(t, "plain", "we(ird")
	tip := testrepo.Git(t, "rev-parse", "HEAD")
	testrepo.Git(t, "update-ref", "refs/heads/-r", tip) // git branch -r would list remotes
	t.Setenv("LANG", "de_DE.UTF-8")                     // LC_ALL=C must win over the user's locale
	t.Setenv("LC_ALL", "de_DE.UTF-8")

	deleted := make(map[string]string)
	err := forceDelete([]string{"plain", "-r", "we(ird", "missing"}, deleted)
	if err == nil {
		t.Error("expected an error for the missing branch")
	}
	want := map[string]string{"plain": tip, "-r": tip, "we(ird": tip}
	if !maps.Equal(deleted, want) {
		t.Errorf("deleted = %v, want %v", deleted, want)
	}
}

func TestFullSHA(t *testing.T) {
	testrepo.New(t)
	tip := testrepo.Git(t, "rev-parse", "HEAD")
	testrepo.CommitFile(t, ".", "moved on")
	moved := testrepo.Git(t, "rev-parse", "HEAD")

	if got := fullSHA(tip[:12], tip); got != tip {
		t.Errorf("fullSHA of the expected commit = %q, want %q", got, tip)
	}
	// The branch moved between recheck and the delete: record where it was.
	if got := fullSHA(moved[:12], tip); got != moved {
		t.Errorf("fullSHA of a different commit = %q, want %q", got, moved)
	}
}
