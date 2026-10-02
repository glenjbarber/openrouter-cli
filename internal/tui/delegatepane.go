package tui

import "strings"

// delegatePane is the output pane for /delegate: the commands given and the
// answers that came back.
//
// It is kept apart from the main conversation so that a delegate does not
// interleave with the exchange the reader is having. The main pane is the
// frame the session already holds, and this is a second set of lines that the
// paint path swaps in while the pane is shown. Nothing here is recorded in
// either conversation, as with the delegate itself.
//
// Every field is guarded by Session.mu.
type delegatePane struct {
	// lines holds the commands and finished answers, oldest first.
	lines []string
	// partial is the answer still arriving, shown below the lines.
	partial string
	// shown reports that the pane is on screen in place of the conversation.
	shown bool
}

// delegateHint is shown while the pane is empty.
const delegateHint = "Nothing has been delegated yet. /delegate QUESTION asks " +
	"one, and the question and its answer are shown here."

// delegateTitle heads the pane while it is shown.
const delegateTitle = "/delegate"

// addDelegateLines appends to the delegate pane.
func (s *Session) addDelegateLines(lines ...string) {
	s.mu.Lock()
	s.dpane.lines = append(s.dpane.lines, lines...)
	s.mu.Unlock()
}

// setDelegatePartial replaces the answer that is still arriving.
func (s *Session) setDelegatePartial(text string) {
	s.mu.Lock()
	s.dpane.partial = text
	s.mu.Unlock()
}

// showDelegatePane chooses which pane is drawn. It is a method rather than a
// key binding so that whatever later switches panes can call it.
func (s *Session) showDelegatePane(on bool) {
	s.mu.Lock()
	s.dpane.shown = on
	s.mu.Unlock()
	s.draw()
}

// applyDelegatePane turns a copy of the frame into the delegate pane when it is
// shown. The caller holds mu and passes the copy it is about to render.
//
// The lines are copied so the renderer, which runs after the lock is released,
// never reads a slice a delegate goroutine is appending to. The spinner and
// the partial main reply are cleared, since they describe the main conversation.
func (s *Session) applyDelegatePane(f *Frame) {
	if !s.dpane.shown {
		return
	}
	f.Title = delegateTitle
	f.Reply = append([]string(nil), s.dpane.lines...)
	f.Delegate = s.dpane.partial
	f.Partial = ""
	f.Spinner = ""
	f.Elapsed = ""
	f.Tint = ""
	f.Hint = ""
	if len(f.Reply) == 0 && f.Delegate == "" {
		f.Hint = delegateHint
	}
}

// cmdPane chooses the pane that is shown: "delegate" or "main".
func (s *Session) cmdPane(args []string) bool {
	switch strings.ToLower(strings.Join(args, " ")) {
	case "delegate", "delegates":
		s.showDelegatePane(true)
	case "main", "conversation", "":
		s.showDelegatePane(false)
	default:
		s.appendLines("usage: /pane [main|delegate]")
	}
	return false
}
