package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/complete"
)

// requiredNames is the vocabulary the interface is expected to offer.
//
// The list states a requirement rather than describing the code, so that a
// command dropped from the table fails here instead of passing unnoticed. It
// is deliberately not derived from the table: a test that read the table would
// agree with whatever the table happened to say.
var requiredNames = []string{
	"/help", "/clear", "/quit", "/exit", "/connect", "/key", "/search",
	"/models", "/freemodels", "/model", "/new", "/bell", "/cognito",
	"/verbose", "/delegate", "/btw", "/main", "/compact", "/mouse", "/info",
}

// Every name the interface is expected to answer to is declared in the table
// and resolves to a command that does something.
func TestEveryCommandNameIsDeclaredAndResolves(t *testing.T) {
	for _, name := range requiredNames {
		c := lookupCommand(name)
		if c == nil {
			t.Errorf("%s is not in the command table", name)
			continue
		}
		if c.run == nil {
			t.Errorf("%s resolves to a command that does nothing", name)
		}
	}
}

// A name declared twice would make the completer offer the same word twice and
// the dispatcher depend on which copy came first.
func TestCommandNamesAreDeclaredOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands {
		for _, n := range c.names {
			if seen[n] {
				t.Errorf("%s is declared more than once", n)
			}
			seen[n] = true
		}
	}
}

// Every name in the table has a description, since a candidate is shown beside
// its description and an empty one would leave the reader with a bare word.
func TestEveryCommandHasADescription(t *testing.T) {
	for _, c := range commands {
		if c.description == "" {
			t.Errorf("%s has no description", c.usageLine())
		}
		if len(c.names) == 0 {
			t.Errorf("a command with the description %q declares no name", c.description)
		}
		for _, n := range c.names {
			if !strings.HasPrefix(n, "/") {
				t.Errorf("%s does not begin with a slash, so it is never read as a command", n)
			}
		}
	}
}

// The help is rendered from the table, so every command appears in it with the
// description the table carries, and no row is left over from a command that
// has since been removed.
func TestHelpTextIsRenderedFromTheTable(t *testing.T) {
	lines := strings.Split(helpText(), "\n")
	if len(lines) != len(commands) {
		t.Fatalf("help has %d lines for %d commands:\n%s", len(lines), len(commands), helpText())
	}
	for i, c := range commands {
		if !strings.Contains(lines[i], c.usageLine()) {
			t.Errorf("line %d of the help does not carry %q:\n%s", i, c.usageLine(), lines[i])
		}
		if !strings.Contains(lines[i], c.description) {
			t.Errorf("line %d of the help does not carry the description %q:\n%s", i, c.description, lines[i])
		}
	}
}

// The descriptions are aligned in one column, so the list can be read down the
// descriptions rather than hunted for. A name longer than the column pushes
// only itself along rather than pushing its description onto the next row.
func TestHelpDescriptionsShareOneColumn(t *testing.T) {
	lines := strings.Split(helpText(), "\n")
	for i, c := range commands {
		col := strings.Index(lines[i], c.description)
		if col < 0 {
			t.Fatalf("line %d of the help does not carry %q:\n%s", i, c.description, lines[i])
		}
		// A name longer than the column pushes only its own description
		// along, since the alternative is the column moving for every row
		// above it.
		if want := max(helpColumn, len(c.usageLine())) + 2; col != want {
			t.Errorf("line %d of the help puts its description at column %d, want %d:\n%s", i, col, want, lines[i])
		}
	}
}

// The completer is offered the table rather than a list beside it, so every
// declared name is reachable by typing it. A name missing here would be a
// command the reader cannot discover without already knowing it.
func TestCompleterOffersEveryDeclaredName(t *testing.T) {
	offered := map[string]bool{}
	for _, c := range candidates() {
		offered[c.Name] = true
	}
	for _, c := range commands {
		for _, n := range c.names {
			if !offered[n] {
				t.Errorf("%s is not offered by the completer", n)
			}
			delete(offered, n)
		}
	}
	for n := range offered {
		t.Errorf("the completer offers %s, which is not in the command table", n)
	}
}

// A slash on its own lists every command, which is how the vocabulary is
// discovered without a name in mind.
func TestSlashListsEveryCommand(t *testing.T) {
	s := &Session{completer: complete.New(candidates())}
	res := s.completer.Complete("/", 1)
	if res.Kind != complete.Ambiguous {
		t.Fatalf("Kind = %v, want Ambiguous", res.Kind)
	}
	if len(res.Candidates) != len(requiredNames) {
		t.Errorf("a slash matched %d commands, want %d", len(res.Candidates), len(requiredNames))
	}
}

