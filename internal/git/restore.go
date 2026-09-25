package git

import (
	"fmt"
	"strings"
)

// RestoreCommand returns a shell command that recreates a deleted branch.
// The name is quoted because branch names may contain characters like ; $ `
// or ( that a shell would act on when the command is pasted.
func RestoreCommand(name, sha string) string {
	// git branch rejects names starting with "-", so recreate those directly.
	if strings.HasPrefix(name, "-") {
		return fmt.Sprintf("git update-ref %s %s", shellQuote("refs/heads/"+name), sha)
	}
	return fmt.Sprintf("git branch %s %s", shellQuote(name), sha)
}

// shellQuote returns s ready to paste into a POSIX shell: unchanged if every
// character is plainly safe, otherwise wrapped in single quotes, inside which
// nothing is special. A single quote within s is written by closing the
// quoted string, adding an escaped quote (\'), and reopening it.
func shellQuote(s string) string {
	unsafe := func(r rune) bool {
		return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' ||
			strings.ContainsRune("-_./+=,@%", r))
	}
	if s != "" && strings.IndexFunc(s, unsafe) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
