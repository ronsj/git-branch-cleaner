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

	var list []string
	unmerged := 0
	for _, b := range m.selectedBranches() {
		line := "  " + b.Name
		if !b.Merged {
			unmerged++
			line += warnStyle.Render("  not merged")
		}
		list = append(list, line)
	}

	var notes []string
	if unmerged > 0 {
		notes = append(notes, warnStyle.Render(fmt.Sprintf(
			"%d branch(es) are not merged into %s. Their commits will only be\nrecoverable via the SHA printed after deletion.", unmerged, m.base)))
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
