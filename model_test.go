package main

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Update is a pure function of (model, msg), so it can be tested like a reducer.

func loadedModel() model {
	m := newModel()
	next, _ := m.Update(branchesLoadedMsg{
		base: "main",
		branches: []Branch{
			{Name: "main", Merged: true},
			{Name: "merged-feature", Merged: true},
			{Name: "gone-feature", Gone: true},
			{Name: "wip", Current: true},
			{Name: "experiment"},
		},
	})
	return next.(model)
}

func press(m model, keys ...string) model {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

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
		branches: []Branch{{Name: "main"}, {Name: "wip", Current: true}, {Name: "experiment"}},
	})
	m = next.(model)
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

func TestRowsFitTerminalWidth(t *testing.T) {
	m := loadedModel()
	m.branches[1].Subject = strings.Repeat("a very long commit subject ", 10)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = next.(model)

	for line := range strings.SplitSeq(m.renderList(), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line is %d cells wide, terminal is 60: %q", w, line)
		}
	}
}

// typeText presses each character of text as its own key.
func typeText(m model, text string) model {
	for _, r := range text {
		m = press(m, string(r))
	}
	return m
}

func visibleNames(m model) []string {
	var names []string
	for _, b := range m.visibleBranches() {
		names = append(names, b.Name)
	}
	return names
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

func TestHiddenSelectionsAreCalledOut(t *testing.T) {
	m := press(loadedModel(), "a") // selects merged-feature and gone-feature
	m = press(typeText(press(m, "/"), "gone"), "enter")

	if got := m.renderSelectionCount(); !strings.Contains(got, "2 selected (1 hidden by filter)") {
		t.Fatalf("selection count = %q", got)
	}
	m = press(m, "enter")
	if confirm := m.renderConfirm(); !strings.Contains(confirm, "merged-feature") {
		t.Fatal("confirm screen should list selections hidden by the filter")
	}
}

func TestToggleOnEmptyFilterResult(t *testing.T) {
	m := press(typeText(press(loadedModel(), "/"), "nope"), "enter", "space")
	if len(m.selectedNames()) != 0 {
		t.Fatal("toggling with no visible branches should do nothing")
	}
}
