package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Update is a pure function of (model, msg), so it can be tested like a reducer.

func daysAgo(n int) time.Time {
	return time.Now().Add(-time.Duration(n) * 24 * time.Hour)
}

// testBranches are listed oldest first, which is the default sort order.
func testBranches() []Branch {
	return []Branch{
		{Name: "main", Merged: true, CommitTime: daysAgo(90)},
		{Name: "merged-feature", Merged: true, CommitTime: daysAgo(60)},
		{Name: "gone-feature", Gone: true, CommitTime: daysAgo(40)},
		{Name: "wip", Current: true, CommitTime: daysAgo(2)},
		{Name: "experiment", CommitTime: daysAgo(1)},
	}
}

func loadedModelWith(opts options) model {
	next, _ := newModel(opts).Update(branchesLoadedMsg{base: "main", branches: testBranches()})
	return next.(model)
}

func loadedModel() model {
	return loadedModelWith(options{})
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

// runCmd runs a command, including every command inside a tea.Batch, and
// returns the messages they produce.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, runCmd(c)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// confirmDelete loads the branches in the current repo, selects name, and
// confirms deletion, returning the result message.
func confirmDelete(t *testing.T, dryRun bool, name string) branchesDeletedMsg {
	t.Helper()
	next, _ := newModel(options{dryRun: dryRun}).Update(loadBranchesCmd())
	m := next.(model)
	m.selected[name] = true
	m = press(m, "enter")

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	for _, msg := range runCmd(cmd) {
		if deleted, ok := msg.(branchesDeletedMsg); ok {
			return deleted
		}
	}
	t.Fatal("confirming produced no branchesDeletedMsg")
	return branchesDeletedMsg{}
}

func TestDryRunConfirmKeepsBranch(t *testing.T) {
	newTestRepo(t, "old")
	msg := confirmDelete(t, true, "old")
	if !branchExists("old") {
		t.Fatal("dry run deleted the branch")
	}
	if !strings.HasPrefix(msg.results[0].Output, "Would delete branch old") {
		t.Errorf("output = %q", msg.results[0].Output)
	}
}

func TestConfirmDeletesBranch(t *testing.T) {
	newTestRepo(t, "old")
	confirmDelete(t, false, "old")
	if branchExists("old") {
		t.Fatal("confirming without dry run should delete the branch")
	}
}

func TestDryRunIsVisible(t *testing.T) {
	m := loadedModel()
	m.dryRun = true
	if !strings.Contains(m.render(), "DRY RUN") {
		t.Error("list view should show the DRY RUN badge")
	}
	m = press(m, "a", "enter")
	if !strings.Contains(m.renderConfirm(), "nothing will actually be deleted") {
		t.Error("confirm screen should say nothing will be deleted")
	}
}

func TestConfirmFitsShortTerminal(t *testing.T) {
	var branches []Branch
	for i := range 30 {
		branches = append(branches, Branch{Name: fmt.Sprintf("old-%02d", i), Merged: true})
	}
	next, _ := newModel(options{dryRun: true}).Update(branchesLoadedMsg{base: "main", branches: branches})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m := press(next.(model), "a", "enter")

	screen := m.render()
	if h := lipgloss.Height(screen); h > 16 {
		t.Errorf("confirm screen is %d lines tall, terminal is 16:\n%s", h, screen)
	}
	for _, want := range []string{"y to preview", "…and ", "nothing will actually be deleted"} {
		if !strings.Contains(screen, want) {
			t.Errorf("confirm screen is missing %q:\n%s", want, screen)
		}
	}
}

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
	if got := visibleNames(next.(model))[0]; got != "experiment" {
		t.Fatalf("after reload the newest branch should still be first, got %q", got)
	}
}

func TestOlderThanHidesRecentBranches(t *testing.T) {
	m := loadedModelWith(options{olderThanDays: 30})
	want := []string{"main", "merged-feature", "gone-feature"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
	if header := m.renderHeader(); !strings.Contains(header, "2 newer than 30 days hidden") {
		t.Errorf("header should say how many are hidden: %q", header)
	}
}

func TestOlderThanAndFilterCombine(t *testing.T) {
	m := typeText(press(loadedModelWith(options{olderThanDays: 30}), "/"), "e")
	// "e" matches merged-feature, gone-feature, and experiment, but
	// experiment is only a day old.
	want := []string{"merged-feature", "gone-feature"}
	if got := visibleNames(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("visible %v, want %v", got, want)
	}
}

func TestOlderThanHidingEverything(t *testing.T) {
	m := loadedModelWith(options{olderThanDays: 365})
	if screen := m.render(); !strings.Contains(screen, "No branches are older than 365 days.") {
		t.Errorf("expected an explanation when every branch is hidden:\n%s", screen)
	}
}

func TestWorktreeBranchesCannotBeSelected(t *testing.T) {
	branches := testBranches()
	branches[1].Worktree = "/work/review" // merged-feature, checked out elsewhere
	next, _ := newModel(options{}).Update(branchesLoadedMsg{base: "main", branches: branches})
	m := press(next.(model), "a")

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
	if got := next.(model).selectedNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("selected %v after reload, want %v", got, want)
	}
}

func TestWorktreeTag(t *testing.T) {
	m := loadedModel()
	b := Branch{Name: "review", Worktree: "/work/review"}
	if tags := m.renderTags(b); !strings.Contains(tags, "worktree") {
		t.Errorf("tags = %q, want a worktree label", tags)
	}
	current := Branch{Name: "wip", Current: true, Worktree: "/work/app"}
	if tags := m.renderTags(current); strings.Contains(tags, "worktree") {
		t.Errorf("the current branch's own worktree shouldn't be labeled: %q", tags)
	}
}
