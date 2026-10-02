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

// mergeRefusal reports which of the refused options an argument is, and the
// reason, or an empty name when it is none of them.
//
// Three spellings reach the same option and each is named, since git accepts
// all of them and a refusal that knew only one would be walked around by the
// others:
//
//   - The option written whole, or with its value joined by an equals sign.
//   - The short option with its value attached, as in -Xtheirs or -sours. Git
//     reads the rest of the argument as the value, so a refusal of -X and -s by
//     exact match misses it. Only a single dash counts: --squash begins with
//     -s in the second position and is a different option. The capital -S is a
//     different option as well, which signs the commit, and is not matched
//     since the comparison is case sensitive.
//   - An abbreviation of a long option. Git takes any prefix of a long option
//     that is not ambiguous, so --strategy-opt reaches --strategy-option and
//     --strategy-o as well. Both refused long options are prefixes of
//     --strategy-option, so one comparison covers them. A prefix shorter than
//     three characters after the dashes is not matched: --st is ambiguous
//     between several options and git refuses it itself, and an abbreviation
//     that short would also be a prefix of options that are harmless. A prefix
//     that is ambiguous at three or more, such as --str, is refused here
//     though git would also refuse it, which costs nothing.
//
// A cluster of short options, such as -ns ours, is not examined. Which letter
// in it takes a value depends on the other letters, and refusing every cluster
// that holds an s or an X would refuse -S with a key id and -m with a message.
func mergeRefusal(arg string) (name, reason string) {
	if r := mergeRefused[arg]; r != "" {
		return arg, r
	}
	// The option written as one argument is refused by the same name as one
	// written as two, since the refusal is by name and would otherwise miss
	// the form git documents.
	long, _, found := strings.Cut(arg, "=")
	if r := mergeRefused[long]; found && r != "" {
		return long, r
	}
	if len(arg) > 2 && arg[0] == '-' && arg[1] != '-' {
		if short := arg[:2]; mergeRefused[short] != "" {
			return short, mergeRefused[short]
		}
	}
	// The long names are compared by prefix. --strategy is itself a prefix of
	// --strategy-option, so an argument no longer than it is the strategy and
	// a longer one is the strategy option.
	const abbreviable = "--strategy-option"
	if len(long) >= len("--")+mergeAbbreviationMin && strings.HasPrefix(abbreviable, long) {
		if len(long) <= len("--strategy") {
			return "--strategy", mergeRefused["--strategy"]
		}
		return abbreviable, mergeRefused[abbreviable]
	}
	return "", ""
}

// mergeAbbreviationMin is how many characters after the dashes an abbreviation
// of a refused long option needs before it is refused. See mergeRefusal.
const mergeAbbreviationMin = 3

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
		if name, reason := mergeRefusal(arg); name != "" {
			return fmt.Errorf("git %s will not take %s: %s", gitMerge, name, reason)
		}
	}
	return nil
}
