package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

func TestConfirmFitsShortTerminal(t *testing.T) {
	var branches []git.Branch
	for i := range 30 {
		branches = append(branches, git.Branch{Name: fmt.Sprintf("old-%02d", i), Merged: true})
	}
	next, _ := New(Options{DryRun: true}).Update(branchesLoadedMsg{base: "main", branches: branches})
	next, _ = next.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	m := press(next.(Model), "a", "enter")

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

// With no base branch, merges weren't checked, so the confirm screen mustn't
// claim the branches aren't merged into an empty name.
func TestConfirmWithoutBase(t *testing.T) {
	next, _ := New(Options{}).Update(branchesLoadedMsg{base: "", branches: []git.Branch{{Name: "old"}}})
	screen := press(next.(Model), "space", "enter").render()
	for _, want := range []string{"old  not checked", "1 branch wasn't checked for merges", "--base"} {
		if !strings.Contains(ansi.Strip(screen), want) {
			t.Errorf("confirm screen is missing %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "not merged into") {
		t.Errorf("confirm screen names an empty base branch:\n%s", screen)
	}
}

func TestConfirmCountsBranches(t *testing.T) {
	for _, tt := range []struct {
		branches []git.Branch
		keys     []string
		want     string
	}{
		{[]git.Branch{{Name: "a"}}, []string{"space", "enter"}, "1 branch is not merged"},
		{[]git.Branch{{Name: "a"}, {Name: "b"}}, []string{"space", "j", "space", "enter"}, "2 branches are not merged"},
	} {
		next, _ := New(Options{}).Update(branchesLoadedMsg{base: "main", branches: tt.branches})
		m := press(next.(Model), tt.keys...)
		if screen := ansi.Strip(m.renderConfirm()); !strings.Contains(screen, tt.want) {
			t.Errorf("confirm screen is missing %q:\n%s", tt.want, screen)
		}
	}
}
