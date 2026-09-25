package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	Toggle      key.Binding
	SelectStale key.Binding
	SelectNone  key.Binding
	Delete      key.Binding
	Refresh     key.Binding
	Sort        key.Binding
	Filter      key.Binding
	ClearFilter key.Binding
	Help        key.Binding
	Quit        key.Binding
	Confirm     key.Binding
	Cancel      key.Binding
}

var keys = keyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Toggle:      key.NewBinding(key.WithKeys("space", "x"), key.WithHelp("space", "toggle")),
	SelectStale: key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "select merged/gone")),
	SelectNone:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "select none")),
	Delete:      key.NewBinding(key.WithKeys("enter", "d"), key.WithHelp("enter", "delete")),
	Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Sort:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "change sort")),
	Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	ClearFilter: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Confirm:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes, delete")),
	Cancel:      key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
}

// ShortHelp and FullHelp satisfy help.KeyMap, so the help bubble can render them.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Toggle, k.SelectStale, k.Delete, k.Filter, k.ClearFilter, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Toggle},
		{k.SelectStale, k.SelectNone, k.Delete},
		{k.Filter, k.ClearFilter, k.Sort},
		{k.Refresh, k.Help, k.Quit},
	}
}
