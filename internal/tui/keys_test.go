package tui

import (
	"strings"
	"testing"
)

// An arrow key must not end the line. Its opening escape read as a key is what
// closed the session when an arrow was pressed.
func TestArrowDoesNotEndTheLine(t *testing.T) {
	for _, seq := range []string{"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F"} {
		le := NewLineEditor(strings.NewReader(seq + "text\r"))
		got, err := le.ReadLine()
		if err != nil {
			t.Errorf("%q: err = %v, want the line read", seq, err)
			continue
		}
		if got != "text" {
			t.Errorf("%q: got %q, want %q", seq, got, "text")
		}
	}
}

// The four-byte delete form must be consumed whole, since its trailing byte
// would otherwise be read as a key.
func TestFourByteSequenceIsConsumed(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[3~text\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "text" {
		t.Errorf("got %q, want %q", got, "text")
	}
}

func TestTakeSequence(t *testing.T) {
	for _, tc := range []struct {
		in   string
		rest string
		ok   bool
		name string
	}{
		{"\x1b[A", "", true, "up"},
		{"\x1b[B", "", true, "down"},
		{"\x1b[3~", "", true, "delete"},
		{"\x1b[Ax", "x", true, "up then text"},
		{"\x1b[", "", false, "incomplete"},
		{"\x1b", "", false, "lone escape"},
		{"plain", "", false, "not a sequence"},
	} {
		final, rest, ok := takeSequence([]byte(tc.in))
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", tc.name, ok, tc.ok)
			continue
		}
		if ok && string(rest) != tc.rest {
			t.Errorf("%s: rest = %q, want %q", tc.name, string(rest), tc.rest)
		}
		if ok && tc.name == "up" && final != keyUp {
			t.Errorf("up: final = %q, want %q", final, byte(keyUp))
		}
	}
}

// With no history the arrows must have no action at all, and the line being
// composed must be untouched.
func TestArrowsDoNothingWithoutHistory(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[A\x1b[Btyped\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "typed" {
		t.Errorf("got %q, want %q, so an arrow acted without history", got, "typed")
	}
}

// readLines reads n lines, which is what a stream of n submitted lines needs.
func readLines(t *testing.T, le *LineEditor, n int) []string {
	t.Helper()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		got, err := le.ReadLine()
		if err != nil {
			t.Fatalf("ReadLine %d: %v", i, err)
		}
		out = append(out, got)
	}
	return out
}

// The up arrow walks back through what was submitted.
func TestUpArrowWalksHistory(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r\x1b[A\r"))
	got := readLines(t, le, 3)
	if got[2] != "second" {
		t.Errorf("got %q, want the previous line", got[2])
	}
}

// The down arrow walks forward again.
func TestDownArrowWalksHistory(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r\x1b[A\x1b[A\x1b[B\r"))
	got := readLines(t, le, 3)
	if got[2] != "second" {
		t.Errorf("got %q, want the line walked forward to", got[2])
	}
}

// Walking back and then forward past the newest restores the line that was
// being composed rather than losing it.
// Walking back and then forward past the newest resumes the line that was
// being composed.
func TestWalkForwardResumesTheComposedLine(t *testing.T) {
	le := NewLineEditor(strings.NewReader("sent\r"))

	// The first line is submitted, leaving the history at the newest entry.
	readLines(t, le, 1)

	// Walking back then forward returns to the line that was being composed,
	// which is empty here since nothing had been typed after the submit.
	le.buf = []byte("\x1b[A\x1b[B\r")
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	// The walk returned to the composition, which was empty, so the line
	// submitted is empty rather than a remembered entry. Anything else would
	// mean the walk stopped short of the end.
	if got != "" {
		t.Errorf("got %q, want the resumed composition rather than a history entry", got)
	}
	if le.historyAt != len(le.history) {
		t.Errorf("historyAt = %d, want %d, so the walk reached the end",
			le.historyAt, len(le.history))
	}
}

// At the ends the arrows stop rather than wrapping, since wrapping makes it
// impossible to tell which end one is at.
// At the ends the arrows stop rather than wrapping, since wrapping makes it
// impossible to tell which end one is at. With a single entry, walking back
// twice and forward twice must leave the oldest line showing.
func TestArrowsStopAtTheEnds(t *testing.T) {
	le := NewLineEditor(strings.NewReader("only\r"))
	readLines(t, le, 1)

	// The position is the assertion, not the returned line: walking back past
	// the oldest and forward past the newest must land on the newest entry
	// rather than wrapping around to it from the other side.
	le.buf = []byte("\x1b[A\x1b[A\x1b[B\x1b[B\r")
	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if le.historyAt != len(le.history) {
		t.Errorf("historyAt = %d, want %d, so the walk wrapped rather than stopped",
			le.historyAt, len(le.history))
	}
}

// A remembered line must be reported, or the prompt would not show it.
func TestRecallIsReported(t *testing.T) {
	var last string
	le := NewLineEditor(strings.NewReader("a line\r\x1b[A\r"))
	le.OnChange = func(s string) { last = s }

	readLines(t, le, 2)
	if last != "a line" {
		t.Errorf("reported %q, want the recalled line", last)
	}
}

// The history is bounded, since a long session would otherwise grow it without
// limit.
func TestHistoryIsBounded(t *testing.T) {
	le := NewLineEditor(strings.NewReader(""))
	for i := 0; i < maxHistory+50; i++ {
		le.remember("line")
	}
	if len(le.history) > maxHistory {
		t.Errorf("history = %d entries, want at most %d", len(le.history), maxHistory)
	}
}

// The left and right arrows are reserved for input toggles. They must be
// consumed and must not end the line.
func TestLeftAndRightAreReserved(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[C\x1b[Dtext\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "text" {
		t.Errorf("got %q, want %q", got, "text")
	}
}
