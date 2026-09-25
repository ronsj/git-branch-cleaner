package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
	"github.com/ronsj/git-branch-cleaner/internal/tui"
)

func main() {
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
		printHistory(m)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// printHistory prints what was deleted, with restore commands. The alt screen
// is cleared on exit, so this is the copy that stays in the terminal.
func printHistory(m tui.Model) {
	if m.DryRun && len(m.History()) > 0 {
		fmt.Println("Dry run: no branches were deleted.")
	}
	for _, r := range m.History() {
		if r.Err != nil {
			continue
		}
		fmt.Println(r)
		if !r.DryRun {
			fmt.Println("  restore:", git.RestoreCommand(r.Name, r.SHA))
		}
	}
}
