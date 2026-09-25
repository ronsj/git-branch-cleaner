package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestParseBranches(t *testing.T) {
	out := "old-feature\t3 months ago\t \t[gone]\tAlex Kim\tAdd login form\n" +
		"main\t2 days ago\t*\t\tSam Lee\tMerge feature/login\n" +
		"wip\t5 minutes ago\t \t[ahead 2]\tSam Lee\tWIP: tabs\tin subject\n" +
		"empty-subject\t1 year, 2 months ago\t \t\tAlex Kim\t\n"

	got := parseBranches(out)
	want := []Branch{
		{Name: "old-feature", LastCommit: "3 months ago", Gone: true, Author: "Alex Kim", Subject: "Add login form"},
		{Name: "main", LastCommit: "2 days ago", Current: true, Author: "Sam Lee", Subject: "Merge feature/login"},
		{Name: "wip", LastCommit: "5 minutes ago", Author: "Sam Lee", Subject: "WIP: tabs\tin subject"},
		{Name: "empty-subject", LastCommit: "1 year, 2 months ago", Author: "Alex Kim"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBranches:\n got  %+v\n want %+v", got, want)
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
