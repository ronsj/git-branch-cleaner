package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type state int

const (
	stateLoading state = iota
	stateBrowsing
	stateConfirming
	stateDeleting
)

// Messages: results of async work (tea.Cmd) are delivered back to Update as these.
type (
	branchesLoadedMsg struct {
		base     string
		branches []Branch
	}
	branchesDeletedMsg struct{ results []deleteResult }
	errMsg             struct{ err error }
)

type model struct {
	state    state
	base     string
	branches []Branch
	selected map[string]bool
	cursor   int
	offset   int // index of the first visible row when the list scrolls
	height   int

	spinner spinner.Model
	help    help.Model

	lastResults []deleteResult // shown under the list after a delete
	deleted     []deleteResult // everything deleted this session, printed on exit
	err         error
}

func newModel() model {
	return model{
		state:    stateLoading,
		selected: make(map[string]bool),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(selectedStyle)),
		help:     help.New(),
	}
}

// Commands run off the UI loop; whatever they return is sent to Update.

func loadBranchesCmd() tea.Msg {
	base, branches, err := loadBranches()
	if err != nil {
		return errMsg{err}
	}
	return branchesLoadedMsg{base, branches}
}

func deleteBranchesCmd(names []string) tea.Cmd {
	return func() tea.Msg {
		return branchesDeletedMsg{deleteBranches(names)}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadBranchesCmd, m.spinner.Tick, tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		m.scrollToCursor()
		return m, nil

	case tea.BackgroundColorMsg:
		m.help.Styles = help.DefaultStyles(msg.IsDark())
		return m, nil

	case spinner.TickMsg:
		if !m.busy() {
			return m, nil // returning no command lets the tick loop stop while idle
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case branchesLoadedMsg:
		var cursorName string
		if m.cursor < len(m.branches) {
			cursorName = m.branches[m.cursor].Name
		}
		m.state = stateBrowsing
		m.base = msg.base
		m.branches = msg.branches
		m.err = nil
		// Drop selections for branches that no longer exist.
		for name := range m.selected {
			if m.indexOf(name) < 0 {
				delete(m.selected, name)
			}
		}
		// Keep the cursor on the same branch if it still exists.
		if i := m.indexOf(cursorName); i >= 0 {
			m.cursor = i
		} else {
			m.cursor = min(m.cursor, max(len(m.branches)-1, 0))
		}
		m.scrollToCursor()
		return m, nil

	case branchesDeletedMsg:
		m.lastResults = msg.results
		m.deleted = append(m.deleted, msg.results...)
		m.state = stateLoading
		return m, loadBranchesCmd

	case errMsg:
		m.err = msg.err
		m.state = stateBrowsing
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.state {
		case stateBrowsing:
			return m.updateBrowsing(msg)
		case stateConfirming:
			return m.updateConfirming(msg)
		}
	}
	return m, nil
}

func (m model) updateBrowsing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, keys.Down):
		if m.cursor < len(m.branches)-1 {
			m.cursor++
		}

	case key.Matches(msg, keys.Toggle):
		if m.cursor < len(m.branches) {
			b := m.branches[m.cursor]
			if !b.Protected(m.base) {
				m.selected[b.Name] = !m.selected[b.Name]
			}
		}
	case key.Matches(msg, keys.SelectStale):
		for _, b := range m.branches {
			if (b.Merged || b.Gone) && !b.Protected(m.base) {
				m.selected[b.Name] = true
			}
		}
	case key.Matches(msg, keys.SelectNone):
		clear(m.selected)

	case key.Matches(msg, keys.Delete):
		if len(m.selectedNames()) > 0 {
			m.state = stateConfirming
		}
	case key.Matches(msg, keys.Refresh):
		m.state = stateLoading
		m.lastResults = nil
		return m, tea.Batch(loadBranchesCmd, m.spinner.Tick)
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	}

	m.scrollToCursor()
	return m, nil
}

func (m model) updateConfirming(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Confirm):
		names := m.selectedNames()
		clear(m.selected)
		m.state = stateDeleting
		return m, tea.Batch(deleteBranchesCmd(names), m.spinner.Tick)
	case key.Matches(msg, keys.Cancel):
		m.state = stateBrowsing
	}
	return m, nil
}

func (m model) busy() bool {
	return m.state == stateLoading || m.state == stateDeleting
}

