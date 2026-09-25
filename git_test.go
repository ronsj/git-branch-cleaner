package main

import (
	"reflect"
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
