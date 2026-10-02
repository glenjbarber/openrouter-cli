package tui

import (
	"strings"
	"testing"
	"time"
)

// The delegate command, its notice and its answer land in the delegate pane
// and not in the main reply.
func TestDelegateOutputGoesToItsOwnPane(t *testing.T) {
	s, _ := auditSession(t, auditStream)
	s.conv.Record("a question", "an answer")
	s.addReply("main line")

	s.startDelegate("what next")
	deadline := time.Now().Add(10 * time.Second)
	for s.delegatePending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.delegatePending(); got != 0 {
		t.Fatalf("%d delegates still running", got)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if got := strings.Join(s.frame.Reply, "\n"); got != "main line" {
		t.Errorf("main reply = %q, want it untouched by the delegate", got)
	}
	if len(s.dpane.lines) < 3 || s.dpane.lines[0] != "/delegate what next" {
		t.Errorf("delegate pane = %q, want the command, the notice and an answer",
			s.dpane.lines)
	}
	if s.dpane.partial != "" {
		t.Errorf("partial = %q, want it cleared when the delegate finished",
			s.dpane.partial)
	}
}

// While the pane is not shown the frame is left alone.
func TestDelegatePaneHiddenLeavesTheFrame(t *testing.T) {
	s := &Session{}
	s.dpane.lines = []string{"/delegate q"}
	f := Frame{Title: "main", Reply: []string{"main line"}, Spinner: "x"}
	s.applyDelegatePane(&f)
	if f.Title != "main" || len(f.Reply) != 1 || f.Spinner != "x" {
		t.Errorf("frame = %+v, want it unchanged", f)
	}
}

// While shown, the pane replaces the conversation and carries none of its
// state, and the rendered rows hold the delegate text and not the main text.
func TestDelegatePaneShownReplacesTheFrame(t *testing.T) {
	s := &Session{}
	s.dpane = delegatePane{
		lines:   []string{"/delegate q", "the answer"},
		partial: "still arriving",
		shown:   true,
	}
	f := Frame{Title: "main", Reply: []string{"main line"}, Partial: "p",
		Spinner: "x", Elapsed: "1s"}
	s.applyDelegatePane(&f)

	if f.Title != delegateTitle || f.Partial != "" || f.Spinner != "" || f.Elapsed != "" {
		t.Errorf("frame = %+v, want the main state cleared", f)
	}
	text := strings.Join(Render(f, 24, 60), "\n")
	for _, want := range []string{"/delegate q", "the answer", "still arriving"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered pane lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "main line") {
		t.Errorf("rendered pane carries the main conversation:\n%s", text)
	}
}

// An empty delegate pane says what it is for.
func TestDelegatePaneEmptyShowsHint(t *testing.T) {
	s := &Session{}
	s.dpane.shown = true
	f := Frame{Reply: []string{"main line"}}
	s.applyDelegatePane(&f)
	if f.Hint != delegateHint || len(f.Reply) != 0 {
		t.Errorf("hint = %q reply = %q, want the empty-pane hint", f.Hint, f.Reply)
	}
}

// /pane chooses the pane, and refuses a name it does not know.
func TestPaneCommandChoosesThePane(t *testing.T) {
	s := delegateSession(t, "http://127.0.0.1:1")
	s.cmdPane([]string{"delegate"})
	if !s.dpane.shown {
		t.Error("/pane delegate did not show the delegate pane")
	}
	s.cmdPane([]string{"main"})
	if s.dpane.shown {
		t.Error("/pane main did not return to the conversation")
	}
	s.cmdPane([]string{"nonsense"})
	if len(s.frame.Reply) == 0 || !strings.Contains(s.frame.Reply[0], "usage") {
		t.Errorf("reply = %q, want a usage line", s.frame.Reply)
	}
}
