package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

// Update is a pure function of (model, msg), so it can be tested like a reducer.

func daysAgo(n int) time.Time {
	return time.Now().Add(-time.Duration(n) * 24 * time.Hour)
}

// testBranches are listed oldest first, which is the default sort order.
func testBranches() []git.Branch {
	return []git.Branch{
		{Name: "main", Merged: true, CommitTime: daysAgo(90)},
		{Name: "merged-feature", Merged: true, CommitTime: daysAgo(60)},
		{Name: "gone-feature", Gone: true, CommitTime: daysAgo(40)},
		{Name: "wip", Current: true, CommitTime: daysAgo(2)},
		{Name: "experiment", CommitTime: daysAgo(1)},
	}
}

func loadedModelWith(opts Options) Model {
	next, _ := New(opts).Update(branchesLoadedMsg{base: "main", branches: testBranches()})
	return next.(Model)
}

func loadedModel() Model {
	return loadedModelWith(Options{})
}

func press(m Model, keys ...string) Model {
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
		m = next.(Model)
	}
	return m
}

// typeText presses each character of text as its own key.
func typeText(m Model, text string) Model {
	for _, r := range text {
		m = press(m, string(r))
	}
	return m
}

func visibleNames(m Model) []string {
	var names []string
	for _, b := range m.visibleBranches() {
		names = append(names, b.Name)
	}
	return names
}
