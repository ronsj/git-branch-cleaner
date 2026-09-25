// Package testrepo creates throwaway git repositories for tests.
package testrepo

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// New creates a git repo with one commit on main plus the given branches, and
// makes it the working directory for the rest of the test. The user's own git
// config is ignored so tests behave the same everywhere.
func New(t *testing.T, branches ...string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@example.com")
	}
	t.Chdir(t.TempDir())

	Git(t, "init", "-q", "-b", "main")
	Git(t, "commit", "-q", "--allow-empty", "-m", "initial")
	for _, b := range branches {
		Git(t, "branch", b)
	}
}

// Git runs git in the current directory and returns its stdout without the
// trailing newline, failing the test if git fails.
func Git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimRight(string(out), "\r\n")
}

// CommitFile changes a file in dir and commits it, giving rebase and bisect
// real commits to work through.
func CommitFile(t *testing.T, dir, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	Git(t, "-C", dir, "add", "file.txt")
	Git(t, "-C", dir, "commit", "-q", "-m", message)
}

// BranchExists reports whether a local branch exists. It checks with git
// directly, independently of the code under test.
func BranchExists(name string) bool {
	return exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run() == nil
}
