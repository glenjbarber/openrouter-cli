package tools

import (
	"strings"
	"testing"
)

// TestGitWorktreeNamesTheWildcardBeforeTheOption checks that a command carrying
// both a wildcard and an option is reported as the pattern it is.
//
// The two are checked in separate passes rather than argument by argument,
// since the option is the earlier of the two and is reached first. Reporting
// the force for `worktree remove --force *` reads as though dropping it made
// the command safe, when the pattern takes every checkout in the repository
// whether or not the force mattered.
func TestGitWorktreeNamesTheWildcardBeforeTheOption(t *testing.T) {
	for _, args := range [][]string{
		{"remove", "--force", "*"},
		{"remove", "-f", "*"},
		{"remove", "*", "--force"},
		{"add", "--force", "build/*"},
	} {
		err := worktreeSafe(args, "/repo")
		if err == nil {
			t.Errorf("git worktree %v was permitted", args)
			continue
		}
		if !strings.Contains(err.Error(), "wildcard") {
			t.Errorf("git worktree %v was refused as %v, want it named the wildcard",
				args, err)
		}
	}
}

// TestGitWorktreeStillRefusesTheForceAlone checks that the option is still
// refused on its own, since the wildcard pass runs first and an option carries
// no glob character of its own to be caught by it.
func TestGitWorktreeStillRefusesTheForceAlone(t *testing.T) {
	for _, arg := range []string{"--force", "-f"} {
		err := worktreeSafe([]string{"remove", arg}, "/repo")
		if err == nil {
			t.Errorf("git worktree remove %s was permitted", arg)
			continue
		}
		if !strings.Contains(err.Error(), "discards local changes") {
			t.Errorf("git worktree remove %s was refused as %v, want it named the force",
				arg, err)
		}
	}
}
