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

func (m Model) renderList() string {
	// Ages are worked out on every render, so they stay current while the
	// app is open.
	now := time.Now()

	// Size each column to its widest value so the columns line up.
	var nameWidth, dateWidth, tagWidth, authorWidth int
	for _, b := range m.branches {
		nameWidth = max(nameWidth, lipgloss.Width(b.Name))
		dateWidth = max(dateWidth, lipgloss.Width(relativeTime(b.CommitTime, now)))
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
			mutedStyle.Render(padRight(relativeTime(b.CommitTime, now), dateWidth)),
			padRight(m.renderTags(b), tagWidth),
			mutedStyle.Render(padRight(author, authorWidth)),
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
