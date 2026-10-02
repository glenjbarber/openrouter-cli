package tui

import (
	"context"
	"os"
	"testing"
	"time"
)

// The reader's first key has to reach the question rather than the line.
//
// The editor owns the terminal while a line is being composed, so the input
// loop does not regain control until a key arrives. A key that is acted on by
// the editor before the question is offered it is lost to the question, and the
// reader sees their `y` appear in the input box and has to press enter and try
// again. This is driven through a pipe rather than by calling the question
// directly, since driving it directly is exactly what a test that passes
// against the broken arrangement would do.

func TestTheFirstKeyReachesTheQuestion(t *testing.T) {
	s, in := focusSession(t)

	// The editor reads, as the input loop does while a turn works. Without a
	// reader on the pipe the key sits in it and the hook is never called.
	go s.editor.ReadLine()

	// The question is opened from another goroutine, as a turn opens it.
	go s.Approve("go", []string{"build"}, s.tools.dir)
	waitFor(t, func() bool { return s.asking() }, "the question was never opened")

	// One key, as a reader pressing `y` once.
	in.Write([]byte("y"))

	waitFor(t, func() bool { return !s.asking() }, "the key did not answer the question")
}

// A key that answers the question must not also appear in the input, since a
// reader who pressed it to approve has not typed it.
func TestTheAnsweringKeyIsNotTypedIntoTheLine(t *testing.T) {
	s, in := focusSession(t)

	// One reader for the whole test. A second call to ReadLine would be a
	// second reader on one pipe, and the two would race for every key.
	done := make(chan string, 1)
	go func() {
		line, _ := s.editor.ReadLine()
		done <- line
	}()

	go s.Approve("go", []string{"build"}, s.tools.dir)
	waitFor(t, func() bool { return s.asking() }, "the question was never opened")

	in.Write([]byte("y"))
	waitFor(t, func() bool { return !s.asking() }, "the key did not answer the question")

	// A key after the question has closed is ordinary input, so it goes into
	// the line and the Enter ends it. A line carrying the answer would be sent
	// to the model, which is what the reader would be shown as having typed.
	in.Write([]byte("z\r"))

	select {
	case line := <-done:
		if line != "z" {
			t.Errorf("the line came back as %q, want \"z\"", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the editor stopped reading after the question")
	}
}

// A key that is not an answer refuses the question rather than being typed.
func TestAnyOtherKeyRefuses(t *testing.T) {
	s, in := focusSession(t)

	go s.editor.ReadLine()

	go s.Approve("go", []string{"build"}, s.tools.dir)
	waitFor(t, func() bool { return s.asking() }, "the question was never opened")

	in.Write([]byte("x"))

	waitFor(t, func() bool { return !s.asking() }, "the key did not close the question")
}

// A key pressed while no question is open is ordinary input, and is not
// swallowed by a hook that has nothing to do with it.
func TestAKeyIsUntouchedWithNoQuestionOpen(t *testing.T) {
	s, in := focusSession(t)

	done := make(chan string, 1)
	go func() {
		line, _ := s.editor.ReadLine()
		done <- line
	}()

	// The Enter ends the line. ReadLine returns on a submit rather than at the
	// end of the text, so a write without one would leave it reading.
	in.Write([]byte("hello\r"))

	select {
	case line := <-done:
		if line != "hello" {
			t.Errorf("the line came back as %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the editor did not read the line")
	}
}

// focusSession builds a session whose editor reads from a pipe, so a test can
// type at it the way a reader types at a terminal.
func focusSession(t *testing.T) (*Session, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("making the pipe: %v", err)
	}
	t.Cleanup(func() { w.Close() })

	// The screen writes into a file, since a question is painted at once and
	// there is nothing to paint it on without one.
	sc, _ := screenCapture(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	conv := NewConversation()
	s := &Session{
		conv:      conv,
		mainConv:  conv,
		screen:    sc,
		ctx:       ctx,
		cancel:    cancel,
		editor:    NewLineEditor(r),
		approvals: newApprovalState(),
		answered:  make(chan bool, 1),
		spinner:   NewSpinner(),
		windows:   newContextLength(),
		tools:     &toolSet{dir: t.TempDir()},
		verbosity: DefaultVerbosity,
	}
	// The hook is what Start assigns. It is set here rather than through Start
	// so that the test drives the editor and the question directly.
	s.editor.OnAsk = s.takeAsk
	return s, w
}
