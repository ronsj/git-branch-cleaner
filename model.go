package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

// sortOrder is the order branches are listed in; s cycles through them.
type sortOrder int

const (
	sortOldest sortOrder = iota
	sortNewest
	sortName
	numSortOrders // not an order; keeps next() in range
)

// String makes sortOrder a fmt.Stringer, so it prints as text in the header.
func (o sortOrder) String() string {
	switch o {
	case sortNewest:
		return "newest first"
	case sortName:
		return "by name"
	default:
		return "oldest first"
	}
}

func (o sortOrder) next() sortOrder {
	return (o + 1) % numSortOrders
}

// sortBranches returns a sorted copy of branches. Ties (same commit time)
// fall back to the name so the order never shuffles between reloads.
func sortBranches(branches []Branch, order sortOrder) []Branch {
	sorted := slices.Clone(branches)
	slices.SortStableFunc(sorted, func(a, b Branch) int {
		byName := cmp.Compare(a.Name, b.Name)
		switch order {
		case sortNewest:
			return cmp.Or(b.CommitTime.Compare(a.CommitTime), byName)
		case sortName:
			return byName
		default:
			return cmp.Or(a.CommitTime.Compare(b.CommitTime), byName)
		}
	})
	return sorted
}

// options are the settings chosen on the command line.
type options struct {
	dryRun        bool // preview deletions instead of running them
	olderThanDays int  // hide branches with commits newer than this; 0 shows all
}

type model struct {
	options
	sortBy   sortOrder
	state    state
	base     string
	branches []Branch // every local branch; see visibleBranches for the filtered list
	selected map[string]bool
	cursor   int // index into visibleBranches()
	offset   int // index of the first visible row when the list scrolls
	width    int
	height   int

	spinner spinner.Model
	help    help.Model
	filter  textinput.Model // focused while the user is typing a filter

	lastResults []deleteResult // shown under the list after a delete
	history     []deleteResult // every delete (or dry-run preview) this session, printed on exit
	err         error
}

func newModel(opts options) model {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "filter by name"

	return model{
		options:  opts,
		state:    stateLoading,
		selected: make(map[string]bool),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(selectedStyle)),
		help:     help.New(),
		filter:   filter,
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

func deleteBranchesCmd(names []string, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			return branchesDeletedMsg{previewDeletes(names)}
		}
		return branchesDeletedMsg{deleteBranches(names)}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadBranchesCmd, m.spinner.Tick, tea.RequestBackgroundColor)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		m.scrollToCursor()
		return m, nil

	case tea.BackgroundColorMsg:
		m.help.Styles = help.DefaultStyles(msg.IsDark())
		m.filter.SetStyles(textinput.DefaultStyles(msg.IsDark()))
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
		if b, ok := m.cursorBranch(); ok {
			cursorName = b.Name
		}
		m.state = stateBrowsing
		m.base = msg.base
		m.branches = sortBranches(msg.branches, m.sortBy)
		m.err = nil
		// Drop selections for branches that no longer exist or have become
		// protected, e.g. checked out in another worktree since the last load.
		for name := range m.selected {
			if i := indexOf(m.branches, name); i < 0 || m.branches[i].Protected(m.base) {
				delete(m.selected, name)
			}
		}
		m.moveCursorTo(cursorName)
		return m, nil

	case branchesDeletedMsg:
		m.lastResults = msg.results
		m.history = append(m.history, msg.results...)
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
		switch {
		case m.state == stateBrowsing && m.filter.Focused():
			return m.updateFiltering(msg)
		case m.state == stateBrowsing:
			return m.updateBrowsing(msg)
		case m.state == stateConfirming:
			return m.updateConfirming(msg)
		}
		return m, nil
	}

	// Anything else (like the text cursor's blink timer) belongs to the filter input.
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	return m, cmd
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
		if m.cursor < len(m.visibleBranches())-1 {
			m.cursor++
		}

	case key.Matches(msg, keys.Toggle):
		if b, ok := m.cursorBranch(); ok && !b.Protected(m.base) {
			m.selected[b.Name] = !m.selected[b.Name]
		}
	case key.Matches(msg, keys.SelectStale):
		for _, b := range m.visibleBranches() {
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
	case key.Matches(msg, keys.Sort):
		b, _ := m.cursorBranch()
		m.sortBy = m.sortBy.next()
		m.branches = sortBranches(m.branches, m.sortBy)
		m.moveCursorTo(b.Name)
	case key.Matches(msg, keys.Refresh):
		m.state = stateLoading
		m.lastResults = nil
		return m, tea.Batch(loadBranchesCmd, m.spinner.Tick)
	case key.Matches(msg, keys.Help):
		m.help.ShowAll = !m.help.ShowAll

	case key.Matches(msg, keys.Filter):
		return m, m.filter.Focus()
	case key.Matches(msg, keys.ClearFilter):
		m.setFilter("")
	}

	m.scrollToCursor()
	return m, nil
}

