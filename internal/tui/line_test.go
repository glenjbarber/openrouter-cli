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
