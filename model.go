package main

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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

// options are the settings chosen on the command line.
type options struct {
	dryRun        bool   // preview deletions instead of running them
	olderThanDays int    // hide branches with commits newer than this; 0 shows all
	baseOverride  string // compare against this branch; "" detects it
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
	quitting    bool           // ctrl+c came in mid-delete; quit once the results are in
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

func (m model) loadBranchesCmd() tea.Cmd {
	baseOverride := m.baseOverride // captured: the command runs in the background
	return func() tea.Msg {
		base, branches, err := loadBranches(baseOverride)
		if err != nil {
			return errMsg{err}
		}
		return branchesLoadedMsg{base, branches}
	}
}

func deleteBranchesCmd(branches []Branch, base string, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			return branchesDeletedMsg{previewDeletes(branches)}
		}
		return branchesDeletedMsg{deleteBranches(branches, base)}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadBranchesCmd(), m.spinner.Tick, tea.RequestBackgroundColor)
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

// selectedBranches returns selected branches in list order (maps are
// unordered in Go).
func (m model) selectedBranches() []Branch {
	var selected []Branch
	for _, b := range m.branches {
		if m.selected[b.Name] {
			selected = append(selected, b)
		}
	}
	return selected
}

func (m model) selectedNames() []string {
	var names []string
	for _, b := range m.selectedBranches() {
		names = append(names, b.Name)
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
