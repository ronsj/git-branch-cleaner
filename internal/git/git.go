// Package git reads, protects, and deletes local branches by running the git
// command in the current directory.
package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// git runs a git subcommand in the current directory and returns its stdout.
func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	// Only strip the trailing newline: fields inside the output can
	// legitimately end in spaces.
	return strings.TrimRight(string(out), "\r\n"), nil
}
