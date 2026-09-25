package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// listHeight is how many branch rows fit on screen: the terminal height
// minus the header, the footer (measured, not assumed), and the two
// "↑/↓ N more" lines.
func (m model) listHeight() int {
	if m.height == 0 {
		return len(m.visibleBranches()) // size unknown yet: show everything
	}
	const headerLines, scrollMarkers = 2, 2
	return max(m.height-headerLines-scrollMarkers-lipgloss.Height(m.renderFooter()), 3)
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "branch-cleaner"
	return v
}

func (m model) render() string {
	var s strings.Builder

	s.WriteString(m.renderHeader() + "\n")
	// The filter takes the blank line under the title, so the list doesn't jump.
	if m.state == stateBrowsing && (m.filter.Focused() || m.filter.Value() != "") {
		s.WriteString(m.filter.View())
	}
	s.WriteString("\n")

	if m.err != nil {
		s.WriteString(errorStyle.Render("Error: "+m.err.Error()) + "\n\n")
		s.WriteString(mutedStyle.Render("r to retry • q to quit"))
		return s.String()
	}

	switch m.state {
	case stateLoading:
		s.WriteString(m.spinner.View() + " Loading branches…")
		return s.String()
	case stateDeleting:
		switch {
		case m.quitting:
			s.WriteString(m.spinner.View() + " Finishing deletions, then quitting…")
		case m.dryRun:
			s.WriteString(m.spinner.View() + " Previewing deletions…")
		default:
			s.WriteString(m.spinner.View() + " Deleting branches…")
		}
		return s.String()
	case stateConfirming:
		s.WriteString(m.renderConfirm())
		return s.String()
	}

	switch {
	case len(m.branches) == 0:
		s.WriteString(mutedStyle.Render("No local branches found.") + "\n")
	case len(m.visibleBranches()) == 0 && m.filter.Value() != "":
		s.WriteString(mutedStyle.Render(fmt.Sprintf("No branches match %q.", m.filter.Value())) + "\n")
	case len(m.visibleBranches()) == 0:
		s.WriteString(mutedStyle.Render(fmt.Sprintf("No branches are older than %s.", days(m.olderThanDays))) + "\n")
	default:
		s.WriteString(m.renderList())
	}

	s.WriteString(m.renderFooter())
	return s.String()
}

// renderFooter is everything below the list: the last delete's results, the
// selection count, and the key help.
func (m model) renderFooter() string {
	var s strings.Builder
	s.WriteString("\n")
	for _, line := range m.resultLines() {
		s.WriteString(line + "\n")
	}
	s.WriteString(m.renderSelectionCount() + "\n")
	if m.filter.Focused() {
		s.WriteString(mutedStyle.Render("type to filter • ↑/↓ move • enter done • esc clear"))
	} else {
		k := keys
		k.ClearFilter.SetEnabled(m.filter.Value() != "")
		s.WriteString(m.help.View(k))
	}
	return s.String()
}

// resultLines renders the last delete's results, failures first so a long
// list can't hide them. At most a quarter of the screen is used; everything
// is listed again on exit.
func (m model) resultLines() []string {
	var failed, deleted []string
	for _, r := range m.lastResults {
		if r.Err != nil {
			failed = append(failed, errorStyle.Render("✗ "+r.Err.Error()))
		} else {
			deleted = append(deleted, mergedStyle.Render("✓ "+r.String()))
		}
	}
	lines := append(failed, deleted...)
	if m.width > 0 {
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, m.width, "…")
		}
	}
	if m.height == 0 || len(lines) <= max(m.height/4, 3) {
		return lines
	}
	shown := slices.Clone(lines[:max(m.height/4, 3)-1])
	return append(shown, mutedStyle.Render(
		fmt.Sprintf("  …and %d more (all listed when you quit)", len(lines)-len(shown))))
}

// renderHeader is the title line: app name, dry-run badge, base branch,
// sort order, and how many branches --older-than is hiding.
func (m model) renderHeader() string {
	header := titleStyle.Render("Branch Cleaner")
	if m.dryRun {
		header += dryRunStyle.Render("  DRY RUN")
	}

	var info []string
	if m.base != "" {
		info = append(info, "base: "+m.base)
	}
	info = append(info, m.sortBy.String())
	if m.olderThanDays > 0 {
		hidden := 0
		for _, b := range m.branches {
			if m.tooRecent(b) {
				hidden++
			}
		}
		info = append(info, fmt.Sprintf("%d newer than %s hidden", hidden, days(m.olderThanDays)))
	}
	header += mutedStyle.Render("  " + strings.Join(info, " · "))

	if m.width > 0 {
		header = ansi.Truncate(header, m.width, "…")
	}
	return header
}

