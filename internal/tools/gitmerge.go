package tools

import (
	"fmt"
	"strings"
)

// gitMerge is the subcommand joining two histories.
//
// It is spelled out here rather than repeated, on the same reasoning as
// gitWorktree: the places that treat it apart from a bare-word subcommand all
// have to name it.
const gitMerge = "merge"

// mergeRefused are the options under which git merge answers a conflict rather
// than reporting one.
//
// -X takes a strategy option, and `-X ours` resolves every disagreement in the
// merge by keeping this side. The merge then completes, exits zero, and records
// a commit. Nothing in the output says a conflict existed, and the work that
// was dropped surfaces later as a fault in whatever depended on it.
// `-X theirs` does the same for the other branch, and the ignore-space forms
// hide a real change as though it were formatting.
//
// The option is refused whole rather than by value. A reader who wants a side
// taken has a decision to make, and a model picking the side is not the reader
// picking it. The cost of refusing is one retry, since an ordinary merge
// reports the conflict and the reader resolves it.
var mergeRefused = map[string]string{
	"-X":                "it answers a conflict instead of reporting one",
	"--strategy-option": "it passes an option to the merge strategy",
	"--strategy":        "it names the merge strategy git runs",
	"-s":                "it names the merge strategy git runs",
}

// mergeSafe refuses the arguments under which git merge would reach further
// than joining two histories that agree.
//
// What is left is the option set that stops short of recording or of choosing:
// --squash and --no-commit leave the result in the working tree for a reader
// to look at, and --no-ff, --ff-only and --no-edit shape how the commit is
// made without deciding its contents. --abort and --continue are not here:
// they recover a merge that stopped on a conflict, and a reader recovering one
// by hand is the ordinary way.
func mergeSafe(args []string) error {
	for _, arg := range args {
		if arg == "" {
			continue
		}
		if reason := mergeRefused[arg]; reason != "" {
			return fmt.Errorf("git %s will not take %s: %s", gitMerge, arg, reason)
		}
		// The option written as one argument is refused by the same name as one
		// written as two, since the refusal is by name and would otherwise miss
		// the form git documents.
		name, _, found := strings.Cut(arg, "=")
		if reason := mergeRefused[name]; found && reason != "" {
			return fmt.Errorf("git %s will not take %s: %s", gitMerge, name, reason)
		}
	}
	return nil
}
