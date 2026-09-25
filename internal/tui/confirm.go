package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) renderConfirm() string {
	heading, keyHint := "Delete these branches?", "y to delete • n/esc to cancel"
	if m.DryRun {
		heading, keyHint = "Preview deleting these branches?", "y to preview • n/esc to cancel"
	}

	// Without a base branch, merges couldn't be checked at all, so say that
	// rather than claim the branches aren't merged.
	label := "  not merged"
	if m.base == "" {
		label = "  not checked"
	}

	var list []string
	unmerged := 0
	for _, b := range m.selectedBranches() {
		line := "  " + b.Name
		if !b.Merged {
			unmerged++
			line += warnStyle.Render(label)
		}
		list = append(list, line)
	}

	var notes []string
	switch {
	case unmerged > 0 && m.base == "":
		notes = append(notes, warnStyle.Render(fmt.Sprintf(
			"%s checked for merges, since no base branch was\nfound (see --base). Their commits will only be recoverable via the\nSHA printed after deletion.",
			countBranches(unmerged, "wasn't", "weren't"))))
	case unmerged > 0:
		notes = append(notes, warnStyle.Render(fmt.Sprintf(
			"%s not merged into %s. Their commits will only be\nrecoverable via the SHA printed after deletion.",
			countBranches(unmerged, "is", "are"), m.base)))
	}
	if m.DryRun {
		notes = append(notes, dryRunStyle.Render("Dry run: nothing will actually be deleted."))
	}
	var footer []string
	if len(notes) > 0 {
		footer = append(append(footer, ""), notes...)
	}
	footer = append(footer, "", mutedStyle.Render(keyHint))

	// Shorten the list, never the footer: the key hint must stay on screen.
	if m.height > 0 {
		// Title and blank line (2), box border (2), heading and blank line (2).
		room := m.height - 6 - lipgloss.Height(strings.Join(footer, "\n"))
		list = fitLines(list, max(room, 1), "")
	}

	body := heading + "\n\n" + strings.Join(list, "\n") + "\n" + strings.Join(footer, "\n")
	return confirmBox.Render(body)
}

// countBranches starts a sentence about n branches: "1 branch is",
// "3 branches are", with the verb given for each.
func countBranches(n int, one, many string) string {
	if n == 1 {
		return "1 branch " + one
	}
	return fmt.Sprintf("%d branches %s", n, many)
}