// days formats a day count: "1 day", "30 days".
func days(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

// renderSelectionCount shows how many branches are selected, calling out any
// the filter is hiding so they aren't deleted by surprise.
func (m model) renderSelectionCount() string {
	selected := len(m.selectedBranches())
	if selected == 0 {
		return ""
	}
	shown := 0
	for _, b := range m.visibleBranches() {
		if m.selected[b.Name] {
			shown++
		}
	}
	hidden := selected - shown
	text := fmt.Sprintf("%d selected", selected)
	if hidden > 0 {
		text += fmt.Sprintf(" (%d hidden by filter)", hidden)
	}
	return selectedStyle.Render(text)
}

// maxAuthorWidth caps the author column so one long name can't crowd out subjects.
const maxAuthorWidth = 20

func (m model) renderList() string {
	// Size each column to its widest value so the columns line up.
	var nameWidth, dateWidth, tagWidth, authorWidth int
	for _, b := range m.branches {
		nameWidth = max(nameWidth, lipgloss.Width(b.Name))
		dateWidth = max(dateWidth, lipgloss.Width(b.LastCommit))
		tagWidth = max(tagWidth, lipgloss.Width(m.renderTags(b)))
		authorWidth = max(authorWidth, lipgloss.Width(b.Author))
	}
	authorWidth = min(authorWidth, maxAuthorWidth)

	visible := m.visibleBranches()
	var s strings.Builder
	end := min(m.offset+m.listHeight(), len(visible))
	if m.offset > 0 {
		s.WriteString(mutedStyle.Render(fmt.Sprintf("  ↑ %d more", m.offset)) + "\n")
	}

	for i := m.offset; i < end; i++ {
		b := visible[i]

		pointer := "  "
		if i == m.cursor {
			pointer = cursorStyle.Render("> ")
		}

		check := "[ ]"
		switch {
		case b.Protected(m.base):
			check = mutedStyle.Render(" - ")
		case m.selected[b.Name]:
			check = selectedStyle.Render("[x]")
		}

		name := padRight(b.Name, nameWidth)
		if i == m.cursor {
			name = cursorStyle.Render(name)
		}

		author := ansi.Truncate(b.Author, authorWidth, "…")

		row := fmt.Sprintf("%s%s %s  %s  %s  %s  %s",
			pointer, check, name,
			mutedStyle.Render(padRight(b.LastCommit, dateWidth)),
			padRight(m.renderTags(b), tagWidth),
			mutedStyle.Render(padRight(author, authorWidth)),
			b.Subject)

		// Cut rows off at the terminal edge instead of letting them wrap.
		if m.width > 0 {
			row = ansi.Truncate(row, m.width, "…")
		}
		s.WriteString(strings.TrimRight(row, " ") + "\n")
	}

	if rest := len(visible) - end; rest > 0 {
		s.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return s.String()
}

// renderTags returns the colored status labels for a branch, e.g. "merged gone".
func (m model) renderTags(b Branch) string {
	var tags []string
	if b.Current {
		tags = append(tags, mutedStyle.Render("current"))
	}
	if b.Name == m.base {
		tags = append(tags, mutedStyle.Render("base"))
	}
	if b.InOtherWorktree() {
		tags = append(tags, mutedStyle.Render("worktree"))
	}
	if b.InProgress != "" {
		tags = append(tags, mutedStyle.Render(b.InProgress))
	}
	// Shown even on protected branches: "worktree merged" says the branch can
	// go once that worktree does. The base is always merged into itself.
	if b.Merged && b.Name != m.base {
		tags = append(tags, mergedStyle.Render("merged"))
	}
	if b.Gone {
		tags = append(tags, goneStyle.Render("gone"))
	}
	return strings.Join(tags, " ")
}

// padRight pads s with spaces to width cells. Unlike fmt's %-*s, it measures
// what's visible on screen, so it works on styled strings and wide characters.
func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

func (m model) renderConfirm() string {
	heading, keyHint := "Delete these branches?", "y to delete • n/esc to cancel"
	if m.dryRun {
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
	if m.dryRun {
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
		list = fitLines(list, max(room, 1))
	}

	body := heading + "\n\n" + strings.Join(list, "\n") + "\n" + strings.Join(footer, "\n")
	return confirmBox.Render(body)
}

// fitLines returns lines unchanged if there are at most n, otherwise the first
// n-1 followed by a summary of the rest.
func fitLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	// Clone so append can't write into the caller's slice.
	shown := slices.Clone(lines[:n-1])
	return append(shown, mutedStyle.Render(fmt.Sprintf("  …and %d more", len(lines)-len(shown))))
}
