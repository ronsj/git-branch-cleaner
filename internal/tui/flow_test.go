package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

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
	m := New(Options{DryRun: dryRun})
	next, _ := m.Update(m.loadBranchesCmd()())
	m = next.(Model)
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
	testrepo.New(t, "old")
	msg := confirmDelete(t, true, "old")
	if !testrepo.BranchExists("old") {
		t.Fatal("dry run deleted the branch")
	}
	if !strings.HasPrefix(msg.results[0].String(), "Would delete branch old") {
		t.Errorf("output = %q", msg.results[0].String())
	}
}

func TestConfirmDeletesBranch(t *testing.T) {
	testrepo.New(t, "old")
	confirmDelete(t, false, "old")
	if testrepo.BranchExists("old") {
		t.Fatal("confirming without dry run should delete the branch")
	}
}
