package main

import (
	"reflect"
	"testing"
)

func TestParseBranches(t *testing.T) {
	out := "old-feature\t3 months ago\t \t[gone]\n" +
		"main\t2 days ago\t*\t\n" +
		"wip\t5 minutes ago\t \t[ahead 2]\n"

	got := parseBranches(out)
	want := []Branch{
		{Name: "old-feature", LastCommit: "3 months ago", Gone: true},
		{Name: "main", LastCommit: "2 days ago", Current: true},
		{Name: "wip", LastCommit: "5 minutes ago"},
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
