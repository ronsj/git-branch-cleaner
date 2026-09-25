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

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		args       []string
		wantStatus int
		wantStderr string
	}{
		{[]string{"-h"}, 0, "Usage: git-branch-cleaner [flags]"},
		{[]string{"--help"}, 0, "-older-than N"},
		{[]string{"stray"}, 2, "unexpected argument: stray"},
		{[]string{"--dryrun"}, 2, "flag provided but not defined: -dryrun"},
		{[]string{"-older-than", "-1"}, 2, "-older-than must be 0 or more days, got -1"},
		{[]string{"--older-than", "abc"}, 2, `invalid value "abc" for flag -older-than`},
	}
	for _, tt := range tests {
		var stdout, stderr strings.Builder
		if status := run(tt.args, &stdout, &stderr); status != tt.wantStatus {
			t.Errorf("run(%q) = %d, want %d", tt.args, status, tt.wantStatus)
		}
		if !strings.Contains(stderr.String(), tt.wantStderr) {
			t.Errorf("run(%q) stderr = %q, want it to contain %q", tt.args, stderr.String(), tt.wantStderr)
		}
		if stdout.Len() != 0 {
			t.Errorf("run(%q) wrote to stdout: %q", tt.args, stdout.String())
		}
	}
}

// Execute used to register flags on the global flag set, which panics the
// second time. run must work any number of times.
func TestRunTwice(t *testing.T) {
	for range 2 {
		var stdout, stderr strings.Builder
		if status := run([]string{"-h"}, &stdout, &stderr); status != 0 {
			t.Fatalf("status = %d", status)
		}
	}
}
