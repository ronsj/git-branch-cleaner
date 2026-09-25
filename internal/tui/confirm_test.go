package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
