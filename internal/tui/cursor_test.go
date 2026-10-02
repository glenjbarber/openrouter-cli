package tui

import (
	"os"
	"strings"
	"testing"
)

// cursorCapture returns a screen writing into a file, so the sequences written
// to the terminal can be read back.
func cursorCapture(t *testing.T) (*Screen, func() string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "cursor")
	if err != nil {
		t.Fatalf("creating the capture: %v", err)
	}
	return &Screen{out: out, in: out, height: 24, width: 80}, func() string {
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatalf("reading the capture: %v", err)
		}
		return string(data)
	}
}

// The sequence that hides the cursor is not written at all. A client that hid
// it and drew a caret of its own would be one thing; this client draws none, so
// hiding it would leave the reader with nothing on the prompt to type at.
func TestTheCursorIsNeverHidden(t *testing.T) {
	sc, read := cursorCapture(t)

	sc.write(seqEnterAlt + seqClear + seqHome + seqPasteOn)
	sc.DrawTinted([]string{"a reply", "> ", ""}, tint{})
	sc.Close()
	got := read()

	if strings.Contains(got, "\x1b[?25l") {
		t.Errorf("the cursor was hidden: %q", got)
	}
	if !strings.Contains(got, seqShowCur) {
		t.Errorf("the cursor was not shown on the way out: %q", got)
	}
}

// Nothing sets the blink rate. A terminal profile is where a reader has already
// said how fast they want it, and a client overriding that would emit a frame
// on a timer for no reason other than to disagree.
func TestTheBlinkRateIsLeftToTheTerminal(t *testing.T) {
	sc, read := cursorCapture(t)

	sc.write(seqEnterAlt + seqClear + seqHome + seqPasteOn)
	sc.Close()
	got := read()

	// Nothing in this client touches the blink: no steady cursor, no timed
	// show and hide. Only mode 25 at all is the show on the way out.
	if strings.Contains(got, "5 q") || strings.Contains(got, "5 h") {
		t.Errorf("the blink was driven by the client: %q", got)
	}
}

// The client draws no caret of its own, so the terminal one is placed on the
// prompt row on every paint. Without that a visible cursor sits wherever it
// was last left, which is nowhere useful.
func TestTheCaretIsPlacedOnThePromptRow(t *testing.T) {
	sc, read := cursorCapture(t)

	sc.DrawTinted([]string{"a reply", "> ", ""}, tint{})

	// Row 2 is the prompt and column 3 is just past the prompt mark.
	if !strings.Contains(read(), "\x1b[2;3H") {
		t.Errorf("the caret was not placed after the prompt: %q", read())
	}
}

// A frame with no prompt draws no caret, since there is nowhere to type.
func TestAFrameWithNoPromptPlacesNoCaret(t *testing.T) {
	sc, read := cursorCapture(t)

	sc.DrawTinted([]string{"a reply", ""}, tint{})

	if strings.Contains(read(), ";1H") {
		t.Errorf("a caret was placed with no prompt to place it on: %q", read())
	}
}
