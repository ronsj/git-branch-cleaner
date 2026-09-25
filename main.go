package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "show what would be deleted without deleting anything")
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

	final, err := tea.NewProgram(newModel(*dryRun)).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// The alt screen is cleared on exit, so print what was deleted
	// (with restore commands) to the normal terminal scrollback.
	m, ok := final.(model)
	if !ok {
		return
	}
	if *dryRun && len(m.history) > 0 {
		fmt.Println("Dry run: no branches were deleted.")
	}
	for _, r := range m.history {
		if r.Err != nil {
			continue
		}
		fmt.Println(r.Output)
		if sha := restoreSHA(r.Output); sha != "" {
			fmt.Printf("  restore: git branch %s %s\n", r.Name, sha)
		}
	}
}
