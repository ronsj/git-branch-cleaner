package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

// listHeight is how many branch rows fit on screen: the terminal height
// minus the header, the footer (measured, not assumed), and the two
// "↑/↓ N more" lines.
func (m Model) listHeight() int {
	visible := m.visibleBranches()
	return m.rowsFor(len(visible), m.renderFooter(visible))
}

// rowsFor is listHeight given the number of visible branches and the
// footer, for render, which has both already.
func (m Model) rowsFor(visible int, footer string) int {
	if m.height == 0 {
		return visible // size unknown yet: show everything
	}
	const headerLines, scrollMarkers = 2, 2
	return max(m.height-headerLines-scrollMarkers-lipgloss.Height(footer), 3)
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "git-branch-cleaner"
	return v
}

func (m Model) render() string {
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
		case m.DryRun:
			s.WriteString(m.spinner.View() + " Previewing deletions…")
		default:
			s.WriteString(m.spinner.View() + " Deleting branches…")
		}
		return s.String()
	case stateConfirming:
		s.WriteString(m.renderConfirm())
		return s.String()
	}

	// Worked out once here and passed down: each takes a pass over every
	// branch, and the list's height depends on the footer's.
	visible := m.visibleBranches()
	footer := m.renderFooter(visible)
	switch {
	case len(m.branches) == 0:
		s.WriteString(mutedStyle.Render("No local branches found.") + "\n")
	case len(visible) == 0 && m.filter.Value() != "":
		s.WriteString(mutedStyle.Render(fmt.Sprintf("No branches match %q.", m.filter.Value())) + "\n")
	case len(visible) == 0:
		s.WriteString(mutedStyle.Render(fmt.Sprintf("No branches are older than %s.", days(m.OlderThanDays))) + "\n")
	default:
		s.WriteString(m.renderList(visible, m.rowsFor(len(visible), footer)))
	}

	s.WriteString(footer)
	return s.String()
}

// renderFooter is everything below the list: the last delete's results, the
// selection count, and the key help.
func (m Model) renderFooter(visible []git.Branch) string {
	var s strings.Builder
	s.WriteString("\n")
	for _, line := range m.resultLines() {
		s.WriteString(line + "\n")
	}
	if m.notice != "" {
		s.WriteString(m.fitWidth(errorStyle.Render(m.notice)) + "\n")
	}
	s.WriteString(m.renderSelectionCount(visible) + "\n")
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
func (m Model) resultLines() []string {
	var failed, deleted []string
	for _, r := range m.lastResults {
		if r.Err != nil {
			failed = append(failed, errorStyle.Render("✗ "+r.String()))
		} else {
			deleted = append(deleted, mergedStyle.Render("✓ "+r.String()))
		}
	}
	lines := append(failed, deleted...)
	for i, line := range lines {
		lines[i] = m.fitWidth(line)
	}
	if m.height == 0 {
		return lines
	}
	return fitLines(lines, max(m.height/4, 3), " (all listed when you quit)")
}

// renderHeader is the title line: app name, dry-run badge, base branch,
// sort order, and how many branches --older-than is hiding.
func (m Model) renderHeader() string {
	header := titleStyle.Render("Git Branch Cleaner")
	if m.DryRun {
		header += dryRunStyle.Render("  DRY RUN")
	}

	var info []string
	if m.base != "" {
		info = append(info, "base: "+m.base)
	}
	info = append(info, m.sortBy.String())
	if m.OlderThanDays > 0 {
		hidden := 0
		for _, b := range m.branches {
			if m.tooRecent(b) {
				hidden++
			}
		}
		info = append(info, fmt.Sprintf("%d newer than %s hidden", hidden, days(m.OlderThanDays)))
	}
	header += mutedStyle.Render("  " + strings.Join(info, " · "))

	return m.fitWidth(header)
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
func (m Model) renderSelectionCount(visible []git.Branch) string {
	selected := len(m.selected)
	if selected == 0 {
		return ""
	}
	shown := 0
	for _, b := range visible {
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

// fitLines returns lines unchanged if there are at most n, otherwise the first
// n-1 followed by a line saying how many more there are, plus note.
func fitLines(lines []string, n int, note string) []string {
	if len(lines) <= n {
		return lines
	}
	// Clone so append can't write into the caller's slice.
	shown := slices.Clone(lines[:n-1])
	return append(shown, mutedStyle.Render(fmt.Sprintf("  …and %d more%s", len(lines)-len(shown), note)))
}

// fitWidth cuts s off at the terminal edge, once the width is known.
func (m Model) fitWidth(s string) string {
	if m.width <= 0 {
		return s
	}
	return ansi.Truncate(s, m.width, "…")
}
