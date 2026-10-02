package tui

import (
	"fmt"

	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// workerPane is the output pane for /spawn: the questions given and what came
// back, with one line per tool call.
//
// It is a pane of its own rather than a second set of lines in the delegate
// pane, since a worker and a delegate are not the same thing to a reader. A
// delegate asks and answers in words. A worker acts on the host, and the calls
// it makes are the thing worth watching while they happen.
//
// Every field is guarded by Session.mu.
type workerPane struct {
	// lines holds the questions, the calls and the finished answers, oldest
	// first.
	lines []string
	// partial is the answer still arriving, shown below the lines.
	partial string
}

// workerHint is shown while the pane is empty.
const workerHint = "Nothing has been spawned yet. /spawn QUESTION starts a " +
	"worker that is given the tools, asks you before running a program, and " +
	"logs what it did."

// workerTitle heads the pane while it is shown.
const workerTitle = "/spawn"

// workerPaneIndex is the index the worker pane has in the pane set. It is pane
// 2, after the main conversation and the delegate pane.
const workerPaneIndex = 2

// workerPaneName is the name the worker pane carries in the pane set.
const workerPaneName = "spawn"

// addWorkerLines appends to the worker pane.
func (s *Session) addWorkerLines(lines ...string) {
	s.mu.Lock()
	s.wpane.lines = append(s.wpane.lines, lines...)
	s.mu.Unlock()
}

// setWorkerPartial replaces the answer that is still arriving.
func (s *Session) setWorkerPartial(text string) {
	s.mu.Lock()
	s.wpane.partial = text
	s.mu.Unlock()
}

// drawWorkerCall reports one call the worker made, in the same words a turn
// uses for its own.
//
// A worker that reads a large file would bury the pane under it, so the result
// is reported by its size rather than drawn, and a failure is drawn as the
// reason on the same line. The reader gets what happened and how much of it
// there was; the worker gets the whole of it, which is what it needs to carry
// on.
func (s *Session) drawWorkerCall(r tools.Result) {
	name := r.Call.Function.Name
	label := s.tools.label(name)
	if label == "" {
		// A call the session cannot account for is marked rather than given
		// no label, on the same reasoning as the turn path.
		label = "?"
	}
	s.addWorkerLines(fmt.Sprintf("[%s] %s %s -> %s", label, name,
		callSummary(r.Call), toolOutcome(r)))
}

// registerWorkerPane puts the worker pane in the pane set when it is not there
// yet. The caller holds mu.
//
// It fills any gap rather than only appending, since the worker pane is pane 2
// and the delegate pane is pane 1, and a set holding only pane 0 would
// otherwise leave the index pointing past its end.
func (s *Session) registerWorkerPane() {
	for s.panes.Len() <= workerPaneIndex {
		s.panes.Add(workerPaneName)
	}
}

// showWorkerPane chooses which pane is drawn: the worker pane when on is true
// and the main conversation otherwise.
func (s *Session) showWorkerPane(on bool) {
	s.mu.Lock()
	s.registerPanes()
	s.registerWorkerPane()
	if on {
		s.panes.Select(workerPaneIndex)
	} else {
		s.panes.Select(mainPane)
	}
	s.mu.Unlock()
	s.draw()
}

// applyWorkerPane turns a copy of the frame into the worker pane when it is
// shown. The caller holds mu and passes the copy it is about to render.
//
// The lines are copied so the renderer, which runs after the lock is released,
// never reads a slice a worker goroutine is appending to. The spinner and the
// partial main reply are cleared, since they describe the main conversation.
func (s *Session) applyWorkerPane(f *Frame) {
	if s.panes.Current() != workerPaneIndex {
		return
	}
	f.Title = workerTitle
	f.Reply = append([]string(nil), s.wpane.lines...)
	f.Delegate = s.wpane.partial
	f.Partial = ""
	f.Spinner = ""
	f.Elapsed = ""
	f.Tint = ""
	f.Hint = ""
	if len(f.Reply) == 0 && f.Delegate == "" {
		f.Hint = workerHint
	}
}
