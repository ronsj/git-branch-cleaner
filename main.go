package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

func main() {
	final, err := tea.NewProgram(newModel()).Run()
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
	for _, r := range m.deleted {
		if r.Err != nil {
			continue
		}
		fmt.Println(r.Output)
		if sha := restoreSHA(r.Output); sha != "" {
			fmt.Printf("  restore: git branch %s %s\n", r.Name, sha)
		}
	}
}
