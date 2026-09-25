package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/ronsj/git-branch-cleaner/internal/git"
)

// maxAuthorWidth caps the author column so one long name can't crowd out subjects.
const maxAuthorWidth = 20

// columnWidths are the widths of the columns whose contents only change when
// the branches are loaded. Each column is sized to its widest value across
// every branch, so the columns line up and don't shift as the filter changes.
type columnWidths struct {
	name, tags, author int
}

// measureColumns measures every branch for columnWidths. That's the slowest
// part of drawing a long list, so it's done once per load, not per draw.
func (m Model) measureColumns() columnWidths {
	var w columnWidths
	for _, b := range m.branches {
		w.name = max(w.name, lipgloss.Width(b.Name))
		w.tags = max(w.tags, lipgloss.Width(m.renderTags(b)))
		w.author = max(w.author, lipgloss.Width(b.Author))
	}
	w.author = min(w.author, maxAuthorWidth)
	return w
}

// renderList draws rows of visible, starting at m.offset.
func (m Model) renderList(visible []git.Branch, rows int) string {
	// Ages are worked out on every render, so they stay current while the
	// app is open. They're plain ASCII, so len is their width on screen.
	now := time.Now()
	dateWidth := 0
	for _, b := range m.branches {
		dateWidth = max(dateWidth, len(relativeTime(b.CommitTime, now)))
	}
	w := m.widths

	var s strings.Builder
	end := min(m.offset+rows, len(visible))
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

		name := padRight(b.Name, w.name)
		if i == m.cursor {
			name = cursorStyle.Render(name)
		}

		author := ansi.Truncate(b.Author, w.author, "…")

		row := fmt.Sprintf("%s%s %s  %s  %s  %s  %s",
			pointer, check, name,
			mutedStyle.Render(padRight(relativeTime(b.CommitTime, now), dateWidth)),
			padRight(m.renderTags(b), w.tags),
			mutedStyle.Render(padRight(author, w.author)),
			b.Subject)

		// Cut rows off at the terminal edge instead of letting them wrap.
		row = m.fitWidth(row)
		s.WriteString(strings.TrimRight(row, " ") + "\n")
	}

	if rest := len(visible) - end; rest > 0 {
		s.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return s.String()
}

// renderTags returns the colored status labels for a branch, e.g. "merged gone".
func (m Model) renderTags(b git.Branch) string {
	var tags []string
	if b.Current {
		tags = append(tags, mutedStyle.Render("current"))
	}
	if b.IsBase(m.base) {
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
	if b.Merged && !b.IsBase(m.base) {
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
