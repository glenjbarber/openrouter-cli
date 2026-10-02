package tui

import (
	"io"
	"testing"
	"time"
)

// strandedWait bounds how long a test waits for a key to be acted on. A key
// that is acted on does so at once, so the wait only ever runs out when the
// key is stranded.
const strandedWait = 500 * time.Millisecond

// A terminal writes one key as one block and then writes nothing until the
// next key. These tests feed the editor that way, through a pipe that blocks
// the way a terminal does, rather than through a reader that ends. A reader
// that ends makes the editor take its end-of-input path, which drains the key
// queue, and that hides a key left in the queue while the editor waits.

// A shifted arrow alone in its block is handed to OnKey without another key
// having to arrive first.
func TestALoneShiftedArrowIsActedOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  string
		want byte
	}{
		{"shift up", "\x1b[1;2A", keyPageUp},
		{"shift down", "\x1b[1;2B", keyPageDown},
	} {
		pr, pw := io.Pipe()
		le := NewLineEditor(pr)
		keys := make(chan byte, 4)
		le.OnKey = func(k byte) { keys <- k }
		go le.ReadLine()

		if _, err := pw.Write([]byte(tc.seq)); err != nil {
			t.Fatalf("%s: write: %v", tc.name, err)
		}
		select {
		case got := <-keys:
			if got != tc.want {
				t.Errorf("%s: OnKey got %d, want %d", tc.name, got, tc.want)
			}
		case <-time.After(strandedWait):
			t.Errorf("%s: OnKey was not called, so the key is waiting for another", tc.name)
		}
		pw.Close()
	}
}

// A plain arrow alone in its block recalls history without another key having
// to arrive first, and is not typed into the line as its final byte.
func TestALoneUpArrowRecallsHistory(t *testing.T) {
	pr, pw := io.Pipe()
	le := NewLineEditor(pr)
	le.remember("old line")
	changed := make(chan string, 4)
	le.OnChange = func(line string) { changed <- line }
	result := make(chan string, 1)
	go func() {
		line, _ := le.ReadLine()
		result <- line
	}()

	if _, err := pw.Write([]byte("\x1b[A")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-changed:
		if got != "old line" {
			t.Errorf("recalled %q, want %q", got, "old line")
		}
	case <-time.After(strandedWait):
		t.Fatalf("nothing was recalled, so the key is waiting for another")
	}

	if _, err := pw.Write([]byte("\r")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case got := <-result:
		if got != "old line" {
			t.Errorf("line was %q, want %q", got, "old line")
		}
	case <-time.After(strandedWait):
		t.Errorf("the line was not submitted")
	}
	pw.Close()
}

// A key typed after a shifted arrow is not what releases it, and is not
// changed by it either: the arrow is acted on once and the text is the text.
func TestAKeyAfterAShiftedArrowIsJustTheKey(t *testing.T) {
	pr, pw := io.Pipe()
	le := NewLineEditor(pr)
	keys := make(chan byte, 4)
	le.OnKey = func(k byte) { keys <- k }
	result := make(chan string, 1)
	go func() {
		line, _ := le.ReadLine()
		result <- line
	}()

	pw.Write([]byte("\x1b[1;2A"))
	select {
	case <-keys:
	case <-time.After(strandedWait):
		t.Fatalf("OnKey was not called before the next key")
	}
	pw.Write([]byte("x\r"))
	select {
	case got := <-result:
		if got != "x" {
			t.Errorf("line was %q, want %q", got, "x")
		}
	case <-time.After(strandedWait):
		t.Errorf("the line was not submitted")
	}
	pw.Close()
}
