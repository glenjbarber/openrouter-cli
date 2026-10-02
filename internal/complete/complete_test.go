package complete

import (
	"strings"
	"testing"
)

// testCommands mirrors the shape of the real command vocabulary, so that the
// cases below read as the reader would meet them. The names are supplied by
// the interface in use; the completer itself holds no names.
func testCommands() []Candidate {
	return []Candidate{
		{"/help", "this list"},
		{"/clear", "clear the pane"},
		{"/quit", "leave the interface"},
		{"/exit", "leave the interface"},
		{"/connect", "test the connection"},
		{"/key", "report the usage"},
		{"/search", "search the pane"},
		{"/models", "list the models"},
		{"/freemodels", "list the free models"},
		{"/model", "choose the model"},
		{"/new", "clear the conversation"},
		{"/bell", "ring the bell"},
		{"/cognito", "record nothing"},
		{"/verbose", "report the stream"},
		{"/delegate", "ask alongside"},
		{"/btw", "start a thread"},
		{"/main", "leave the thread"},
		{"/compact", "summarise"},
		{"/mouse", "turn the mouse on or off"},
		{"/info", "report the settings"},
	}
}

func testCompleter() Completer { return New(testCommands()) }

// complete runs the completer with the caret at the end of the line, which is
// where the line editor holds it, since the editor does not move the caret.
func completeAt(c Completer, line string) Result { return c.Complete(line, len(line)) }

// names returns the candidate names, for comparison without the descriptions.
func names(cands []Candidate) []string {
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.Name)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A prefix matching exactly one command completes to it, and the whole line is
// returned with the command in place of what was typed.
func TestUniqueCommandCompletes(t *testing.T) {
	res := completeAt(testCompleter(), "/comp")
	if res.Kind != Unique {
		t.Fatalf("Kind = %v, want Unique", res.Kind)
	}
	if !res.Applied() {
		t.Error("Applied = false, want true for a unique match")
	}
	if res.Line != "/compact" {
		t.Errorf("Line = %q, want %q", res.Line, "/compact")
	}
	if res.Set != "commands" {
		t.Errorf("Set = %q, want commands", res.Set)
	}
}

// The comparison ignores case, since a reader types a word in whatever case
// they happen to use, and the completed word takes the case of the table.
func TestCommandCompletionIgnoresCase(t *testing.T) {
	for _, typed := range []string{"/COMP", "/Comp", "/cOmPaCt"} {
		res := completeAt(testCompleter(), typed)
		if res.Kind != Unique {
			t.Errorf("%q: Kind = %v, want Unique", typed, res.Kind)
			continue
		}
		if res.Line != "/compact" {
			t.Errorf("%q: Line = %q, want %q", typed, res.Line, "/compact")
		}
	}
}