// selectedNames returns selected branches in list order (maps are unordered in Go).
func (m model) selectedNames() []string {
	var names []string
	for _, b := range m.branches {
		if m.selected[b.Name] {
			names = append(names, b.Name)
		}
	}
	return names
}

func (m model) indexOf(name string) int {
	for i, b := range m.branches {
		if b.Name == name {
			return i
		}
	}
	return -1
}

// listHeight is how many branch rows fit on screen; the rest is header,
// footer, and help.
func (m model) listHeight() int {
	if m.height == 0 {
		return len(m.branches) // size unknown yet: show everything
	}
	chrome := 8 + len(m.lastResults)
	if m.help.ShowAll {
		chrome += 3
	}
	return max(m.height-chrome, 3)
}

// scrollToCursor adjusts offset so the cursor row stays visible.
// It has a pointer receiver because it mutates the model in place.
func (m *model) scrollToCursor() {
	rows := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, len(m.branches)-rows))
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "branch-cleaner"
	return v
}

func (m model) render() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("Branch Cleaner"))
	if m.base != "" {
		s.WriteString(mutedStyle.Render("  base: " + m.base))
	}
	s.WriteString("\n\n")

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
		s.WriteString(m.spinner.View() + " Deleting branches…")
		return s.String()
	case stateConfirming:
		s.WriteString(m.renderConfirm())
		return s.String()
	}

	if len(m.branches) == 0 {
		s.WriteString(mutedStyle.Render("No local branches found.") + "\n")
	} else {
		s.WriteString(m.renderList())
	}

	s.WriteString("\n")
	for _, r := range m.lastResults {
		if r.Err != nil {
			s.WriteString(errorStyle.Render("✗ "+r.Err.Error()) + "\n")
		} else {
			s.WriteString(mergedStyle.Render("✓ "+r.Output) + "\n")
		}
	}

	if n := len(m.selectedNames()); n > 0 {
		s.WriteString(selectedStyle.Render(fmt.Sprintf("%d selected", n)) + "\n")
	} else {
		s.WriteString("\n")
	}
	s.WriteString(m.help.View(keys))
	return s.String()
}

func (m model) renderList() string {
	nameWidth := 0
	for _, b := range m.branches {
		nameWidth = max(nameWidth, lipgloss.Width(b.Name))
	}

	var s strings.Builder
	end := min(m.offset+m.listHeight(), len(m.branches))
	if m.offset > 0 {
		s.WriteString(mutedStyle.Render(fmt.Sprintf("  ↑ %d more", m.offset)) + "\n")
	}

	for i := m.offset; i < end; i++ {
		b := m.branches[i]

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

		name := fmt.Sprintf("%-*s", nameWidth, b.Name)
		if i == m.cursor {
			name = cursorStyle.Render(name)
		}

		var tags []string
		if b.Current {
			tags = append(tags, mutedStyle.Render("current"))
		}
		if b.Name == m.base {
			tags = append(tags, mutedStyle.Render("base"))
		}
		if b.Merged && !b.Protected(m.base) {
			tags = append(tags, mergedStyle.Render("merged"))
		}
		if b.Gone {
			tags = append(tags, goneStyle.Render("gone"))
		}

		fmt.Fprintf(&s, "%s%s %s  %s  %s\n",
			pointer, check, name,
			mutedStyle.Render(fmt.Sprintf("%-14s", b.LastCommit)),
			strings.Join(tags, " "))
	}

	if rest := len(m.branches) - end; rest > 0 {
		s.WriteString(mutedStyle.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return s.String()
}

func (m model) renderConfirm() string {
	var s strings.Builder
	s.WriteString("Delete these branches?\n\n")

	unmerged := 0
	for _, name := range m.selectedNames() {
		b := m.branches[m.indexOf(name)]
		line := "  " + name
		if !b.Merged {
			unmerged++
			line += warnStyle.Render("  not merged")
		}
		s.WriteString(line + "\n")
	}
	if unmerged > 0 {
		s.WriteString("\n" + warnStyle.Render(fmt.Sprintf(
			"%d branch(es) are not merged into %s. Their commits will only be\nrecoverable via the SHA printed after deletion.", unmerged, m.base)))
		s.WriteString("\n")
	}
	s.WriteString("\n" + mutedStyle.Render("y to delete • n/esc to cancel"))
	return confirmBox.Render(s.String())
}
