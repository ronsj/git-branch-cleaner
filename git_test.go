package main

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// row places one branch's field values at their positions in a record.
func row(name, relativeDate, unixDate, head, upstreamTrack, worktree, author, subject, sha string) []string {
	var f [numFields]string
	f[fieldName], f[fieldRelativeDate], f[fieldUnixDate] = name, relativeDate, unixDate
	f[fieldHead], f[fieldUpstreamTrack], f[fieldWorktree] = head, upstreamTrack, worktree
	f[fieldAuthor], f[fieldSubject], f[fieldSHA] = author, subject, sha
	return f[:]
}

// forEachRefOutput builds for-each-ref output in branchFormat from rows of
// field values, trimmed of its final newline the way git() returns it.
func forEachRefOutput(rows ...[]string) string {
	var out strings.Builder
	for _, fields := range rows {
		out.WriteString(strings.Join(fields, "\x00") + "\x00\n")
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func TestParseBranches(t *testing.T) {
	out := forEachRefOutput(
		row("old-feature", "3 months ago", "1750000000", " ", "[gone]", "/work/review", "Alex Kim", "Add login form", "aaaa111"),
		row("main", "2 days ago", "1757000000", "*", "", "/work/app", "Sam Lee", "Merge feature/login", "bbbb222"),
		row("wip", "5 minutes ago", "1757100000", " ", "[ahead 2]", "/work/odd\tpath\nwith newline", "Sam Lee", "WIP: tabs\tin subject", "cccc333"),
		row("empty-subject", "1 year, 2 months ago", "1720000000", " ", "", "", "Alex Kim", "", "dddd444"),
		row("bad-time", "1 day ago", "not-a-number", " ", "", "", "Alex Kim", "", "eeee555"),
	)

	got := parseBranches(out)
	want := []Branch{
		{Name: "old-feature", LastCommit: "3 months ago", CommitTime: time.Unix(1750000000, 0), Gone: true, Worktree: "/work/review", Author: "Alex Kim", Subject: "Add login form", SHA: "aaaa111"},
		{Name: "main", LastCommit: "2 days ago", CommitTime: time.Unix(1757000000, 0), Current: true, Worktree: "/work/app", Author: "Sam Lee", Subject: "Merge feature/login", SHA: "bbbb222"},
		{Name: "wip", LastCommit: "5 minutes ago", CommitTime: time.Unix(1757100000, 0), Worktree: "/work/odd\tpath\nwith newline", Author: "Sam Lee", Subject: "WIP: tabs\tin subject", SHA: "cccc333"},
		{Name: "empty-subject", LastCommit: "1 year, 2 months ago", CommitTime: time.Unix(1720000000, 0), Author: "Alex Kim", SHA: "dddd444"},
		{Name: "bad-time", LastCommit: "1 day ago", CommitTime: time.Unix(0, 0), Author: "Alex Kim", SHA: "eeee555"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranches:\n got  %+v\n want %+v", got, want)
	}
}

func TestEveryBranchFieldHasAFormat(t *testing.T) {
	// A keyed array leaves any position without an entry empty.
	for i, f := range branchFields {
		if f == "" {
			t.Errorf("branchFields[%d] has no format", i)
		}
	}
}

func TestParseBranchesWithTrailingNewline(t *testing.T) {
	out := forEachRefOutput(row("a", "now", "1", " ", "", "", "Sam", "", "ffff666")) + "\n"
	if got := parseBranches(out); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("parseBranches = %+v, want the one branch", got)
	}
}

func TestParseBranchesEmpty(t *testing.T) {
	if got := parseBranches(""); len(got) != 0 {
		t.Errorf("expected no branches, got %+v", got)
	}
}

func TestDeleteResultString(t *testing.T) {
	sha := "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"
	tests := []struct {
		result deleteResult
		want   string
	}{
		{deleteResult{Name: "feature/x", SHA: sha}, "Deleted branch feature/x (was 1a2b3c4)."},
		{deleteResult{Name: "feature/x", SHA: sha, DryRun: true}, "Would delete branch feature/x (at 1a2b3c4)."},
	}
	for _, tt := range tests {
		if got := tt.result.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

// newTestRepo creates a git repo with one commit on main plus the given
// branches, and makes it the working directory for the rest of the test.
// The user's own git config is ignored so tests behave the same everywhere.
func newTestRepo(t *testing.T, branches ...string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@example.com")
	}
	t.Chdir(t.TempDir())

	mustGit(t, "init", "-q", "-b", "main")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "initial")
	for _, b := range branches {
		mustGit(t, "branch", b)
	}
}

func mustGit(t *testing.T, args ...string) string {
	t.Helper()
	out, err := git(args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func branchExists(name string) bool {
	_, err := git("rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

func TestLoadBranches(t *testing.T) {
	newTestRepo(t, "done")
	mustGit(t, "switch", "-q", "-c", "wip")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "Work in progress")

	base, branches, err := loadBranches()
	if err != nil {
		t.Fatal(err)
	}
	if base != "main" {
		t.Errorf("base = %q, want main", base)
	}

	byName := make(map[string]Branch)
	for _, b := range branches {
		byName[b.Name] = b
	}
	if !byName["done"].Merged {
		t.Error("done has no new commits, so it should be merged into main")
	}
	if wip := byName["wip"]; wip.Merged || !wip.Current || wip.Subject != "Work in progress" {
		t.Errorf("wip = %+v, want unmerged, current, with its commit subject", wip)
	}
	if age := time.Since(byName["wip"].CommitTime); age < 0 || age > time.Minute {
		t.Errorf("wip was committed just now, but CommitTime is %v ago", age)
	}
}

func TestPreviewDeletesKeepsBranches(t *testing.T) {
	newTestRepo(t, "old")

	old := loadBranch(t, "old")

	results := previewDeletes([]Branch{old})

	if results[0].Err != nil || results[0].SHA != mustGit(t, "rev-parse", "refs/heads/old") {
		t.Errorf("preview of old = %+v, want its current commit", results[0])
	}
	if !strings.HasPrefix(results[0].String(), "Would delete branch old (at ") {
		t.Errorf("preview message = %q", results[0].String())
	}
	if !branchExists("old") {
		t.Fatal("a dry run must not delete the branch")
	}
}

func TestDeleteBranchesCanBeRestored(t *testing.T) {
	newTestRepo(t, "old")
	tip := mustGit(t, "rev-parse", "refs/heads/old")

	results := deleteBranches(branchesNamed(t, "old"), "main")
	if results[0].Err != nil {
		t.Fatal(results[0].Err)
	}
	if results[0].SHA != tip {
		t.Errorf("recorded SHA = %q, want the branch's full tip %q", results[0].SHA, tip)
	}
	if branchExists("old") {
		t.Fatal("branch should be deleted")
	}

	// The restore command printed on exit should bring it back.
	mustGit(t, "branch", "old", results[0].SHA)
	if !branchExists("old") {
		t.Fatal("branch should be restored from the recorded SHA")
	}
}

func TestProtected(t *testing.T) {
	tests := []struct {
		name   string
		branch Branch
		want   bool
	}{
		{"plain branch", Branch{Name: "feature"}, false},
		{"base branch", Branch{Name: "main"}, true},
		{"current branch", Branch{Name: "wip", Current: true, Worktree: "/work/app"}, true},
		{"in another worktree", Branch{Name: "review", Worktree: "/work/review"}, true},
		{"mid-rebase", Branch{Name: "feature", InProgress: "rebasing"}, true},
		{"mid-bisect", Branch{Name: "feature", InProgress: "bisecting"}, true},
	}
	for _, tt := range tests {
		if got := tt.branch.Protected("main"); got != tt.want {
			t.Errorf("%s: Protected = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoadBranchesDetectsOtherWorktrees(t *testing.T) {
	newTestRepo(t, "review")
	wt := filepath.Join(t.TempDir(), "review-wt")
	mustGit(t, "worktree", "add", "-q", wt, "review")

	_, branches, err := loadBranches()
	if err != nil {
		t.Fatal(err)
	}
	var review Branch
	for _, b := range branches {
		if b.Name == "review" {
			review = b
		}
	}

	// Git reports the real path; on macOS the temp dir is behind a symlink.
	wantPath, _ := filepath.EvalSymlinks(wt)
	if review.Worktree != wantPath {
		t.Errorf("Worktree = %q, want %q", review.Worktree, wantPath)
	}
	if !review.Protected("main") {
		t.Error("a branch checked out in another worktree should be protected")
	}

	// The protection matches git's own rule: it refuses this delete.
	if results := deleteBranches(branchesNamed(t, "review"), "main"); results[0].Err == nil {
		t.Error("expected git to refuse deleting a branch checked out in a worktree")
	}
}

// commitFile changes a file in dir and commits it, giving rebase and bisect
// real commits to work through.
func commitFile(t *testing.T, dir, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, "-C", dir, "add", "file.txt")
	mustGit(t, "-C", dir, "commit", "-q", "-m", message)
}

// branchesNamed loads the named branches from the test repo.
func branchesNamed(t *testing.T, names ...string) []Branch {
	t.Helper()
	_, all, err := loadBranches()
	if err != nil {
		t.Fatal(err)
	}
	var branches []Branch
	for _, name := range names {
		i := indexOf(all, name)
		if i < 0 {
			t.Fatalf("branch %q not found", name)
		}
		branches = append(branches, all[i])
	}
	return branches
}

func loadBranch(t *testing.T, name string) Branch {
	t.Helper()
	_, branches, err := loadBranches()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range branches {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("branch %q not found", name)
	return Branch{}
}

func TestLoadBranchesDetectsRebaseInProgress(t *testing.T) {
	newTestRepo(t)
	mustGit(t, "switch", "-q", "-c", "feature")
	commitFile(t, ".", "one")
	// Stop the rebase at its first commit, like pausing to fix a conflict.
	t.Setenv("GIT_SEQUENCE_EDITOR", "sed -i.bak s/^pick/edit/")
	mustGit(t, "rebase", "-q", "-i", "HEAD~1")

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
	if results := deleteBranches(branchesNamed(t, "feature"), "main"); results[0].Err == nil {
		t.Error("expected git to refuse deleting a branch that's being rebased")
	}
}

func TestLoadBranchesDetectsBisectInOtherWorktree(t *testing.T) {
	newTestRepo(t, "feature")
	wt := filepath.Join(t.TempDir(), "bisect-wt")
	mustGit(t, "worktree", "add", "-q", wt, "feature")
	for _, message := range []string{"one", "two", "three", "four"} {
		commitFile(t, wt, message)
	}
	mustGit(t, "-C", wt, "bisect", "start")
	mustGit(t, "-C", wt, "bisect", "bad")
	mustGit(t, "-C", wt, "bisect", "good", "HEAD~4")

	feature := loadBranch(t, "feature")
	// This is why the state files are needed: bisect detached the worktree's
	// HEAD, so git no longer reports the branch as checked out there.
	if feature.Worktree != "" {
		t.Fatalf("test setup: expected bisect to detach HEAD, but Worktree = %q", feature.Worktree)
	}
	if feature.InProgress != "bisecting" || !feature.Protected("main") {
		t.Errorf("feature = %+v, want protected and marked bisecting", feature)
	}
	if results := deleteBranches(branchesNamed(t, "feature"), "main"); results[0].Err == nil {
		t.Error("expected git to refuse deleting a branch that's being bisected")
	}
}

func TestLoadBranchesWithOddWorktreePath(t *testing.T) {
	newTestRepo(t, "review")
	wt := filepath.Join(t.TempDir(), "odd\tname\nwith newline")
	mustGit(t, "worktree", "add", "-q", wt, "review")

	review := loadBranch(t, "review") // fails if the branch dropped out of the list
	wantPath, _ := filepath.EvalSymlinks(wt)
	if review.Worktree != wantPath {
		t.Errorf("Worktree = %q, want %q", review.Worktree, wantPath)
	}
	if review.Author != "Test" {
		t.Errorf("fields after the path are misaligned: Author = %q", review.Author)
	}
}

func TestLoadBranchesWithTagNamedLikeBranches(t *testing.T) {
	newTestRepo(t, "v1")
	mustGit(t, "tag", "v1") // same name as a branch
	// A tag named like the base branch, on a commit that isn't in the branch.
	mustGit(t, "switch", "-q", "-c", "side")
	mustGit(t, "commit", "-q", "--allow-empty", "-m", "side")
	mustGit(t, "tag", "main")
	mustGit(t, "branch", "only-in-tag")

	base, branches, err := loadBranches()
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]Branch)
	for _, b := range branches {
		byName[b.Name] = b
	}
	if base != "main" {
		t.Errorf("base = %q, want main", base)
	}
	if _, ok := byName["main"]; !ok {
		t.Fatalf("main is missing; names were shortened to avoid the tag: %v", slices.Collect(maps.Keys(byName)))
	}
	if !byName["main"].Protected(base) {
		t.Error("the base branch must stay protected when a tag shares its name")
	}
	if byName["only-in-tag"].Merged {
		t.Error("only-in-tag is merged into the tag main, not the branch main")
	}
	if results := deleteBranches(branchesNamed(t, "v1"), "main"); results[0].Err != nil {
		t.Errorf("a branch that shares a tag's name should be deletable: %v", results[0].Err)
	}
}

func TestBaseBranchFromOriginHeadWhenAmbiguous(t *testing.T) {
	newTestRepo(t, "trunk", "origin/trunk") // local branch named like the remote one
	mustGit(t, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	mustGit(t, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")

	if got := baseBranch(); got != "trunk" {
		t.Errorf("baseBranch = %q, want trunk (origin's default branch)", got)
	}
}

func TestDeleteBranchNamedLikeAnOption(t *testing.T) {
	newTestRepo(t)
	// git branch refuses names starting with "-", but plumbing can create them.
	for _, name := range []string{"-r", "--all"} {
		mustGit(t, "update-ref", "refs/heads/"+name, "HEAD")
	}

	for _, r := range previewDeletes(branchesNamed(t, "-r", "--all")) {
		if r.Err != nil {
			t.Errorf("preview of %q: %v", r.Name, r.Err)
		}
	}
	for _, r := range deleteBranches(branchesNamed(t, "-r", "--all"), "main") {
		if r.Err != nil {
			t.Errorf("delete of %q: %v", r.Name, r.Err)
		}
		if branchExists(r.Name) {
			t.Errorf("%q should be deleted", r.Name)
		}
	}
}

func TestDeleteBranchesInSeveralBatches(t *testing.T) {
	newTestRepo(t)
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
	tip := mustGit(t, "rev-parse", "HEAD")

	results := deleteBranches(branchesNamed(t, names...), "main")
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
	newTestRepo(t, "a", "busy", "b")
	mustGit(t, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "busy")
	branches := branchesNamed(t, "a", "busy", "b")
	mustGit(t, "branch", "-D", "b") // deleted by someone else after loading

	results := deleteBranches(branches, "main")
	if results[0].Err != nil || branchExists("a") {
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
	newTestRepo(t, "feature")
	loaded := branchesNamed(t, "feature")
	// Someone commits to the branch after the list was loaded.
	mustGit(t, "switch", "-q", "feature")
	commitFile(t, ".", "new work")
	mustGit(t, "switch", "-q", "main")

	results := deleteBranches(loaded, "main")
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "changed since") {
		t.Errorf("err = %v, want a 'changed since' refusal", results[0].Err)
	}
	if !branchExists("feature") {
		t.Fatal("a branch with commits the user hasn't seen must not be deleted")
	}
}

func TestDeleteSkipsBranchNoLongerMerged(t *testing.T) {
	newTestRepo(t)
	commitFile(t, ".", "second")
	mustGit(t, "branch", "done") // merged: points at main's tip
	loaded := branchesNamed(t, "done")
	if !loaded[0].Merged {
		t.Fatal("test setup: done should start out merged")
	}
	mustGit(t, "reset", "-q", "--hard", "HEAD~1") // main drops the commit

	results := deleteBranches(loaded, "main")
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "no longer merged") {
		t.Errorf("err = %v, want a 'no longer merged' refusal", results[0].Err)
	}
	if !branchExists("done") {
		t.Fatal("a branch that stopped being merged must not be deleted without a warning")
	}
}
