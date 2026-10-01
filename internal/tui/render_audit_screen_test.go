package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureScreen returns a Screen writing to a file that can be read back.
//
// The terminal ioctls fail on a regular file, which does not matter here: the
// restore ignores their error, since there is nothing to put back when the
// termios was never changed. What the test reads is the byte stream, which is
// what the terminal would have been sent.
func captureScreen(t *testing.T, mouse bool) (*Screen, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	s := &Screen{out: f, in: f, mouse: mouse}
	return s, func() string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the capture file: %v", err)
		}
		return string(b)
	}
}

// Bracketed paste must be turned off on the way out. The mode belongs to the
// terminal rather than to the alternate screen, so it survives the switch
// back: a shell left believing the mode is on wraps a paste in markers that
// nothing reads, and the reader loses the text they pasted.
func TestCloseTurnsBracketedPasteOff(t *testing.T) {
	s, read := captureScreen(t, false)
	s.Close()
	out := read()

	if !strings.Contains(out, seqPasteOff) {
		t.Errorf("the exit did not turn bracketed paste off: %q", out)
	}
	pasteAt := strings.Index(out, seqPasteOff)
	altAt := strings.Index(out, seqExitAlt)
	if pasteAt < 0 || altAt < 0 || pasteAt > altAt {
		t.Errorf("bracketed paste must be turned off before the alternate screen is left: %q", out)
	}
}

// The alternate screen must be left at all, since leaving it behind would
// leave the reader in the frame rather than in their shell.
func TestCloseLeavesTheAlternateScreen(t *testing.T) {
	s, read := captureScreen(t, false)
	s.Close()
	out := read()

	if !strings.Contains(out, seqExitAlt) {
		t.Errorf("the alternate screen was not left: %q", out)
	}
	if !strings.Contains(out, seqShowCur) {
		t.Errorf("the cursor was not shown again: %q", out)
	}
}

// Mouse reporting is put back the way it was found, which means off only when
// this client turned it on. A terminal that was already reporting is left
// reporting, since that is the state it was found in and another program on
// the same terminal may depend on it.
func TestCloseRestoresMouseReportingAsFound(t *testing.T) {
	s, read := captureScreen(t, true)
	s.Close()
	if out := read(); !strings.Contains(out, seqMouseOff) {
		t.Errorf("reporting was left on: %q", out)
	}

	s, read = captureScreen(t, false)
	s.Close()
	if out := read(); strings.Contains(out, seqMouseOff) {
		t.Errorf("reporting was turned off although this client never turned it on: %q", out)
	}
}

// The signal handler and the ordinary exit both restore, and a signal arriving
// during the exit must not restore twice. A second restore would leave the
// alternate screen twice over, which on some terminals leaves the screen
// cleared.
func TestCloseIsIdempotent(t *testing.T) {
	s, read := captureScreen(t, true)
	s.Close()
	first := read()
	if first == "" {
		t.Fatal("the first close wrote nothing")
	}

	s.Close()
	if after := read(); after != first {
		t.Errorf("a second close wrote %q, want nothing beyond %q", after, first)
	}
}

// A frame with no rows leaves the caret where it is. There is no last row to
// place it against, and a guessed row would put it somewhere the reader does
// not expect, so no positioning sequence is written at all.
func TestDrawLeavesTheCaretOnAnEmptyFrame(t *testing.T) {
	s, read := captureScreen(t, false)
	s.height, s.width = 10, 40
	s.Draw(nil)
	out := read()

	// The rows below the frame are cleared, since a frame of no rows would
	// otherwise leave whatever an earlier frame had put there, and the cursor
	// is sent home rather than to a row and a column guessed at.
	if !strings.HasSuffix(out, seqHome) {
		t.Errorf("the frame did not end at the home position: %q", out)
	}
	if caretPlace(out) != "" {
		t.Errorf("an empty frame positioned the caret: %q", out)
	}
}

// A frame with rows puts the caret just past the prompt, so that the next
// character appears where it is being typed. The row below the prompt is drawn
// when the frame has not filled the height, and the caret must not land on it.
func TestDrawPlacesTheCaretOnThePrompt(t *testing.T) {
	for _, height := range []int{6, 10, 24, 40} {
		s, read := captureScreen(t, false)
		s.height, s.width = 40, 40
		lines := Render(Frame{Input: "hi"}, height, 40)
		s.Draw(lines)
		out := read()

		place := caretPlace(out)
		if place == "" {
			t.Fatalf("height=%d: the caret was not placed: %q", height, out)
		}
		row := promptRow(lines)
		if row < 0 {
			t.Fatalf("height=%d: the frame has no prompt: %q", height, lines)
		}
		if got := caretRow(place); got != row+1 {
			t.Errorf("height=%d: the caret is on row %d, want the prompt on row %d: %q",
				height, got, row+1, lines)
		}
		// The column is just past the marker and the text.
		if got := caretColumn(place); got != 5 {
			t.Errorf("height=%d: the caret is at column %d, want 5",
				height, got)
		}
	}
}

// caretPlace returns the positioning sequence Draw ended with, or an empty
// string when the cursor was not placed against a row.
func caretPlace(out string) string {
	at := strings.LastIndex(out, "\x1b[")
	if at < 0 {
		return ""
	}
	seq := out[at:]
	if strings.HasSuffix(seq, "H") && strings.Count(seq, ";") == 1 {
		return seq
	}
	return ""
}

// caretRow returns the row of a positioning sequence, counting from one.
func caretRow(seq string) int {
	body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "H")
	row, _, _ := strings.Cut(body, ";")
	return digits(row)
}

// caretColumn returns the column of a positioning sequence.
func caretColumn(seq string) int {
	body := strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "H")
	_, col, _ := strings.Cut(body, ";")
	return digits(col)
}

// digits reads a run of figures as a number.
func digits(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
