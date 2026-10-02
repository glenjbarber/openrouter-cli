package tools

import (
	"strings"
	"testing"
)

// TestGitMergeRefusesTheOptionsThatAnswerAConflict checks the guard on merge,
// since it is the one permitted subcommand whose options can resolve a
// disagreement without anybody being told.
//
// -X ours completes the merge, exits zero and records a commit, and nothing in
// the output says a conflict existed. The work it dropped surfaces later as a
// fault in whatever depended on it, which is why the option is refused whole
// rather than by value: a model picking the side is not a reader picking it.
func TestGitMergeRefusesTheOptionsThatAnswerAConflict(t *testing.T) {
	s, _ := gitFixture(t)

	for _, c := range []struct{ args, want string }{
		{`{"args":["merge","-X","ours","feature"]}`, "answers a conflict"},
		{`{"args":["merge","-Xtheirs","feature"]}`, "answers a conflict"},
		{`{"args":["merge","--strategy-option=ours","feature"]}`, "passes an option"},
		{`{"args":["merge","--strategy","recursive-ours","feature"]}`, "names the merge strategy"},
		{`{"args":["merge","-s","ort","feature"]}`, "names the merge strategy"},
		{`{"args":["merge","-sours","feature"]}`, "names the merge strategy"},
		{`{"args":["merge","-sort","feature"]}`, "names the merge strategy"},
		{`{"args":["merge","-Xignore-space-change","feature"]}`, "answers a conflict"},
		{`{"args":["merge","--strat=ours","feature"]}`, "names the merge strategy"},
		{`{"args":["merge","--strategy-opt=theirs","feature"]}`, "passes an option"},
		{`{"args":["merge","--strategy-o","theirs","feature"]}`, "passes an option"},
	} {
		err := mustFail(t, call(t, s, gitTool, c.args))
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s was refused without the reason: %v", c.args, err)
		}
		if !strings.Contains(err.Error(), "merge") {
			t.Errorf("the refusal for %s did not name the subcommand: %v", c.args, err)
		}
	}

	// Nothing was merged, which a refusal after the fact would not prove: the
	// option would already have answered the conflict and recorded a commit.
	log := mustText(t, call(t, s, gitTool, `{"args":["log","--oneline"]}`))
	if lines := strings.Count(strings.TrimRight(log, "\n"), "\n") + 1; lines != 1 {
		t.Errorf("the fixture holds %d commits, want the one it was made with: %q",
			lines, log)
	}
}

// TestGitMergeTakesTheOptionsThatStopShort checks the option set left open: the
// ones that shape how a commit is made without deciding its contents, and
// --squash and --no-commit, which leave the result in the working tree.
//
// The argument list is checked rather than the command being run. A merge in a
// fixture holding a single branch is an up-to-date no-op whatever the options,
// so running one would prove nothing about what the tool permits.
func TestGitMergeTakesTheOptionsThatStopShort(t *testing.T) {
	for _, args := range [][]string{
		{"merge", "feature"},
		{"merge", "--squash", "feature"},
		{"merge", "--no-commit", "feature"},
		{"merge", "--no-ff", "feature"},
		{"merge", "--ff-only", "feature"},
		{"merge", "--no-edit", "feature"},
		// Neighbours of the refused options by spelling, which the matching on
		// short and abbreviated forms must leave alone.
		{"merge", "--stat", "feature"},
		{"merge", "--signoff", "feature"},
		{"merge", "-Sabc123", "feature"},
	} {
		argv, err := gitArgv(args)
		if err != nil {
			t.Errorf("git %s was refused: %v", strings.Join(args, " "), err)
			continue
		}
		if argv[0] != "merge" {
			t.Errorf("git %s ran as %s", strings.Join(args, " "), strings.Join(argv, " "))
		}
	}
}
