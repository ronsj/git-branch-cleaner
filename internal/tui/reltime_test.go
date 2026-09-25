package tui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	tests := []struct {
		age  time.Duration
		want string
	}{
		{-time.Minute, "in the future"},
		{0, "0 seconds ago"},
		{89 * time.Second, "89 seconds ago"},
		{90 * time.Second, "2 minutes ago"},
		{89 * time.Minute, "89 minutes ago"},
		{90 * time.Minute, "2 hours ago"},
		{35 * time.Hour, "35 hours ago"},
		{36 * time.Hour, "2 days ago"},
		{13 * day, "13 days ago"},
		{14 * day, "2 weeks ago"},
		{69 * day, "10 weeks ago"},
		{70 * day, "2 months ago"}, // git rounds (70+15)/30 down to 2
		{364 * day, "12 months ago"},
		{365 * day, "1 year ago"},
		{400 * day, "1 year, 1 month ago"},
		{800 * day, "2 years, 2 months ago"},
		{1824 * day, "5 years ago"},
		{1825 * day, "5 years ago"},
		{4000 * day, "11 years ago"},
	}
	for _, tt := range tests {
		if got := relativeTime(now.Add(-tt.age), now); got != tt.want {
			t.Errorf("age %v: got %q, want %q", tt.age, got, tt.want)
		}
	}
}

// TestRelativeTimeMatchesGit compares against git's own --date=relative output
// for commits of various ages, away from the rounding boundaries so the
// second or two between git's clock and ours can't matter.
func TestRelativeTimeMatchesGit(t *testing.T) {
	testrepo.New(t)
	day := int64(24 * 60 * 60)
	ages := map[string]int64{
		"minutes": 45 * 60, "hours": 5 * 60 * 60, "days": 3*day + 60, "weeks": 20 * day,
		"months": 100 * day, "year-month": 400 * day, "years-months": 800 * day, "years": 2000 * day,
	}
	now := time.Now().Unix()
	for name, age := range ages {
		t.Setenv("GIT_COMMITTER_DATE", fmt.Sprintf("%d +0000", now-age))
		testrepo.Git(t, "commit", "-q", "--allow-empty", "-m", name)
		testrepo.Git(t, "branch", name)
	}

	out := testrepo.Git(t, "for-each-ref", "--format=%(refname:lstrip=2)%09%(committerdate:unix)%09%(committerdate:relative)", "refs/heads/")
	checked := 0
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(line, "\t")
		if _, ok := ages[fields[0]]; !ok {
			continue
		}
		unix, _ := strconv.ParseInt(fields[1], 10, 64)
		if got := relativeTime(time.Unix(unix, 0), time.Now()); got != fields[2] {
			t.Errorf("%s: got %q, git says %q", fields[0], got, fields[2])
		}
		checked++
	}
	if checked != len(ages) {
		t.Fatalf("compared %d branches, want %d", checked, len(ages))
	}
}
