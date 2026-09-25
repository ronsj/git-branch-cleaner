package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

func TestOlderThanHidesRecentBranches(t *testing.T) {
	m := loadedModelWith(Options{OlderThanDays: 30})
	want := []string{"main", "merged-feature", "gone-feature"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
	if header := m.renderHeader(); !strings.Contains(header, "2 newer than 30 days hidden") {
		t.Errorf("header should say how many are hidden: %q", header)
	}
}

func TestOlderThanAndFilterCombine(t *testing.T) {
	m := typeText(press(loadedModelWith(Options{OlderThanDays: 30}), "/"), "e")
	// "e" matches merged-feature, gone-feature, and experiment, but
	// experiment is only a day old.
	want := []string{"merged-feature", "gone-feature"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
}

func TestBaseOverrideIsUsedWhenLoading(t *testing.T) {
	testrepo.New(t, "develop")
	m := New(Options{BaseOverride: "develop"})
	loaded, ok := m.loadBranchesCmd()().(branchesLoadedMsg)
	if !ok || loaded.base != "develop" {
		t.Fatalf("loaded %+v, want base develop", loaded)
	}
}
