package git

import (
	"fmt"
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

	results := PreviewDeletes([]Branch{old})

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

	for _, r := range PreviewDeletes(branchesNamed(t, "-r", "--all")) {
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
