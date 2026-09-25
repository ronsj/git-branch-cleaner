package tui

import (
	"strings"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/git"
)

func TestWorktreeTag(t *testing.T) {
	m := loadedModel()
	b := git.Branch{Name: "review", Worktree: "/work/review"}
	if tags := m.renderTags(b); !strings.Contains(tags, "worktree") {
		t.Errorf("tags = %q, want a worktree label", tags)
	}
	current := git.Branch{Name: "wip", Current: true, Worktree: "/work/app"}
	if tags := m.renderTags(current); strings.Contains(tags, "worktree") {
		t.Errorf("the current branch's own worktree shouldn't be labeled: %q", tags)
	}
}

func TestInProgressTag(t *testing.T) {
	m := loadedModel()
	b := git.Branch{Name: "feature", InProgress: "rebasing"}
	if tags := m.renderTags(b); !strings.Contains(tags, "rebasing") {
		t.Errorf("tags = %q, want a rebasing label", tags)
	}
}

func TestMergedShownOnProtectedBranches(t *testing.T) {
	m := loadedModel()
	inWorktree := git.Branch{Name: "release", Merged: true, Worktree: "/work/release"}
	if tags := m.renderTags(inWorktree); !strings.Contains(tags, "worktree") || !strings.Contains(tags, "merged") {
		t.Errorf("tags = %q, want both worktree and merged", tags)
	}
	base := git.Branch{Name: "main", Merged: true}
	if tags := m.renderTags(base); strings.Contains(tags, "merged") {
		t.Errorf("the base branch shouldn't be labeled merged: %q", tags)
	}
}

// With --base origin/main, the local main that tracks it is the base.
func TestBaseTagOnBranchTrackingRemoteBase(t *testing.T) {
	next, _ := New(Options{}).Update(branchesLoadedMsg{base: "origin/main"})
	m := next.(Model)
	main := git.Branch{Name: "main", Merged: true, Upstream: "refs/remotes/origin/main"}
	if tags := m.renderTags(main); !strings.Contains(tags, "base") || strings.Contains(tags, "merged") {
		t.Errorf("tags = %q, want base and not merged", tags)
	}
}