// updateFiltering handles keys while the filter input is focused. Letters go
// to the input, so j/k type instead of moving; the arrow keys still move.
func (m model) updateFiltering(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter.Blur()
		return m, nil
	case "esc":
		m.filter.Blur()
		m.setFilter("")
		return m, nil
	case "up", "down":
		return m.updateBrowsing(msg)
	}

	before := m.filter.Value()
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.cursor, m.offset = 0, 0
	}
	return m, cmd
}

// setFilter replaces the filter text and moves the cursor back to the top.
func (m *model) setFilter(value string) {
	m.filter.SetValue(value)
	m.cursor, m.offset = 0, 0
}

func (m model) updateConfirming(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Confirm):
		names := m.selectedNames()
		clear(m.selected)
		m.state = stateDeleting
		return m, tea.Batch(deleteBranchesCmd(names, m.dryRun), m.spinner.Tick)
	case key.Matches(msg, keys.Cancel):
		m.state = stateBrowsing
	}
	return m, nil
}

func (m model) busy() bool {
	return m.state == stateLoading || m.state == stateDeleting
}

// visibleBranches returns the branches shown in the list: those not hidden
// by --older-than whose names match the filter, case-insensitively.
func (m model) visibleBranches() []Branch {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if query == "" && m.olderThanDays == 0 {
		return m.branches
	}
	var visible []Branch
	for _, b := range m.branches {
		if m.tooRecent(b) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(b.Name), query) {
			continue
		}
		visible = append(visible, b)
	}
	return visible
}

// tooRecent reports whether --older-than hides b.
func (m model) tooRecent(b Branch) bool {
	minAge := time.Duration(m.olderThanDays) * 24 * time.Hour
	return m.olderThanDays > 0 && time.Since(b.CommitTime) < minAge
}

// moveCursorTo puts the cursor on the named branch if it's visible, and
// otherwise keeps the cursor in bounds.
func (m *model) moveCursorTo(name string) {
	visible := m.visibleBranches()
	if i := indexOf(visible, name); i >= 0 {
		m.cursor = i
	} else {
		m.cursor = min(m.cursor, max(len(visible)-1, 0))
	}
	m.scrollToCursor()
}

// cursorBranch returns the branch under the cursor, if the list isn't empty.
func (m model) cursorBranch() (Branch, bool) {
	visible := m.visibleBranches()
	if m.cursor < len(visible) {
		return visible[m.cursor], true
	}
	return Branch{}, false
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

func indexOf(branches []Branch, name string) int {
	for i, b := range branches {
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
		return len(m.visibleBranches()) // size unknown yet: show everything
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
	m.offset = max(0, min(m.offset, len(m.visibleBranches())-rows))
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
		if m.dryRun {
			s.WriteString(m.spinner.View() + " Previewing deletions…")
		} else {
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

	s.WriteString("\n")
	for _, r := range m.lastResults {
		if r.Err != nil {
			s.WriteString(errorStyle.Render("✗ "+r.Err.Error()) + "\n")
		} else {
			s.WriteString(mergedStyle.Render("✓ "+r.Output) + "\n")
		}
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
	selected := m.selectedNames()
	if len(selected) == 0 {
		return ""
	}
	visible := m.visibleBranches()
	hidden := 0
	for _, name := range selected {
		if indexOf(visible, name) < 0 {
			hidden++
		}
	}
	text := fmt.Sprintf("%d selected", len(selected))
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
	if b.Merged && !b.Protected(m.base) {
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
	for _, name := range m.selectedNames() {
		b := m.branches[indexOf(m.branches, name)]
		line := "  " + name
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
