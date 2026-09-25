package git

import (
	"os"
	"os/exec"
	"testing"

	"github.com/ronsj/git-branch-cleaner/internal/testrepo"
)

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"feature/login-2", "feature/login-2"},
		{"a;b", "'a;b'"},
		{"it's", `'it'\''s'`},
		{"", "''"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestRestoreCommandsAreSafeToPaste(t *testing.T) {
	testrepo.New(t)
	names := []string{
		"feature/login", "pwn;touch${IFS}PWNED", "$(touch${IFS}PWNED)", "`touch${IFS}PWNED`",
		"a&b|c>PWNED", "it's", `say"hi"`, "-r", "--all", "café",
	}
	for _, name := range names {
		testrepo.Git(t, "update-ref", "refs/heads/"+name, "HEAD")
	}
	tip := testrepo.Git(t, "rev-parse", "HEAD")

	for _, r := range DeleteBranches(branchesNamed(t, names...), "main") {
		if r.Err != nil {
			t.Fatalf("deleting %q: %v", r.Name, r.Err)
		}
		cmd := RestoreCommand(r.Name, r.SHA)
		if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
			t.Errorf("restore command for %q failed: %s\n%s", r.Name, cmd, out)
			continue
		}
		if got, _ := git("rev-parse", "refs/heads/"+r.Name); got != tip {
			t.Errorf("after %s, branch %q = %q, want %q", cmd, r.Name, got, tip)
		}
	}
	if _, err := os.Stat("PWNED"); err == nil {
		t.Error("a restore command ran part of a branch name as a shell command")
	}
}
