// Package cmd implements the git-branch-cleaner command line: flags, running
// the UI, and the summary printed on exit.
package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
	"github.com/ronsj/git-branch-cleaner/internal/tui"
)

// Execute parses the command line, runs the UI, and prints what was deleted.
// Like a main function, it exits the process itself on errors.
func Execute() {
	dryRun := flag.Bool("dry-run", false, "show what would be deleted without deleting anything")
	olderThan := flag.Int("older-than", 0, "hide branches whose last commit is less than `N` days old")
	base := flag.String("base", "", "compare against `branch` instead of detecting it\n(default: origin's default branch, then main, then master)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: git-branch-cleaner [flags]\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Find and delete stale local git branches. Run it inside a git repository.\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument: %s\n\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	if *olderThan < 0 {
		fmt.Fprintf(os.Stderr, "-older-than must be 0 or more days, got %d\n", *olderThan)
		os.Exit(2)
	}

	opts := tui.Options{DryRun: *dryRun, OlderThanDays: *olderThan, BaseOverride: *base}
	final, err := tea.NewProgram(tui.New(opts)).Run()
	// Run returns the last model even when it fails, and branches may already
	// have been deleted, so print the history before reporting the error.
	if m, ok := final.(tui.Model); ok {
		printHistory(os.Stdout, m.History(), m.DryRun)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// printHistory prints what was deleted, with restore commands, and what
// wasn't and why. The alt screen is cleared on exit, so this is the copy that
// stays in the terminal.
func printHistory(w io.Writer, history []git.DeleteResult, dryRun bool) {
	if dryRun && len(history) > 0 {
		fmt.Fprintln(w, "Dry run: no branches were deleted.")
	}
	for _, r := range history {
		fmt.Fprintln(w, r)
		if r.Err == nil && !r.DryRun {
			fmt.Fprintln(w, "  restore:", git.RestoreCommand(r.Name, r.SHA))
		}
	}
}
