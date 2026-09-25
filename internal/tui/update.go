package tui

import (
	"maps"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		selectedAt := make(map[string]string) // name -> commit when selected
		for _, b := range m.selectedBranches() {
			selectedAt[b.Name] = b.SHA
		}
		m.state = stateBrowsing
		m.base = msg.base
		m.branches = sortBranches(msg.branches, m.sortBy)
		m.err = nil
		// Keep only selections that still exist at the same commit and are
		// still allowed: not protected (e.g. checked out in another worktree
		// since the last load) and not hidden by --older-than. A branch with
		// new commits is deselected, so it can't be deleted along with
		// commits the user never saw.
		stillSelected := make(map[string]bool)
		for _, b := range m.branches {
			if sha, ok := selectedAt[b.Name]; ok && sha == b.SHA && !b.Protected(m.base) && !m.tooRecent(b) {
				stillSelected[b.Name] = true
			}
		}
		m.selected = stillSelected
		m.widths = m.measureColumns()
		m.moveCursorTo(cursorName)
		return m, nil

	case branchesDeletedMsg:
		m.lastResults = msg.results
		m.history = append(m.history, msg.results...)
		if m.quitting {
			return m, tea.Quit
		}
		m.state = stateLoading
		return m, m.loadBranchesCmd()

	case errMsg:
		m.err = msg.err
		m.state = stateBrowsing
		return m, nil

	case tea.KeyPressMsg:
		// q quits too while loading or deleting: nothing else takes keys
		// then, and the filter (where q is just a letter) can't be open.
		if msg.String() == "ctrl+c" || m.busy() && key.Matches(msg, keys.Quit) {
			// Quitting mid-delete would lose the results, and with them the
			// restore commands printed on exit, so wait for them.
			if m.state == stateDeleting {
				m.quitting = true
				return m, nil
			}
			return m, tea.Quit
		}
		switch {
		case m.state == stateBrowsing && m.err != nil:
			return m.updateError(msg)
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

func (m Model) updateBrowsing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			m.selected = maps.Clone(m.selected)
			if m.selected[b.Name] {
				delete(m.selected, b.Name)
			} else {
				m.selected[b.Name] = true
			}
		}
	case key.Matches(msg, keys.SelectStale):
		m.selected = maps.Clone(m.selected)
		for _, b := range m.visibleBranches() {
			if (b.Merged || b.Gone) && !b.Protected(m.base) {
				m.selected[b.Name] = true
			}
		}
	case key.Matches(msg, keys.SelectNone):
		m.selected = make(map[string]bool)

	case key.Matches(msg, keys.Delete):
		if len(m.selected) > 0 {
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
		return m, tea.Batch(m.loadBranchesCmd(), m.spinner.Tick)
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

// updateError handles keys while an error is shown. The error replaces the
// list, so only retry and quit work: any other key would act on branches the
// user can't see, and enter then y would delete them with no confirm screen.
func (m Model) updateError(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, keys.Refresh, keys.Quit) {
		return m.updateBrowsing(msg)
	}
	return m, nil
}

// updateFiltering handles keys while the filter input is focused. Letters go
// to the input, so j/k type instead of moving; the arrow keys still move.
func (m Model) updateFiltering(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
func (m *Model) setFilter(value string) {
	m.filter.SetValue(value)
	m.cursor, m.offset = 0, 0
}

func (m Model) updateConfirming(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Confirm):
		selected := m.selectedBranches()
		m.selected = make(map[string]bool)
		m.state = stateDeleting
		return m, tea.Batch(deleteBranchesCmd(selected, m.base, m.DryRun), m.spinner.Tick)
	case key.Matches(msg, keys.Cancel):
		m.state = stateBrowsing
	}
	return m, nil
}
