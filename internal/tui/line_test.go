package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadLinePlain(t *testing.T) {
	le := NewLineEditor(strings.NewReader("hello\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestReadLineBackspace(t *testing.T) {
	// DEL and BS are both accepted as backspace, since terminals differ.
	for _, key := range []byte{keyBackspace, keyDelete} {
		le := NewLineEditor(bytes.NewReader([]byte{'a', 'b', key, 'c', '\r'}))
		got, err := le.ReadLine()
		if err != nil {
			t.Fatalf("ReadLine: %v", err)
		}
		if got != "ac" {
			t.Errorf("key %#x: got %q, want %q", key, got, "ac")
		}
	}
}

func TestReadLineCtrlUClearsAll(t *testing.T) {
	le := NewLineEditor(strings.NewReader("discard me\x15kept\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "kept" {
		t.Errorf("got %q, want %q", got, "kept")
	}
}

// Ctrl-W kills the word before the cursor, keeping everything before the space
// that precedes it, which is what a shell does.
func TestReadLineCtrlWClearsWord(t *testing.T) {
	le := newLineReader("one two\x17three\r")
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "onethree" {
		t.Errorf("got %q, want %q", got, "onethree")
	}
}

// A single word with no preceding space is cleared whole, since there is
// nothing before it to keep.
func TestReadLineCtrlWOnSingleWord(t *testing.T) {
	le := newLineReader("scratch\x17\r")
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// newLineReader is a short constructor for the literal-input cases below.
func newLineReader(s string) *LineEditor { return NewLineEditor(strings.NewReader(s)) }

func TestReadLineCtrlCInterrupts(t *testing.T) {
	le := NewLineEditor(strings.NewReader("abc\x03"))
	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Errorf("err = %v, want ErrInterrupt", err)
	}
}

func TestReadLineCtrlDOnEmptyEnds(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x04"))
	if _, err := le.ReadLine(); err != ErrEndOfInput {
		t.Errorf("err = %v, want ErrEndOfInput", err)
	}
}

// A multibyte rune arriving one byte at a time is held until the sequence is
// complete, so it is not committed as a replacement character.
func TestReadLineMultibyte(t *testing.T) {
	const want = "café"
	raw := []byte(want)
	le := NewLineEditor(bytes.NewReader(append(append([]byte{}, raw...), '\r')))

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Backspace removes a whole multibyte rune rather than half of one.
func TestReadLineBackspaceMultibyte(t *testing.T) {
	le := NewLineEditor(strings.NewReader("café\x7f\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "caf" {
		t.Errorf("got %q, want %q", got, "caf")
	}
}

func TestReadLineEndOfInput(t *testing.T) {
	le := NewLineEditor(strings.NewReader(""))
	if _, err := le.ReadLine(); err != ErrEndOfInput {
		t.Errorf("err = %v, want ErrEndOfInput", err)
	}
}

// An escape is treated as a key rather than as the start of a sequence, since
// no escape sequence is read as input here.
func TestReadLineEscapeInterrupts(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b"))
	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Errorf("err = %v, want ErrInterrupt", err)
	}
}

// The composed line must be reported as it is typed, since the terminal is in
// raw mode with echo disabled and nothing else would draw it.
func TestReadLineReportsEachKeystroke(t *testing.T) {
	var seen []string
	le := NewLineEditor(strings.NewReader("abc\r"))
	le.OnChange = func(line string) { seen = append(seen, line) }

	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("seen = %v, want one report per character", seen)
	}
	if seen[0] != "a" || seen[2] != "abc" {
		t.Errorf("seen = %v, want the growing line", seen)
	}
}

// Backspace must be reported too, or the line on screen keeps a character that
// was removed.
func TestReadLineReportsBackspace(t *testing.T) {
	var seen []string
	le := NewLineEditor(strings.NewReader("ab\x7f\r"))
	le.OnChange = func(line string) { seen = append(seen, line) }

	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	last := seen[len(seen)-1]
	if last != "a" {
		t.Errorf("last = %q, want the line after the backspace", last)
	}
}

func TestReadLineReportsCtrlU(t *testing.T) {
	var last string
	le := NewLineEditor(strings.NewReader("discard\x15\r"))
	le.OnChange = func(line string) { last = line }

	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if last != "" {
		t.Errorf("last = %q, want empty after Ctrl-U", last)
	}
}

// A multibyte character is reported only once it is whole, so a character is
// never shown half formed.
func TestReadLineReportsMultibyteWhole(t *testing.T) {
	var seen []string
	le := NewLineEditor(strings.NewReader("é\r"))
	le.OnChange = func(line string) { seen = append(seen, line) }

	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if len(seen) != 1 || seen[0] != "é" {
		t.Errorf("seen = %q, want one report of the whole rune", seen)
	}
}

// A nil callback must not panic, which is what a non-interactive reader uses.
func TestReadLineNilCallback(t *testing.T) {
	le := NewLineEditor(strings.NewReader("hi\r"))
	le.OnChange = nil

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

// The input row carries the composed line, so it must be visible in the frame.
func TestRenderShowsComposedInput(t *testing.T) {
	lines := Render(Frame{Input: "/connect"}, 8, 40)
	if lines[len(lines)-1] != "> /connect" {
		t.Errorf("last line = %q, want the composed line", lines[len(lines)-1])
	}
}

// Without a completer the key is still a tab, since a reader indenting a
// message may want it and nothing has been installed to take it.
func TestReadLineTabWithoutCompleter(t *testing.T) {
	le := newLineReader("a\tb\r")
	if le.OnTab != nil {
		t.Fatal("OnTab is set, so this is not the case under test")
	}
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "a\tb" {
		t.Errorf("got %q, want %q", got, "a\tb")
	}
}

// The completer owns the key when one is installed, and the line it returns
// replaces what was typed.
func TestReadLineTabCompletes(t *testing.T) {
	le := newLineReader("/comp\t\r")
	var asked string
	le.OnTab = func(line string) string {
		asked = line
		return "/compact"
	}
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if asked != "/comp" {
		t.Errorf("the completer was asked about %q, want %q", asked, "/comp")
	}
	if got != "/compact" {
		t.Errorf("got %q, want %q", got, "/compact")
	}
}

// A completer that chooses nothing leaves the line as it stands. The key is
// not turned into a tab, since the reader asked for a completion rather than
// for whitespace.
func TestReadLineTabLeavesLineWhenNothingChosen(t *testing.T) {
	le := newLineReader("/mo\t\r")
	le.OnTab = func(string) string { return "" }
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "/mo" {
		t.Errorf("got %q, want the line unchanged", got)
	}
}

// The line is reported after a Tab even when nothing was chosen, so that a
// report the completer wrote is drawn by the same path that draws any other
// keystroke.
func TestReadLineTabReportsTheLine(t *testing.T) {
	le := newLineReader("/mo\t\r")
	var seen []string
	le.OnChange = func(line string) { seen = append(seen, line) }
	le.OnTab = func(string) string { return "" }
	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("OnChange was never called, so a report would never be drawn")
	}
	if last := seen[len(seen)-1]; last != "/mo" {
		t.Errorf("last reported line = %q, want %q", last, "/mo")
	}
}

// A Tab pressed part way through a multibyte rune abandons it rather than
// committing half a character, since the reader asked for a word rather than
// for the rest of a character.
func TestReadLineTabAbandonsPartialRune(t *testing.T) {
	// The first byte of the two byte sequence for U+00E9, then a Tab.
	le := newLineReader("a\xC3\t\r")
	le.OnTab = func(line string) string { return line + "z" }
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if strings.ContainsRune(got, 0xFFFD) {
		t.Errorf("got %q, want no replacement character from an abandoned rune", got)
	}
	if got != "az" {
		t.Errorf("got %q, want %q", got, "az")
	}
}
