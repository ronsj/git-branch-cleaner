// Package cmd implements the git-branch-cleaner command line: flags, running
// the UI, and the summary printed on exit.
package cmd

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/ronsj/git-branch-cleaner/internal/git"
	"github.com/ronsj/git-branch-cleaner/internal/tui"
)

// Execute runs the command line and exits with its status.
func Execute() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses args, runs the UI, and prints the summary of what was deleted to
// stdout, with usage and errors going to stderr. It returns the exit status:
// 0 on success, 1 on error, 2 for a usage error. It uses its own flag set, so
// it can run more than once (in tests, say).
func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("git-branch-cleaner", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dryRun := flags.Bool("dry-run", false, "show what would be deleted without deleting anything")
	olderThan := flags.Int("older-than", 0, "hide branches whose last commit is less than `N` days old")
	base := flags.String("base", "", "compare against `branch` instead of detecting it\n(default: origin's default branch, then main, then master)")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "Usage: git-branch-cleaner [flags]\n\n")
		fmt.Fprintf(flags.Output(), "Find and delete stale local git branches. Run it inside a git repository.\n\nFlags:\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		// The flag package has already printed the problem and the usage.
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n\n", flags.Arg(0))
		flags.Usage()
		return 2
	}
	if *olderThan < 0 {
		fmt.Fprintf(stderr, "-older-than must be 0 or more days, got %d\n", *olderThan)
		return 2
	}

	opts := tui.Options{DryRun: *dryRun, OlderThanDays: *olderThan, BaseOverride: *base}
	final, err := tea.NewProgram(tui.New(opts)).Run()
	// Run returns the last model even when it fails, and branches may already
	// have been deleted, so print the history before reporting the error.
	if m, ok := final.(tui.Model); ok {
		printHistory(stdout, m.History(), m.DryRun)
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
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
