package git

import (
	"slices"
	"testing"
)

// branchesNamed loads the named branches from the test repo.
func branchesNamed(t *testing.T, names ...string) []Branch {
	t.Helper()
	_, all, err := LoadBranches("")
	if err != nil {
		t.Fatal(err)
	}
	var branches []Branch
	for _, name := range names {
		i := slices.IndexFunc(all, func(b Branch) bool { return b.Name == name })
		if i < 0 {
			t.Fatalf("branch %q not found", name)
		}
		branches = append(branches, all[i])
	}
	return branches
}

func loadBranch(t *testing.T, name string) Branch {
	t.Helper()
	_, branches, err := LoadBranches("")
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
