package git

import (
	"testing"
)

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

func TestIsBase(t *testing.T) {
	tracksOrigin := Branch{Name: "main", Upstream: "refs/remotes/origin/main"}
	tests := []struct {
		name   string
		branch Branch
		base   string
		want   bool
	}{
		{"local base", tracksOrigin, "main", true},
		{"tracks a remote-tracking base", tracksOrigin, "origin/main", true},
		{"tracks a different branch", Branch{Name: "feature", Upstream: "refs/remotes/origin/feature"}, "origin/main", false},
		{"no base", Branch{Name: "main"}, "", false},
	}
	for _, tt := range tests {
		if got := tt.branch.IsBase(tt.base); got != tt.want {
			t.Errorf("%s: IsBase(%q) = %v, want %v", tt.name, tt.base, got, tt.want)
		}
	}
}

func TestProtectedReason(t *testing.T) {
	tests := []struct {
		branch Branch
		want   string
	}{
		{Branch{Name: "feature"}, ""},
		{Branch{Name: "main", Current: true}, "it's the base branch"},
		{Branch{Name: "wip", Current: true, Worktree: "/work/app"}, "it's checked out"},
		{Branch{Name: "review", Worktree: "/work/review"}, "it's checked out in another worktree"},
		{Branch{Name: "feature", InProgress: "rebasing"}, "it's in use (rebasing)"},
	}
	for _, tt := range tests {
		if got := tt.branch.ProtectedReason("main"); got != tt.want {
			t.Errorf("%s: ProtectedReason = %q, want %q", tt.branch.Name, got, tt.want)
		}
	}
}
