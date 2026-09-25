package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

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
		[]string{"old-feature", "3 months ago", "1750000000", " ", "[gone]", "/work/review", "Alex Kim", "Add login form"},
		[]string{"main", "2 days ago", "1757000000", "*", "", "/work/app", "Sam Lee", "Merge feature/login"},
		[]string{"wip", "5 minutes ago", "1757100000", " ", "[ahead 2]", "/work/odd\tpath\nwith newline", "Sam Lee", "WIP: tabs\tin subject"},
		[]string{"empty-subject", "1 year, 2 months ago", "1720000000", " ", "", "", "Alex Kim", ""},
		[]string{"bad-time", "1 day ago", "not-a-number", " ", "", "", "Alex Kim", ""},
	)

	got := parseBranches(out)
	want := []Branch{
		{Name: "old-feature", LastCommit: "3 months ago", CommitTime: time.Unix(1750000000, 0), Gone: true, Worktree: "/work/review", Author: "Alex Kim", Subject: "Add login form"},
		{Name: "main", LastCommit: "2 days ago", CommitTime: time.Unix(1757000000, 0), Current: true, Worktree: "/work/app", Author: "Sam Lee", Subject: "Merge feature/login"},
		{Name: "wip", LastCommit: "5 minutes ago", CommitTime: time.Unix(1757100000, 0), Worktree: "/work/odd\tpath\nwith newline", Author: "Sam Lee", Subject: "WIP: tabs\tin subject"},
		{Name: "empty-subject", LastCommit: "1 year, 2 months ago", CommitTime: time.Unix(1720000000, 0), Author: "Alex Kim"},
		{Name: "bad-time", LastCommit: "1 day ago", CommitTime: time.Unix(0, 0), Author: "Alex Kim"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranches:\n got  %+v\n want %+v", got, want)
	}
}

func TestParseBranchesWithTrailingNewline(t *testing.T) {
	out := forEachRefOutput([]string{"a", "now", "1", " ", "", "", "Sam", ""}) + "\n"
	if got := parseBranches(out); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("parseBranches = %+v, want the one branch", got)
	}
}

func TestParseBranchesEmpty(t *testing.T) {
	if got := parseBranches(""); len(got) != 0 {
		t.Errorf("expected no branches, got %+v", got)
	}
}

func TestRestoreSHA(t *testing.T) {
	tests := []struct {
		output, want string
	}{
		{"Deleted branch feature/x (was 1a2b3c4).", "1a2b3c4"},
		{"Deleted branch fix-(parens) (was abcdef0).", "abcdef0"},
		{"something unexpected", ""},
	}
	for _, tt := range tests {
		if got := restoreSHA(tt.output); got != tt.want {
			t.Errorf("restoreSHA(%q) = %q, want %q", tt.output, got, tt.want)
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

	results := previewDeletes([]string{"old", "missing"})

	if results[0].Err != nil || !strings.HasPrefix(results[0].Output, "Would delete branch old (at ") {
		t.Errorf("preview of old = %+v", results[0])
	}
	if results[1].Err == nil {
		t.Error("previewing a branch that doesn't exist should report an error")
	}
	if !branchExists("old") {
		t.Fatal("a dry run must not delete the branch")
	}
}

func TestDeleteBranchesCanBeRestored(t *testing.T) {
	newTestRepo(t, "old")

	results := deleteBranches([]string{"old"})
	if results[0].Err != nil {
		t.Fatal(results[0].Err)
	}
	if branchExists("old") {
		t.Fatal("branch should be deleted")
	}

	// The restore command printed on exit should bring it back.
	mustGit(t, "branch", "old", restoreSHA(results[0].Output))
	if !branchExists("old") {
		t.Fatal("branch should be restored from the SHA in git's output")
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
	if results := deleteBranches([]string{"review"}); results[0].Err == nil {
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
	if results := deleteBranches([]string{"feature"}); results[0].Err == nil {
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
	if results := deleteBranches([]string{"feature"}); results[0].Err == nil {
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
