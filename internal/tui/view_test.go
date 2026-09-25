package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

func TestRowsFitTerminalWidth(t *testing.T) {
	m := loadedModel()
	m.branches[1].Subject = strings.Repeat("a very long commit subject ", 10)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	m = next.(Model)

	for line := range strings.SplitSeq(m.renderList(m.visibleBranches(), m.listHeight()), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line is %d cells wide, terminal is 60: %q", w, line)
		}
	}
}

func TestHiddenSelectionsAreCalledOut(t *testing.T) {
	m := press(loadedModel(), "a") // selects merged-feature and gone-feature
	m = press(typeText(press(m, "/"), "gone"), "enter")

	if got := m.renderSelectionCount(m.visibleBranches()); !strings.Contains(got, "2 selected (1 hidden by filter)") {
		t.Fatalf("selection count = %q", got)
	}
	m = press(m, "enter")
	if confirm := m.renderConfirm(); !strings.Contains(confirm, "merged-feature") {
		t.Fatal("confirm screen should list selections hidden by the filter")
	}
}

func TestDryRunIsVisible(t *testing.T) {
	m := loadedModel()
	m.DryRun = true
	if !strings.Contains(m.render(), "DRY RUN") {
		t.Error("list view should show the DRY RUN badge")
	}
	m = press(m, "a", "enter")
	if !strings.Contains(m.renderConfirm(), "nothing will actually be deleted") {
		t.Error("confirm screen should say nothing will be deleted")
	}
}

func TestOlderThanHidingEverything(t *testing.T) {
	m := loadedModelWith(Options{OlderThanDays: 365})
	if screen := m.render(); !strings.Contains(screen, "No branches are older than 365 days.") {
		t.Errorf("expected an explanation when every branch is hidden:\n%s", screen)
	}
}

func TestManyResultsFitTheScreen(t *testing.T) {
	m := loadedModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = next.(Model)
	for i := range 40 {
		m.lastResults = append(m.lastResults, git.DeleteResult{Name: fmt.Sprintf("old-%02d", i), SHA: "abc1234"})
	}
	m.lastResults = append(m.lastResults, git.DeleteResult{
		Name: "busy",
		Err:  fmt.Errorf("git branch: error: cannot delete branch 'busy' used by worktree at '/a/very/long/path/that/would/wrap/on/a/narrow/screen'"),
	})

	screen := m.render()
	if h := lipgloss.Height(screen); h > 24 {
		t.Errorf("screen is %d lines tall, terminal is 24:\n%s", h, screen)
	}
	for line := range strings.SplitSeq(screen, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("line is %d wide, terminal is 60: %q", w, line)
		}
	}
	if !strings.Contains(screen, "✗") {
		t.Error("the failure must stay visible even when results are cut short")
	}
	if !strings.Contains(screen, "all listed when you quit") {
		t.Error("expected a note that the rest are listed on exit")
	}
}

func TestListFillsScreenExactly(t *testing.T) {
	var branches []git.Branch
	for i := range 60 {
		branches = append(branches, git.Branch{Name: fmt.Sprintf("b-%02d", i), CommitTime: daysAgo(100 - i)})
	}
	next, _ := New(Options{}).Update(branchesLoadedMsg{base: "main", branches: branches})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m := next.(Model)
	for range 30 {
		m = press(m, "j") // mid-list, so both "more" markers show
	}

	for _, fullHelp := range []bool{false, true} {
		if fullHelp {
			m = press(m, "?")
		}
		if h := lipgloss.Height(m.render()); h != 20 {
			t.Errorf("full help %v: screen is %d lines, want exactly the terminal's 20", fullHelp, h)
		}
	}
}

// Run with: go test -run '^$' -bench .
func BenchmarkRender(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		var branches []git.Branch
		for i := range n {
			branches = append(branches, git.Branch{
				Name: fmt.Sprintf("feature/branch-%05d", i), Merged: true, CommitTime: daysAgo(n - i),
				Author: "Alex Kim", Subject: "Some commit subject",
			})
		}
		next, _ := New(Options{}).Update(branchesLoadedMsg{base: "main", branches: branches})
		next, _ = next.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		m := press(next.(Model), "a") // worst case: everything selected

		b.Run(fmt.Sprintf("branches=%d", n), func(b *testing.B) {
			for b.Loop() {
				_ = m.render()
			}
		})
	}
}
