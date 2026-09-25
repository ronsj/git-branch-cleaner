package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "show what would be deleted without deleting anything")
	olderThan := flag.Int("older-than", 0, "hide branches whose last commit is less than `N` days old")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: branch-cleaner [flags]\n\n")
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

	opts := options{dryRun: *dryRun, olderThanDays: *olderThan}
	final, err := tea.NewProgram(newModel(opts)).Run()
	// Run returns the last model even when it fails, and branches may already
	// have been deleted, so print the history before reporting the error.
	if m, ok := final.(model); ok {
		printHistory(m)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// printHistory prints what was deleted, with restore commands. The alt screen
// is cleared on exit, so this is the copy that stays in the terminal.
func printHistory(m model) {
	if m.dryRun && len(m.history) > 0 {
		fmt.Println("Dry run: no branches were deleted.")
	}
	for _, r := range m.history {
		if r.Err != nil {
			continue
		}
		fmt.Println(r)
		if !r.DryRun {
			fmt.Printf("  restore: git branch %s %s\n", r.Name, r.SHA)
		}
	}
}
