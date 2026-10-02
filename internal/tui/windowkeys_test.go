package tui

import (
	"io"
	"testing"
	"time"
)

// feed writes one key at a time with a short wait between, the way a terminal
// delivers them, so the editor blocks on the pipe between the prefix and the
// key after it.
func feed(t *testing.T, pw *io.PipeWriter, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, err := pw.Write([]byte(k)); err != nil {
			t.Fatalf("write: %v", err)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// The prefix alone must not act, and the key after it, arriving in a separate
// block, is what acts.
func TestWindowPrefixThenKeyActsAcrossBlocks(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want byte
	}{
		{";", keyWindowNext},
		{"j", keyWindowPrev},
	} {
		pr, pw := io.Pipe()
		le := NewLineEditor(pr)
		keys := make(chan byte, 4)
		le.OnKey = func(k byte) { keys <- k }
		go le.ReadLine()

		feed(t, pw, "\x02")
		select {
		case k := <-keys:
			t.Fatalf("the prefix alone acted: %d", k)
		default:
		}
		feed(t, pw, tc.key)
		select {
		case got := <-keys:
			if got != tc.want {
				t.Errorf("prefix %s: OnKey got %d, want %d", tc.key, got, tc.want)
			}
		case <-time.After(strandedWait):
			t.Errorf("prefix %s: OnKey was not called", tc.key)
		}
		pw.Close()
	}
}

// Both keys in one block act as well.
func TestWindowPrefixThenKeyInOneBlock(t *testing.T) {
	pr, pw := io.Pipe()
	le := NewLineEditor(pr)
	keys := make(chan byte, 4)
	le.OnKey = func(k byte) { keys <- k }
	go le.ReadLine()
	feed(t, pw, "\x02;")
	select {
	case got := <-keys:
		if got != keyWindowNext {
			t.Errorf("got %d, want %d", got, keyWindowNext)
		}
	case <-time.After(strandedWait):
		t.Error("OnKey was not called")
	}
	pw.Close()
}

// An unknown key after the prefix, including enter, is consumed: nothing is
// typed, no line ends, and OnKey is not called. The line goes on afterwards.
func TestWindowPrefixThenUnknownKeyIsConsumed(t *testing.T) {
	for _, k := range []string{"x", "n", "p", "\r", "\x1b[A", "\x02"} {
		pr, pw := io.Pipe()
		le := NewLineEditor(pr)
		le.remember("old")
		keys := make(chan byte, 4)
		le.OnKey = func(b byte) { keys <- b }
		result := make(chan string, 1)
		go func() {
			line, _ := le.ReadLine()
			result <- line
		}()

		feed(t, pw, "a", "\x02", k)
		select {
		case line := <-result:
			t.Fatalf("key %q after the prefix ended the line with %q", k, line)
		default:
		}
		select {
		case b := <-keys:
			t.Fatalf("key %q after the prefix called OnKey with %d", k, b)
		default:
		}
		// The prefix is spent: a plain n is typed, and the line is just "an".
		feed(t, pw, "n", "\r")
		select {
		case line := <-result:
			if line != "an" {
				t.Errorf("key %q: line was %q, want %q", k, line, "an")
			}
		case <-time.After(strandedWait):
			t.Errorf("key %q: the line was not submitted", k)
		}
		pw.Close()
	}
}

// Without a prefix, a semicolon and j are text.
func TestWindowKeysWithoutPrefixAreText(t *testing.T) {
	pr, pw := io.Pipe()
	le := NewLineEditor(pr)
	keys := make(chan byte, 4)
	le.OnKey = func(b byte) { keys <- b }
	result := make(chan string, 1)
	go func() {
		line, _ := le.ReadLine()
		result <- line
	}()
	feed(t, pw, ";", "j", "\r")
	select {
	case line := <-result:
		if line != ";j" {
			t.Errorf("line was %q, want ;j", line)
		}
	case <-time.After(strandedWait):
		t.Error("the line was not submitted")
	}
	if len(keys) != 0 {
		t.Error("OnKey was called for plain text")
	}
	pw.Close()
}

// The pane set wraps in both directions.
func TestPaneStepWraps(t *testing.T) {
	var p paneSet
	p.Add("two")
	p.Add("three")
	if got := p.Step(1); got != 1 {
		t.Errorf("next = %d, want 1", got)
	}
	p.Step(1)
	if got := p.Step(1); got != 0 {
		t.Errorf("next at the end = %d, want 0", got)
	}
	if got := p.Step(-1); got != 2 {
		t.Errorf("previous at the start = %d, want 2", got)
	}
	var one paneSet
	if got := one.Step(1); got != 0 {
		t.Errorf("one pane: next = %d, want 0", got)
	}
}

// The session moves between main and delegate with the keys, and /pane and
// the keys share one state.
func TestSessionWindowKeysSwitchPanes(t *testing.T) {
	s := delegateSession(t, "http://127.0.0.1:1")
	s.editor = NewLineEditor(s.screen.in)
	s.editor.OnKey = s.handleKey
	cur := func() int {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.panes.Current()
	}
	s.editor.OnKey(keyWindowNext)
	if cur() != delegatePaneIndex {
		t.Errorf("next from main: pane %d", cur())
	}
	s.editor.OnKey(keyWindowNext)
	if cur() != mainPane {
		t.Errorf("next wraps to main: pane %d", cur())
	}
	s.editor.OnKey(keyWindowPrev)
	if cur() != delegatePaneIndex {
		t.Errorf("previous wraps to delegate: pane %d", cur())
	}
	s.cmdPane([]string{"main"})
	if cur() != mainPane {
		t.Errorf("/pane main after the keys: pane %d", cur())
	}
}
