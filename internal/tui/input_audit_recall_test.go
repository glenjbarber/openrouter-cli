package tui

import (
	"strings"
	"testing"
)

// Walking forward past the newest must resume the line that was being composed.
// The composition is captured on leaving it for the history, since a walk that
// discarded it would leave the reader with nothing when they walked back out.
func TestAuditWalkingForwardResumesTheComposedLine(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r" + "typed" + "\x1b[A\x1b[B\r"))

	got := readLines(t, le, 3)
	if got[2] != "typed" {
		t.Errorf("got %q, want the line that was being composed", got[2])
	}
}

// The same after two recalls, since the composition is held aside for the walk
// rather than overwritten by each remembered line.
func TestAuditWalkingForwardTwiceResumesTheComposedLine(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r" +
		"typed\x1b[A\x1b[A\x1b[B\x1b[B\r"))

	got := readLines(t, le, 3)
	if got[2] != "typed" {
		t.Errorf("got %q, want the line that was being composed", got[2])
	}
}

// Walking forward past the newest with nothing composed restores an empty line
// rather than the line remembered before it.
func TestAuditWalkingForwardFromNothingComposed(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\r\x1b[A\x1b[B\r"))

	got := readLines(t, le, 2)
	if got[1] != "" {
		t.Errorf("got %q, want an empty line", got[1])
	}
}

// The arrows stop at each end rather than wrapping, since wrapping makes it
// impossible to tell which end one is at.
func TestAuditArrowsStopAtTheEnds(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r" + "\x1b[A\x1b[A\x1b[A\x1b[A\r"))

	got := readLines(t, le, 3)
	if got[2] != "first" {
		t.Errorf("got %q, want the oldest line, so an arrow wrapped", got[2])
	}

	// Forward from the newest end, where there is nothing ahead, does nothing
	// at all rather than wrapping back to the oldest.
	le = NewLineEditor(strings.NewReader("first\rsecond\r\x1b[B\x1b[B\r"))
	got = readLines(t, le, 3)
	if got[2] != "" {
		t.Errorf("got %q, want the line left alone, so an arrow wrapped", got[2])
	}
}

// A paste that has landed is abandoned with the line, so that text the reader
// discarded is not prepended to whatever they type next.
func TestAuditAbandonedPasteIsNotCarriedIntoTheNextLine(t *testing.T) {
	le := NewLineEditor(strings.NewReader(
		pasteStart + "pasted" + pasteEnd + // the paste lands
			"\x03" + // and the line is abandoned
			"typed\r"))

	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Fatalf("err = %v, want the paste abandoned with the line", err)
	}
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "typed" {
		t.Errorf("got %q, want the next line on its own", got)
	}
}

// A paste abandoned with an escape rather than a control character is abandoned
// in the same way, since an escape on an idle prompt is an interrupt.
func TestAuditAbandonedPasteOnEscapeIsNotCarried(t *testing.T) {
	le := NewLineEditor(strings.NewReader(
		pasteStart + "pasted" + pasteEnd + "\x1b" + "typed\r"))

	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Fatalf("err = %v, want the paste abandoned with the line", err)
	}
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "typed" {
		t.Errorf("got %q, want the next line on its own", got)
	}
}
