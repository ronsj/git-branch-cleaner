package tui

import (
	"cmp"
	"slices"

	"github.com/ronsj/git-branch-cleaner/internal/git"
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
func sortBranches(branches []git.Branch, order sortOrder) []git.Branch {
	sorted := slices.Clone(branches)
	slices.SortStableFunc(sorted, func(a, b git.Branch) int {
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
