package tui

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

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

// A write to the frame and the paint path that copies it must not be able to
// run at the same time.
//
// The paint path copies the whole frame under the lock, and it does so while
// the twiddle turns and while a deferred repaint is drawn, which is most of
// the time a turn is in flight. A pane write made without the lock is
// therefore a write the copy can be taken across, and the reader sees a
// half-written exchange rather than a late one.
//
// The race is not visible from one goroutine, so it is not left to the race
// detector alone. A write is made while the lock the paint path holds is held
// on purpose, and the write has to wait for it.
func TestPaneWriteWaitsForTheLockThePaintPathHolds(t *testing.T) {
	s, capture := auditSession(t, auditStream)

	s.mu.Lock()
	done := make(chan struct{})
	go func() {
		s.addReply("written while the paint path held the lock")
		close(done)
	}()

	select {
	case <-done:
		s.mu.Unlock()
		t.Fatal("a pane write went in while the paint path held the lock")
	case <-time.After(50 * time.Millisecond):
	}
	s.mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the pane write did not complete once the lock was released")
	}

	s.draw()
	if got := auditLastFrame(t, capture); !strings.Contains(got, "written while the paint path held the lock") {
		t.Errorf("the line the write was waiting for is not on the screen.")
	}
}

// The same lock guards the status fields, which the request goroutine writes
// while the paint path is copying the frame it read them from.
func TestStatusWriteWaitsForTheLockThePaintPathHolds(t *testing.T) {
	s, _ := auditSession(t, auditStream)

	s.mu.Lock()
	done := make(chan struct{})
	go func() {
		s.updateStatus()
		close(done)
	}()

	select {
	case <-done:
		s.mu.Unlock()
		t.Fatal("the status was written while the paint path held the lock")
	case <-time.After(50 * time.Millisecond):
	}
	s.mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the status write did not complete once the lock was released")
	}
}

// interruptSession returns a session reading its keys from a pipe, with the
// callbacks Start installs, so that a key can be delivered the way a terminal
// would deliver it rather than by calling the path under test directly.
func interruptSession(t *testing.T, keys string) (*Session, func() string) {
	t.Helper()
	s, capture := auditSession(t, auditStream)

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating the input pipe: %v", err)
	}
	t.Cleanup(func() { reader.Close(); writer.Close() })

	s.editor = NewLineEditor(reader)
	s.editor.OnChange = func(line string) {
		s.mu.Lock()
		s.frame.Input = line
		s.mu.Unlock()
		s.draw()
	}

	go func() {
		io.WriteString(writer, keys)
		writer.Close()
	}()
	return s, capture
}

// runSession drives Run on its own goroutine and reports what it returned,
// bounded so that a session that never leaves fails rather than hangs.
func runSession(t *testing.T, s *Session) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the session did not leave")
		return nil
	}
}

// An interrupt with a line in hand abandons the line rather than the session,
// which is what a shell does.
//
// The editor reports the composed line after every keystroke, so the frame
// holds it. The frame was cleared before it was tested, which left nothing to
// test and made every interrupt end the session whatever had been typed.
func TestInterruptWithALineInHandAbandonsTheLine(t *testing.T) {
	s, capture := interruptSession(t, "half a thought\x03")

	if err := runSession(t, s); errors.Is(err, ErrQuit) {
		t.Fatal("an interrupt with a line in hand ended the session")
	}

	if got := auditLastFrame(t, capture); strings.Contains(got, "> half a thought") {
		t.Errorf("the abandoned line was sent to the model.\n%s", got)
	}
}

// An interrupt with nothing in hand is the way out, and must still be one.
// The two are told apart by what was in hand, so the fix for the case above
// must not swallow this one.
func TestInterruptWithNoLineInHandEndsTheSession(t *testing.T) {
	s, _ := interruptSession(t, "\x03")

	if err := runSession(t, s); !errors.Is(err, ErrQuit) {
		t.Errorf("an interrupt with no line in hand returned %v, want ErrQuit", err)
	}
}

// The status must not report a request finished while it is still running.
//
// The accounting arrives on the last chunk of a turn and calls updateStatus
// with it, which wrote the idle state over the working one. The bar then read
// idle while the reply was still arriving and the twiddle still turning, which
// is the one reading the state field exists to prevent.
func TestStatusReportsWorkWhileARequestIsInFlight(t *testing.T) {
	s, _ := auditSession(t, auditStream)

	s.mu.Lock()
	s.frame.Busy = true
	s.mu.Unlock()

	s.updateStatus()

	s.mu.Lock()
	state := s.frame.Status.State
	s.mu.Unlock()
	if state != stateWorking {
		t.Errorf("the state is %q with a request in flight, want %q", state, stateWorking)
	}
}

// The other half of the same reading: with nothing in flight the bar is idle,
// which is what the derivation must not lose.
func TestStatusReportsIdleWithNoRequestInFlight(t *testing.T) {
	s, _ := auditSession(t, auditStream)

	s.updateStatus()

	s.mu.Lock()
	state := s.frame.Status.State
	s.mu.Unlock()
	if state != stateIdle {
		t.Errorf("the state is %q with no request in flight, want %q", state, stateIdle)
	}
}
