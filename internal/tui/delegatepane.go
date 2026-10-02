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

// delegatePaneIndex is the index the delegate pane has in the pane set. It is
// pane 1, directly after the main conversation.
const delegatePaneIndex = 1

// delegatePaneName is the name the delegate pane carries in the pane set.
const delegatePaneName = "delegate"

// registerPanes puts the delegate pane in the pane set when it is not there
// yet. The caller holds mu. It is done on first use rather than at
// construction, so that a Session built without a constructor still has both
// panes to move between.
func (s *Session) registerPanes() {
	for s.panes.Len() <= delegatePaneIndex {
		s.panes.Add(delegatePaneName)
	}
}

// showDelegatePane chooses which pane is drawn: the delegate pane when on is
// true and the main conversation otherwise. The pane set holds the choice, so
// this and the next and previous keys cannot disagree about which pane is shown.
func (s *Session) showDelegatePane(on bool) {
	s.mu.Lock()
	s.registerPanes()
	if on {
		s.panes.Select(delegatePaneIndex)
	} else {
		s.panes.Select(mainPane)
	}
	s.mu.Unlock()
	s.draw()
}

// stepPane shows the next pane for a delta of one and the previous for minus
// one, wrapping around, as tmux does for its next and previous window keys.
//
// The set is grown to the worker pane as well, since a reader stepping forward
// past the delegate pane should reach the worker output rather than wrap back
// to the conversation and never see it.
func (s *Session) stepPane(delta int) {
	s.mu.Lock()
	s.registerPanes()
	s.registerWorkerPane()
	s.panes.Step(delta)
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
	if s.panes.Current() != delegatePaneIndex {
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

// cmdPane chooses the pane that is shown: the conversation, the delegate
// output or the worker output.
//
// The three panes are one vocabulary rather than two commands, since a reader
// stepping between them with the next and previous keys should be able to name
// the one they want to land on.
func (s *Session) cmdPane(args []string) bool {
	switch strings.ToLower(strings.Join(args, " ")) {
	case "delegate", "delegates":
		s.showDelegatePane(true)
	case "spawn", "worker", "workers":
		s.showWorkerPane(true)
	case "main", "conversation", "":
		s.showDelegatePane(false)
	default:
		s.appendLines("usage: /pane [main|delegate|spawn]")
	}
	return false
}
