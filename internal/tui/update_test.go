package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

func TestToggleSelection(t *testing.T) {
	m := press(loadedModel(), "j", "space")
	if !m.selected["merged-feature"] {
		t.Fatal("expected merged-feature to be selected")
	}
	m = press(m, "space")
	if m.selected["merged-feature"] {
		t.Fatal("expected second toggle to deselect")
	}
}

func TestProtectedBranchesCannotBeSelected(t *testing.T) {
	m := press(loadedModel(), "space")   // cursor on base branch
	m = press(m, "j", "j", "j", "space") // cursor on current branch
	if n := len(m.selectedNames()); n != 0 {
		t.Fatalf("expected nothing selected, got %v", m.selectedNames())
	}
}

func TestSelectStaleSkipsProtectedAndUnmerged(t *testing.T) {
	m := press(loadedModel(), "a")
	want := []string{"merged-feature", "gone-feature"}
	if got := m.selectedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v, want %v", got, want)
	}
}

func TestCursorStaysInBounds(t *testing.T) {
	m := press(loadedModel(), "k", "k")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d after moving up from top", m.cursor)
	}
	m = press(m, "j", "j", "j", "j", "j", "j", "j")
	if m.cursor != 4 {
		t.Fatalf("cursor = %d after moving past bottom", m.cursor)
	}
}

func TestCursorFollowsBranchAfterReload(t *testing.T) {
	m := press(loadedModel(), "j", "j", "j", "j") // on "experiment"
	next, _ := m.Update(branchesLoadedMsg{
		base:     "main",
		branches: []git.Branch{{Name: "main"}, {Name: "wip", Current: true}, {Name: "experiment"}},
	})
	m = next.(Model)
	if got := m.branches[m.cursor].Name; got != "experiment" {
		t.Fatalf("cursor on %q after reload, want experiment", got)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	m := press(loadedModel(), "enter")
	if m.state != stateBrowsing {
		t.Fatal("enter with nothing selected should not open confirm")
	}

	m = press(m, "a", "enter")
	if m.state != stateConfirming {
		t.Fatalf("state = %v, want confirming", m.state)
	}

	m = press(m, "n")
	if m.state != stateBrowsing || len(m.selectedNames()) != 2 {
		t.Fatal("cancel should return to the list and keep the selection")
	}
}

func TestFilterNarrowsList(t *testing.T) {
	m := typeText(press(loadedModel(), "/"), "FEAT")
	want := []string{"merged-feature", "gone-feature"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
	if !m.filter.Focused() {
		t.Fatal("filter should stay focused while typing")
	}
}

func TestFilterCapturesShortcutKeys(t *testing.T) {
	m := typeText(press(loadedModel(), "/"), "ja")
	if m.filter.Value() != "ja" {
		t.Fatalf("filter = %q, want the typed text", m.filter.Value())
	}
	if m.cursor != 0 || len(m.selectedNames()) != 0 {
		t.Fatal("j and a should type into the filter, not move or select")
	}
}

func TestArrowKeysMoveWhileFiltering(t *testing.T) {
	m := press(typeText(press(loadedModel(), "/"), "feat"), "down")
	if m.cursor != 1 || !m.filter.Focused() {
		t.Fatalf("cursor = %d, focused = %v; want 1, true", m.cursor, m.filter.Focused())
	}
}

func TestEnterKeepsFilterAndEscClearsIt(t *testing.T) {
	m := press(typeText(press(loadedModel(), "/"), "feat"), "enter")
	if m.filter.Focused() || len(m.visibleBranches()) != 2 {
		t.Fatal("enter should leave the filter applied but stop typing")
	}
	if m.state != stateBrowsing {
		t.Fatal("enter in the filter should not open the delete confirmation")
	}

	m = press(m, "esc")
	if m.filter.Value() != "" || len(m.visibleBranches()) != 5 {
		t.Fatal("esc should clear the filter")
	}
}

func TestSelectStaleOnlySelectsVisible(t *testing.T) {
	m := press(typeText(press(loadedModel(), "/"), "gone"), "enter", "a")
	want := []string{"gone-feature"}
	if got := m.selectedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v, want %v", got, want)
	}
}

func TestToggleOnEmptyFilterResult(t *testing.T) {
	m := press(typeText(press(loadedModel(), "/"), "nope"), "enter", "space")
	if len(m.selectedNames()) != 0 {
		t.Fatal("toggling with no visible branches should do nothing")
	}
}

func TestWorktreeBranchesCannotBeSelected(t *testing.T) {
	branches := testBranches()
	branches[1].Worktree = "/work/review" // merged-feature, checked out elsewhere
	next, _ := New(Options{}).Update(branchesLoadedMsg{base: "main", branches: branches})
	m := press(next.(Model), "a")

	if m.selected["merged-feature"] {
		t.Error("a should skip branches checked out in another worktree")
	}
	m = press(m, "j", "space")
	if m.selected["merged-feature"] {
		t.Error("space should not select a branch checked out in another worktree")
	}
}

func TestReloadDropsSelectionsThatBecameProtected(t *testing.T) {
	m := press(loadedModel(), "a") // selects merged-feature and gone-feature

	branches := testBranches()
	branches[1].Worktree = "/work/review" // merged-feature got checked out elsewhere
	next, _ := m.Update(branchesLoadedMsg{base: "main", branches: branches})

	want := []string{"gone-feature"}
	if got := next.(Model).selectedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v after reload, want %v", got, want)
	}
}

func TestCtrlCWaitsForDeletesToFinish(t *testing.T) {
	m := press(loadedModel(), "a", "enter", "y")
	if m.state != stateDeleting {
		t.Fatalf("state = %v, want deleting", m.state)
	}

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd != nil {
		t.Fatal("ctrl+c mid-delete should not quit yet")
	}
	if !strings.Contains(m.render(), "then quitting") {
		t.Error("the screen should say it will quit when deletion finishes")
	}

	results := []git.DeleteResult{{Name: "merged-feature", SHA: "abc1234"}}
	next, cmd = m.Update(branchesDeletedMsg{results})
	m = next.(Model)
	if len(m.history) != 1 {
		t.Fatal("the results should be kept for the exit summary")
	}
	if cmd == nil {
		t.Fatal("expected a quit command once the results arrived")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.Quit once the results arrived")
	}
}

func TestCtrlCQuitsImmediatelyWhenNotDeleting(t *testing.T) {
	_, cmd := loadedModel().Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c while browsing should quit right away")
	}
}
