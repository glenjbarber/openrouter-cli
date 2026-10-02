package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/complete"
)

// Tab on a prefix naming one command completes to it and adds the space that
// separates it from what follows. A reader pressing Tab and typing nothing else
// should be able to send the line.
func TestAUniqueCompletionAddsASpace(t *testing.T) {
	s := completionSession()

	got := s.completeLine("/comp")

	if got != "/compact " {
		t.Errorf("completing /comp gave %q, want %q", got, "/compact ")
	}
}

// A line that already ends in a space is left alone, since a second space is
// something the reader typed rather than something completion added.
func TestTheSpaceIsNotAddedTwice(t *testing.T) {
	if got := withTrailingSpace("/model "); got != "/model " {
		t.Errorf("a line already spaced gave %q", got)
	}
	if got := withTrailingSpace("/model"); got != "/model " {
		t.Errorf("an unspaced line gave %q", got)
	}
	if got := withTrailingSpace(""); got != "" {
		t.Errorf("an empty line gave %q", got)
	}
}

// Tab on an ambiguous prefix completes to the first match rather than only
// listing them, since a reader pressing Tab is asking for a word.
func TestTheFirstTabCompletesToTheFirstMatch(t *testing.T) {
	s := completionSession()

	got := s.completeLine("/mo")

	if got == "" {
		t.Fatal("an ambiguous prefix was not completed at all")
	}
	if !strings.HasPrefix(got, "/mo") {
		t.Errorf("completing /mo gave %q, want a match for it", got)
	}
	if !strings.HasSuffix(got, " ") {
		t.Errorf("completing /mo gave %q, want a trailing space", got)
	}
}

// A second Tab moves to the next match rather than completing to the first
// again, which is what a reader pressing Tab again is asking for.
func TestTheSecondTabMovesToTheNextMatch(t *testing.T) {
	s := completionSession()

	first := s.completeLine("/mo")
	// A reader pressing Tab again has not typed anything, so the line is
	// whatever the completion left there. The editor hands it back unchanged.
	second := s.completeLine(first)

	if second == "" {
		t.Fatal("the second Tab did nothing")
	}
	if second == first {
		t.Errorf("the second Tab completed to the same match: %q", first)
	}
	if !strings.HasPrefix(second, "/mo") {
		t.Errorf("the second match is %q, which does not match the prefix", second)
	}
}

// The cycle wraps round, as the model filter does, so a reader who has seen
// the whole set and presses Tab again is taken back to the first rather than
// handed a key that has stopped doing anything.
func TestTheCycleWrapsRound(t *testing.T) {
	s := completionSession()

	line := "/mo"
	seen := map[string]bool{}
	var first string
	for i := 0; i < 5; i++ {
		line = s.completeLine(line)
		if i == 0 {
			first = line
		}
		seen[line] = true
	}

	if len(seen) < 2 {
		t.Fatalf("five presses of Tab reached %d matches", len(seen))
	}
	if !seen[first] {
		t.Errorf("the cycle did not come back to the first match: %v", seen)
	}
}

// A reader who typed more since the last Tab is asking something else, and
// cycling would complete to a word that no longer matches.
func TestTypingBetweenCompletionsStartsTheCycleAgain(t *testing.T) {
	s := completionSession()

	first := s.completeLine("/mo")
	s.clearCycle()

	second := s.completeLine("/mo")

	if second != first {
		t.Errorf("a changed prefix cycled rather than started again: %q then %q",
			first, second)
	}
}

// One match is not a cycle, so the line is completed and the candidates are
// not offered for something with nowhere to go.
func TestAUniqueMatchIsNotCycled(t *testing.T) {
	s := completionSession()

	first := s.completeLine("/comp")
	second := s.completeLine(strings.TrimSpace(first))

	if second == "" {
		t.Error("the second Tab emptied the line")
	}
	if second != first {
		t.Errorf("a unique match cycled: %q then %q", first, second)
	}
}

// The completion is reported where the reader pressed, not in the pane.
func TestACompletionSaysNothingInThePane(t *testing.T) {
	s := completionSession()

	s.completeLine("/mo")

	if s.frame.Notice != "" {
		t.Errorf("a completion that chose a word left a notice: %q", s.frame.Notice)
	}
}

// Nothing is completed when the prefix names nothing, and the reader is told so
// beside the prompt.
func TestNothingIsCompletedAndItSaysSo(t *testing.T) {
	s := completionSession()

	if got := s.completeLine("/zzz"); got != "" {
		t.Errorf("completing /zzz gave %q", got)
	}
	if !strings.Contains(s.frame.Notice, "/zzz") {
		t.Errorf("nothing was said about it: %q", s.frame.Notice)
	}
}

// A line that is not a command is left alone rather than guessed at.
func TestAnOrdinaryLineIsNotCompleted(t *testing.T) {
	s := completionSession()

	for _, line := range []string{"", "hello there", "/model gpt"} {
		if got := s.completeLine(line); got != "" {
			t.Errorf("%q completed to %q", line, got)
		}
	}
}

// The candidates are still offered for an ambiguous prefix, since a reader who
// wants the list rather than a word still wants it.
func TestTheCandidatesAreStillListed(t *testing.T) {
	s := completionSession()

	s.completeLine("/mo")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "matching") {
		t.Errorf("the candidates were not listed:\n%s", joined)
	}
}

// The completer itself is unchanged, so the result kinds are what it reports.
func TestTheResultKindsAreUnchanged(t *testing.T) {
	c := complete.New(candidates())

	if got := c.Complete("/comp", 5); got.Kind != complete.Unique {
		t.Errorf("/comp is %v, want Unique", got.Kind)
	}
	if got := c.Complete("/mo", 3); got.Kind != complete.Ambiguous {
		t.Errorf("/mo is %v, want Ambiguous", got.Kind)
	}
}

// The listing says where in the set the reader is, as the model filter does,
// since the line itself already holds the choice.
func TestTheListingSaysWhichMatchIsInForce(t *testing.T) {
	s := completionSession()

	s.completeLine("/mo")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "[1 of 3]") {
		t.Errorf("the listing does not say which match is in force:\n%s", joined)
	}

	s.completeLine(s.completeLine("/mo"))
	joined = strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "[2 of 3]") {
		t.Errorf("the listing does not follow the cycle:\n%s", joined)
	}
}
