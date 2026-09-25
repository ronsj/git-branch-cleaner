package tui

import (
	"reflect"
	"testing"
)

func TestSortCyclesAndKeepsCursor(t *testing.T) {
	m := press(loadedModel(), "j") // cursor on merged-feature

	m = press(m, "s")
	want := []string{"experiment", "wip", "gone-feature", "merged-feature", "main"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("newest first = %v, want %v", got, want)
	}
	if b, _ := m.cursorBranch(); b.Name != "merged-feature" {
		t.Fatalf("cursor moved to %q after sorting", b.Name)
	}

	m = press(m, "s")
	want = []string{"experiment", "gone-feature", "main", "merged-feature", "wip"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("by name = %v, want %v", got, want)
	}

	m = press(m, "s")
	if m.sortBy != sortOldest {
		t.Fatalf("sort should cycle back to oldest first, got %v", m.sortBy)
	}
}

func TestSortSurvivesReload(t *testing.T) {
	m := press(loadedModel(), "s") // newest first
	next, _ := m.Update(branchesLoadedMsg{base: "main", branches: testBranches()})
	if got := visibleNames(next.(Model))[0]; got != "experiment" {
		t.Fatalf("after reload the newest branch should still be first, got %q", got)
	}
}
