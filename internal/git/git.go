// Package git reads, protects, and deletes local branches by running the git
// command in the current directory.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs a git subcommand in the current directory and returns its stdout.
// Stdout is returned even if git fails, since git may have done part of the
// work (deleted some of the branches, say).
func git(args ...string) (string, error) {
	return run("", nil, nil, args...)
}

// run is git with input on stdin, extra environment variables, and -c config
// settings.
func run(stdin string, env, config []string, args ...string) (string, error) {
	var options []string
	for _, c := range config {
		options = append(options, "-c", c)
	}
	cmd := exec.Command("git", append(options, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	// Only strip the trailing newline: fields inside the output can
	// legitimately end in spaces.
	stdout := strings.TrimRight(string(out), "\r\n")
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout, fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout, nil
}
