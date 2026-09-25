package git

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

// row places one branch's field values at their positions in a record.
func row(name, unixDate, head, upstream, upstreamTrack, worktree, author, subject, sha string) []string {
	var f [numFields]string
	f[fieldName], f[fieldUnixDate] = name, unixDate
	f[fieldHead], f[fieldUpstream], f[fieldUpstreamTrack], f[fieldWorktree] = head, upstream, upstreamTrack, worktree
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
		row("old-feature", "1750000000", " ", "refs/remotes/origin/old-feature", "[gone]", "/work/review", "Alex Kim", "Add login form", "aaaa111"),
		row("main", "1757000000", "*", "refs/remotes/origin/main", "", "/work/app", "Sam Lee", "Merge feature/login", "bbbb222"),
		row("wip", "1757100000", " ", "", "[ahead 2]", "/work/odd\tpath\nwith newline", "Sam Lee", "WIP: tabs\tin subject", "cccc333"),
		row("empty-subject", "1720000000", " ", "", "", "", "Alex Kim", "", "dddd444"),
		row("bad-time", "not-a-number", " ", "", "", "", "Alex Kim", "", "eeee555"),
		// A terminal would act on these instead of showing them.
		row("escapes", "1720000000", " ", "", "", "", "\x1b[31mMallory", "Fix\r\x1b]8;;https://evil.example\x07link\u009b2J", "ffff666"),
	)

	got := parseBranches(out)
	want := []Branch{
		{Name: "old-feature", CommitTime: time.Unix(1750000000, 0), Upstream: "refs/remotes/origin/old-feature", Gone: true, Worktree: "/work/review", Author: "Alex Kim", Subject: "Add login form", SHA: "aaaa111"},
		{Name: "main", CommitTime: time.Unix(1757000000, 0), Current: true, Upstream: "refs/remotes/origin/main", Worktree: "/work/app", Author: "Sam Lee", Subject: "Merge feature/login", SHA: "bbbb222"},
		{Name: "wip", CommitTime: time.Unix(1757100000, 0), Worktree: "/work/odd\tpath\nwith newline", Author: "Sam Lee", Subject: "WIP: tabs in subject", SHA: "cccc333"},
		{Name: "empty-subject", CommitTime: time.Unix(1720000000, 0), Author: "Alex Kim", SHA: "dddd444"},
		{Name: "bad-time", CommitTime: time.Unix(0, 0), Author: "Alex Kim", SHA: "eeee555"},
		{Name: "escapes", CommitTime: time.Unix(1720000000, 0), Author: "[31mMallory", Subject: "Fix]8;;https://evil.examplelink2J", SHA: "ffff666"},
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
	out := forEachRefOutput(row("a", "1", " ", "", "", "", "Sam", "", "ffff666")) + "\n"
	if got := parseBranches(out); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("parseBranches = %+v, want the one branch", got)
	}
}

func TestParseBranchesEmpty(t *testing.T) {
	if got := parseBranches(""); len(got) != 0 {
		t.Errorf("expected no branches, got %+v", got)
	}
}

func TestLoadBranches(t *testing.T) {
	testrepo.New(t, "done")
	testrepo.Git(t, "switch", "-q", "-c", "wip")
	testrepo.Git(t, "commit", "-q", "--allow-empty", "-m", "Work in progress")

	base, branches, err := LoadBranches("")
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

func TestLoadBranchesDetectsOtherWorktrees(t *testing.T) {
	testrepo.New(t, "review")
	wt := filepath.Join(t.TempDir(), "review-wt")
	testrepo.Git(t, "worktree", "add", "-q", wt, "review")

	_, branches, err := LoadBranches("")
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
	// Ask git directly: the app skips protected branches before git sees them.
	if _, err := git("branch", "-D", "--", "review"); err == nil {
		t.Error("expected git to refuse deleting a branch checked out in a worktree")
	}
}

func TestLoadBranchesWithOddWorktreePath(t *testing.T) {
	testrepo.New(t, "review")
	wt := filepath.Join(t.TempDir(), "odd\tname\nwith newline")
	testrepo.Git(t, "worktree", "add", "-q", wt, "review")

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
	testrepo.New(t, "v1")
	testrepo.Git(t, "tag", "v1") // same name as a branch
	// A tag named like the base branch, on a commit that isn't in the branch.
	testrepo.Git(t, "switch", "-q", "-c", "side")
	testrepo.Git(t, "commit", "-q", "--allow-empty", "-m", "side")
	testrepo.Git(t, "tag", "main")
	testrepo.Git(t, "branch", "only-in-tag")

	base, branches, err := LoadBranches("")
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
	if results := DeleteBranches(branchesNamed(t, "v1"), "main"); results[0].Err != nil {
		t.Errorf("a branch that shares a tag's name should be deletable: %v", results[0].Err)
	}
}

func TestBaseBranchFromOriginHeadWhenAmbiguous(t *testing.T) {
	testrepo.New(t, "trunk", "origin/trunk") // local branch named like the remote one
	testrepo.Git(t, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	testrepo.Git(t, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")

	if got := baseBranch(); got != "trunk" {
		t.Errorf("baseBranch = %q, want trunk (origin's default branch)", got)
	}
}

func TestLoadBranchesWithBaseOverride(t *testing.T) {
	testrepo.New(t)
	testrepo.Git(t, "switch", "-q", "-c", "develop")
	testrepo.CommitFile(t, ".", "develop work")
	testrepo.Git(t, "branch", "done") // merged into develop, not main
	testrepo.Git(t, "switch", "-q", "main")

	merged := func(base string) bool {
		t.Helper()
		gotBase, branches, err := LoadBranches(base)
		if err != nil {
			t.Fatal(err)
		}
		if base != "" && gotBase != base {
			t.Fatalf("base = %q, want %q", gotBase, base)
		}
		i := slices.IndexFunc(branches, func(b Branch) bool { return b.Name == "done" })
		return branches[i].Merged
	}
	if merged("") {
		t.Error("without --base, done isn't merged into main")
	}
	if !merged("develop") {
		t.Error("with --base develop, done is merged")
	}

	if _, _, err := LoadBranches("no-such-branch"); err == nil || !strings.Contains(err.Error(), "doesn't exist") {
		t.Errorf("err = %v, want a 'doesn't exist' error", err)
	}
}

// Local main is often behind origin/main, so --base origin/main sees merges
// the local main doesn't have yet. Local main, which tracks it, is the base.
func TestLoadBranchesWithRemoteTrackingBase(t *testing.T) {
	testrepo.New(t)
	testrepo.Git(t, "remote", "add", "origin", t.TempDir())
	testrepo.Git(t, "switch", "-q", "-c", "feature")
	testrepo.CommitFile(t, ".", "feature work")
	// origin/main has merged feature; local main hasn't pulled it yet.
	testrepo.Git(t, "update-ref", "refs/remotes/origin/main", "HEAD")
	testrepo.Git(t, "switch", "-q", "main")
	testrepo.Git(t, "branch", "-q", "--set-upstream-to=origin/main")

	base, branches, err := LoadBranches("origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if base != "origin/main" {
		t.Errorf("base = %q, want origin/main", base)
	}
	byName := make(map[string]Branch)
	for _, b := range branches {
		byName[b.Name] = b
	}
	if !byName["feature"].Merged {
		t.Error("feature is merged into origin/main")
	}
	if main := byName["main"]; !main.IsBase(base) || !main.Protected(base) {
		t.Errorf("main = %+v, want it treated as the base, since it tracks origin/main", main)
	}
	if loadBranch(t, "feature").Merged {
		t.Error("without --base, feature isn't merged into local main")
	}
}
