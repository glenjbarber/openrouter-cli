package tools

import (
	"fmt"
	"strings"
)

// gitWorktree is the subcommand managing extra checkouts of the repository.
//
// It is spelled out here rather than repeated, since the places that treat it
// apart from the bare-word subcommands all have to name it and a word written
// out four times is a word that falls behind.
const gitWorktree = "worktree"

// worktreeRefused are the options under which git worktree reaches further than
// the checkout it was named with.
//
// `--force` removes a worktree holding local changes, which is the one form
// where a checkout a reader was working in is discarded rather than set aside.
// It is refused by name because the argument is all there is to go on, the same
// way gitRefused refuses an option that runs a program.
var worktreeRefused = map[string]string{
	"--force": "it discards local changes in the worktree rather than refusing",
	"-f":      "it discards local changes in the worktree rather than refusing",
}

// worktreeSafe refuses the arguments under which git worktree would reach
// beyond the one checkout it was named with. repo is the root the tool is
// contained to, named in the diagnostic so a reader is told what the boundary
// is rather than only that something was outside it.
//
// Three things are refused, and the reason each is worth a name of its own:
//
//   - A wildcard. The arguments go to git as an array and are never read by a
//     shell, so `worktree remove *` reaches git as a literal asterisk and git
//     treats it as a pattern matching every checkout it has. It would take each
//     one, and a worktree that is removed is not restored by the next command.
//   - A path leaving the repository. The tool is contained to the repository at
//     the working directory, and `git worktree remove ../somewhere` or an
//     absolute path would step outside that containment through a subcommand
//     whose whole purpose is to name a directory.
//   - The options above, which discard rather than refuse.
//
// The wildcard is checked before the options, since an argument can carry both
// and the pattern is the more useful of the two reasons to report. `worktree
// remove --force *` would take every checkout in the repository whether or not
// the force mattered, so the pattern is what the reader needs told.
//
// The reading forms pass untouched. `list` reads, and `add`, `move`, `lock`,
// `prune`, and `remove` are the workflow the set was widened for, so the cost
// of refusing a wildcard is one retry rather than the checkout it protected.
func worktreeSafe(args []string, repo string) error {
	for _, arg := range args {
		if arg == "" {
			continue
		}
		// A glob character is refused wherever it appears rather than only at
		// the head of an argument, since a path such as build/*/x names every
		// worktree beneath it, and an argument that looks like an option can be
		// named with a separator in front of it.
		if strings.ContainsAny(arg, "*?[]") {
			return fmt.Errorf("git %s will not take %q: a wildcard reaches every "+
				"checkout it matches, and a worktree that is removed is not put "+
				"back by the next command; name the one path meant",
				gitWorktree, arg)
		}
		if reason := worktreeRefused[arg]; reason != "" {
			return fmt.Errorf("git %s will not take %s: %s", gitWorktree, arg, reason)
		}
		// An option is not a path, so it is not checked for leaving. The
		// options that would redirect git are refused by gitRefused, and this
		// subcommand takes none of its own that name a directory.
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if escapes(arg) {
			return fmt.Errorf("git %s will not take %q: it is outside the "+
				"repository, and the tool is contained to %s",
				gitWorktree, arg, repo)
		}
	}
	return nil
}