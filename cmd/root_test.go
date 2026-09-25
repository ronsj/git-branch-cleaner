package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/git"
)

func TestPrintHistory(t *testing.T) {
	sha := "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b"
	history := []git.DeleteResult{
		{Name: "old", SHA: sha},
		{Name: "moved", Err: errors.New("it changed since you selected it")},
	}
	var out strings.Builder
	printHistory(&out, history, false)

	want := "Deleted branch old (was 1a2b3c4).\n" +
		"  restore: git branch old " + sha + "\n" +
		"Didn't delete moved: it changed since you selected it\n"
	if out.String() != want {
		t.Errorf("printHistory printed:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestPrintHistoryDryRun(t *testing.T) {
	history := []git.DeleteResult{
		{Name: "old", SHA: "1a2b3c4", DryRun: true},
		{Name: "busy", DryRun: true, Err: errors.New("it's checked out in another worktree")},
	}
	var out strings.Builder
	printHistory(&out, history, true)

	want := "Dry run: no branches were deleted.\n" +
		"Would delete branch old (at 1a2b3c4).\n" +
		"Wouldn't delete busy: it's checked out in another worktree\n"
	if out.String() != want {
		t.Errorf("printHistory printed:\n%s\nwant:\n%s", out.String(), want)
	}
}