// An ambiguous prefix is not guessed at. The line is left as it was typed and
// the candidates are reported, since choosing one of them would pick a command
// the reader did not ask for.
func TestAmbiguousPrefixReportsCandidates(t *testing.T) {
	res := completeAt(testCompleter(), "/mo")
	if res.Kind != Ambiguous {
		t.Fatalf("Kind = %v, want Ambiguous", res.Kind)
	}
	if res.Applied() {
		t.Error("Applied = true, want false for an ambiguous prefix")
	}
	if res.Line != "/mo" {
		t.Errorf("Line = %q, want the line unchanged", res.Line)
	}
	want := []string{"/models", "/model", "/mouse"}
	if got := names(res.Candidates); !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

// The candidates are reported in the order the set declares them, so a listing
// is the same on every run.
func TestAmbiguousCandidatesKeepSetOrder(t *testing.T) {
	// /m matches the model, main and mouse commands, and the set declares
	// them in this order.
	res := completeAt(testCompleter(), "/m")
	want := []string{"/models", "/model", "/main", "/mouse"}
	if got := names(res.Candidates); !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

// A prefix matching no command says so rather than completing to nothing. It
// is distinct from a position where completion does not apply, since a reader
// who has mistyped a command should be told the command is not found rather
// than told there is nothing to complete.
func TestNoMatchReportsNoMatch(t *testing.T) {
	res := completeAt(testCompleter(), "/zzz")
	if res.Kind != NoMatch {
		t.Fatalf("Kind = %v, want NoMatch", res.Kind)
	}
	if res.Applied() {
		t.Error("Applied = true, want false when nothing matched")
	}
	if len(res.Candidates) != 0 {
		t.Errorf("candidates = %v, want none", names(res.Candidates))
	}
	if res.Prefix != "/zzz" {
		t.Errorf("Prefix = %q, want the word that was typed", res.Prefix)
	}
	if res.Line != "/zzz" {
		t.Errorf("Line = %q, want the line unchanged", res.Line)
	}
}

// A command position offers commands only. A word that looks like a
// configuration option is not found there, since the two sets are kept apart.
func TestCommandPositionOffersCommandsOnly(t *testing.T) {
	res := completeAt(testCompleter(), "/OPENR")
	if res.Kind != NoMatch {
		t.Errorf("Kind = %v, want NoMatch: an option is not a command", res.Kind)
	}
	if res.Set != "commands" {
		t.Errorf("Set = %q, want commands", res.Set)
	}
}

// A slash on its own matches every command, which is how a reader discovers
// the vocabulary without knowing a name in advance.
func TestSlashAloneListsEveryCommand(t *testing.T) {
	res := completeAt(testCompleter(), "/")
	if res.Kind != Ambiguous {
		t.Fatalf("Kind = %v, want Ambiguous", res.Kind)
	}
	if got, want := len(res.Candidates), len(testCommands()); got != want {
		t.Errorf("candidates = %d, want every one of the %d commands", got, want)
	}
}

// A configuration option is completed as a whole word, so a prefix that
// matches one of them exactly completes to it.
func TestOptionCompletes(t *testing.T) {
	res := completeAt(testCompleter(), "OPENROUTER_B")
	if res.Kind != Unique {
		t.Fatalf("Kind = %v, want Unique", res.Kind)
	}
	if res.Line != "OPENROUTER_BELL" {
		t.Errorf("Line = %q, want %q", res.Line, "OPENROUTER_BELL")
	}
	if res.Set != "options" {
		t.Errorf("Set = %q, want options", res.Set)
	}
}

// The options are named in the table, and a word typed in another case is
// completed to the case the table carries.
func TestOptionIgnoresCase(t *testing.T) {
	res := completeAt(testCompleter(), "openrouter_bell")
	if res.Kind != Unique {
		t.Fatalf("Kind = %v, want Unique", res.Kind)
	}
	if res.Line != "OPENROUTER_BELL" {
		t.Errorf("Line = %q, want %q", res.Line, "OPENROUTER_BELL")
	}
}

// A prefix matching several options reports them rather than choosing one.
func TestOptionAmbiguityReportsCandidates(t *testing.T) {
	res := completeAt(testCompleter(), "OPENROUTER_MO")
	if res.Kind != Ambiguous {
		t.Fatalf("Kind = %v, want Ambiguous", res.Kind)
	}
	want := []string{"OPENROUTER_MODEL", "OPENROUTER_MOUSE"}
	if got := names(res.Candidates); !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
	if res.Set != "options" {
		t.Errorf("Set = %q, want options", res.Set)
	}
}

// An option position offers options only, so a command name typed without its
// slash is not found there.
func TestOptionPositionOffersOptionsOnly(t *testing.T) {
	res := completeAt(testCompleter(), "comp")
	if res.Kind != NoMatch {
		t.Errorf("Kind = %v, want NoMatch: a command needs its slash", res.Kind)
	}
	if res.Set != "options" {
		t.Errorf("Set = %q, want options", res.Set)
	}
	if res.Line != "comp" {
		t.Errorf("Line = %q, want the line unchanged", res.Line)
	}
}

// A command name is completed only behind its slash. The same word without one
// is a single word being typed, so it reaches the options and is not found.
func TestCommandNotCompletedWithoutSlash(t *testing.T) {
	res := completeAt(testCompleter(), "compact")
	if res.Applied() {
		t.Errorf("Applied = true, want false: %q was completed", res.Line)
	}
	if res.Line == "/compact" {
		t.Errorf("Line = %q, want a command is not completed without its slash", res.Line)
	}
	if res.Kind != NoMatch {
		t.Errorf("Kind = %v, want NoMatch", res.Kind)
	}
}

// An argument after a command is that command's own business, so it is left
// alone. Completing it would be guessing at what the command accepts.
func TestCommandArgumentIsNotCompleted(t *testing.T) {
	for _, line := range []string{"/model g", "/model gpt-4o", "/delegate is this"} {
		res := completeAt(testCompleter(), line)
		if res.Kind != NotApplicable {
			t.Errorf("%q: Kind = %v, want NotApplicable", line, res.Kind)
		}
		if res.Line != line {
			t.Errorf("%q: Line = %q, want the line unchanged", line, res.Line)
		}
	}
}

// An argument position does not fall through to the options either, since a
// word after a command is an argument and not the name of a setting.
func TestCommandArgumentDoesNotCompleteAsOption(t *testing.T) {
	res := completeAt(testCompleter(), "/model OPENROUTER_B")
	if res.Kind != NotApplicable {
		t.Errorf("Kind = %v, want NotApplicable", res.Kind)
	}
}

// Once a line carries a space it is a message being written, so a word inside
// it is not completed as an option. Completing there would put a setting where
// the reader was writing prose.
func TestMessageIsNotCompleted(t *testing.T) {
	for _, line := range []string{"hello OPENRO", "what is a model OPENROUTER_MOUSE"} {
		res := completeAt(testCompleter(), line)
		if res.Kind != NotApplicable {
			t.Errorf("%q: Kind = %v, want NotApplicable", line, res.Kind)
		}
		if res.Line != line {
			t.Errorf("%q: Line = %q, want the line unchanged", line, res.Line)
		}
	}
}

// A caret in the middle of a command name completes the whole token and keeps
// the rest of the line, so nothing either side of it is lost.
func TestCaretInMiddleReplacesWholeToken(t *testing.T) {
	line := "/hel world"
	res := testCompleter().Complete(line, 3)
	if res.Kind != Unique {
		t.Fatalf("Kind = %v, want Unique", res.Kind)
	}
	if res.Line != "/help world" {
		t.Errorf("Line = %q, want %q", res.Line, "/help world")
	}
}

// A caret at the end of the command name, just before a space, still completes
// the name and keeps what follows.
func TestCaretAtEndOfCommandName(t *testing.T) {
	line := "/hel world"
	res := testCompleter().Complete(line, 4)
	if res.Kind != Unique {
		t.Fatalf("Kind = %v, want Unique", res.Kind)
	}
	if res.Line != "/help world" {
		t.Errorf("Line = %q, want %q", res.Line, "/help world")
	}
}

// An empty line has no token to complete, so the key finds nothing to do.
func TestEmptyLineIsNotApplicable(t *testing.T) {
	res := testCompleter().Complete("", 0)
	if res.Kind != NotApplicable {
		t.Errorf("Kind = %v, want NotApplicable", res.Kind)
	}
	if res.Line != "" {
		t.Errorf("Line = %q, want empty", res.Line)
	}
}

// A caret at the head of a line is before any token, so there is nothing to
// complete even though the line holds text.
func TestCaretAtHeadIsNotApplicable(t *testing.T) {
	res := testCompleter().Complete("/comp", 0)
	if res.Kind != NotApplicable {
		t.Errorf("Kind = %v, want NotApplicable", res.Kind)
	}
	if res.Line != "/comp" {
		t.Errorf("Line = %q, want the line unchanged", res.Line)
	}
}

// An offset outside the line is brought inside it rather than refused, so a
// caller that has lost track of the caret is still served.
func TestCaretOutsideLineIsClamped(t *testing.T) {
	c := testCompleter()
	if res := c.Complete("/comp", 100); res.Line != "/compact" {
		t.Errorf("a caret past the end gave Line = %q, want %q", res.Line, "/compact")
	}
	if res := c.Complete("/comp", -5); res.Kind != NotApplicable {
		t.Errorf("a caret before the start gave Kind = %v, want NotApplicable", res.Kind)
	}
}

// A word that is already complete is reported as the single match rather than
// as nothing, since it does match one candidate.
func TestExactNameIsAUniqueMatch(t *testing.T) {
	res := completeAt(testCompleter(), "/compact")
	if res.Kind != Unique {
		t.Errorf("Kind = %v, want Unique", res.Kind)
	}
	if res.Line != "/compact" {
		t.Errorf("Line = %q, want the word unchanged", res.Line)
	}
}

// The result names the outcome in words rather than as a numeral, since a
// report would say nothing with a numeral.
func TestKindString(t *testing.T) {
	for kind, want := range map[Kind]string{
		NotApplicable: "not applicable",
		NoMatch:       "no match",
		Ambiguous:     "ambiguous",
		Unique:        "unique",
	} {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

// The completer does not reach past its own vocabulary. An option position does
// not offer a command and a command position does not offer an option, which is
// what keeps the two sets from being mixed by a prefix that fits both.
func TestSetsAreKeptApart(t *testing.T) {
	// No command name is a prefix of an option name once the leading slash is
	// accounted for, so the two never compete for the same caret.
	for _, c := range testCommands() {
		for _, o := range options {
			if strings.HasPrefix(strings.ToLower(o.Name), strings.ToLower(c.Name)) {
				t.Errorf("option %q begins with command %q", o.Name, c.Name)
			}
		}
	}
}

// The color keys are lower case and carry no OPENROUTER_ prefix, and they are
// completed the same way as the others.
func TestColorOptionsComplete(t *testing.T) {
	res := completeAt(testCompleter(), "color_")
	if res.Kind != Unique || res.Line != "color_theme" {
		t.Errorf("color_ gave %v %q, want Unique color_theme", res.Kind, res.Line)
	}
	res = completeAt(testCompleter(), "col")
	if res.Kind != Ambiguous {
		t.Fatalf("col: Kind = %v, want Ambiguous", res.Kind)
	}
	want := []string{"color", "color_theme"}
	if got := names(res.Candidates); !equalStrings(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}
