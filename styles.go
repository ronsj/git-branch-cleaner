package main

import "charm.land/lipgloss/v2"

// ANSI palette indexes (0-15) are remapped by the user's terminal theme,
// so these read well on both light and dark backgrounds.
var (
	accent = lipgloss.Color("5") // magenta
	green  = lipgloss.Color("2")
	yellow = lipgloss.Color("3")
	red    = lipgloss.Color("1")
	muted  = lipgloss.Color("8") // bright black / gray

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedStyle    = lipgloss.NewStyle().Foreground(muted)
	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	selectedStyle = lipgloss.NewStyle().Foreground(accent)
	mergedStyle   = lipgloss.NewStyle().Foreground(green)
	goneStyle     = lipgloss.NewStyle().Foreground(yellow)
	errorStyle    = lipgloss.NewStyle().Foreground(red)
	warnStyle     = lipgloss.NewStyle().Bold(true).Foreground(red)
	dryRunStyle   = lipgloss.NewStyle().Bold(true).Foreground(yellow)

	confirmBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2)
)