// completionSession is a session with nothing but a completer, which is all the
// completion needs: the reports are written to the reply pane.
func completionSession() *Session {
	return &Session{completer: complete.New(candidates())}
}

// A prefix naming one command is completed, and the line is returned for the
// editor to compose in place of what was typed.
func TestCompleteLineCompletesAUniquePrefix(t *testing.T) {
	s := completionSession()
	if got := s.completeLine("/comp"); got != "/compact" {
		t.Errorf("completing /comp gave %q, want %q", got, "/compact")
	}
	if len(s.frame.Reply) != 0 {
		t.Errorf("a completion that chose a word wrote to the pane: %q", s.frame.Reply)
	}
}

// An ambiguous prefix is listed rather than guessed at, and the line is left as
// it stands. Choosing one of them would complete to a command the reader did
// not ask for.
func TestCompleteLineListsAnAmbiguousPrefix(t *testing.T) {
	s := completionSession()
	if got := s.completeLine("/mo"); got != "" {
		t.Errorf("completing /mo gave %q, want the line unchanged", got)
	}
	body := strings.Join(s.frame.Reply, "\n")
	for _, want := range []string{"/models", "/model", "/mouse"} {
		if !strings.Contains(body, want) {
			t.Errorf("the listing does not carry %s:\n%s", want, body)
		}
	}
}

// A prefix matching nothing says so rather than leaving the reader to wonder
// whether the key was heard.
func TestCompleteLineReportsNoMatch(t *testing.T) {
	s := completionSession()
	if got := s.completeLine("/zzz"); got != "" {
		t.Errorf("completing /zzz gave %q, want the line unchanged", got)
	}
	if body := strings.Join(s.frame.Reply, "\n"); !strings.Contains(body, "/zzz") {
		t.Errorf("the pane does not say what was looked for:\n%s", body)
	}
}

// A caret where completion means nothing still says so. A Tab that changed
// nothing and wrote nothing would read as a dead key.
func TestCompleteLineIsNeverSilent(t *testing.T) {
	for _, line := range []string{"", "hello there", "/model gpt"} {
		s := completionSession()
		if got := s.completeLine(line); got != "" {
			t.Errorf("%q: completing gave %q, want the line unchanged", line, got)
		}
		if len(s.frame.Reply) == 0 {
			t.Errorf("%q: a Tab that completed nothing wrote nothing to the pane", line)
		}
	}
}

// The configuration option names are offered where a setting is being named,
// which is a bare single word rather than a word inside a message.
func TestCompleteLineOffersConfigurationOptions(t *testing.T) {
	s := completionSession()
	if got := s.completeLine("OPENROUTER_MOUSE"); got != "OPENROUTER_MOUSE" {
		t.Errorf("completing an option that was already whole gave %q", got)
	}
	if got := s.completeLine("OPENROUTER_B"); got != "OPENROUTER_BELL" {
		t.Errorf("completing OPENROUTER_B gave %q, want %q", got, "OPENROUTER_BELL")
	}
	for _, line := range []string{"hello OPENRO", "what is a model OPENROUTER_MOUSE"} {
		s := completionSession()
		if got := s.completeLine(line); got != "" {
			t.Errorf("%q: completing gave %q, want the line unchanged", line, got)
		}
	}
}

// A word the table does not carry is reported rather than refused, since what
// the reader typed looks like a command and is worth answering.
func TestUnknownCommandIsReported(t *testing.T) {
	s := completionSession()
	if s.command("/nope") {
		t.Error("an unknown command ended the session")
	}
	if body := strings.Join(s.frame.Reply, "\n"); !strings.Contains(body, "unknown command: /nope") {
		t.Errorf("the pane does not report the unknown command:\n%s", body)
	}
}

// /quit and /exit are one command reached by two words, and both leave. The
// test calls the entry rather than the session, so that no test quits.
func TestQuitAndExitAreTheSameCommand(t *testing.T) {
	quit := lookupCommand("/quit")
	exit := lookupCommand("/exit")
	if quit == nil || exit == nil {
		t.Fatal("both /quit and /exit must be declared")
	}
	if quit != exit {
		t.Error("/quit and /exit resolve to different commands, so one would leave and the other would not")
	}
}
