package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// The keys the approval question is answered with.
//
// Three rather than two, because answering yes is not the only safe thing to
// do with a question the reader did not expect: once is the answer a reader
// who meant this one command gives, and it is the one a model asking the same
// thing in every round should not be able to obtain by accident.
const (
	keyApproveOnce = 'y'
	keyApproveAll  = 'a'
	keyRefuse      = 'n'
)

// approvalState is what the session remembers about one program.
//
// A grant is remembered so that a model asking for the same build in six
// rounds is asked once rather than six times, and a refusal is remembered on
// the same terms, since a model retrying a denied command would otherwise be
// able to wear the reader down by asking again. Both are held per session and
// neither is written to the configuration file: a grant given at the keyboard
// is a statement about this session, and a file is somewhere it would outlive
// it.
//
// It is guarded by the session mutex, since the question is asked from the
// turn goroutine rather than from the input one.
type approvalState struct {
	// granted holds the programs the reader approved for the session.
	granted map[string]bool
	// refused holds the programs the reader refused for the session.
	refused map[string]bool
}

// newApprovalState returns the state a session begins with, permitting nothing
// and refusing nothing, so that the first call of each program is asked about.
func newApprovalState() *approvalState {
	return &approvalState{
		granted: make(map[string]bool),
		refused: make(map[string]bool),
	}
}

// remembered reports what the reader already decided about a program.
//
// The grant is tested first, so that a grant given later in the session wins
// over a refusal given earlier: a reader who says no and then changes their
// mind has changed it, and a tool asking about a program they have since
// approved should not be refused on the strength of the earlier answer.
func (a *approvalState) remembered(command string) (approved, answered bool) {
	if a.granted[command] {
		return true, true
	}
	if a.refused[command] {
		return false, true
	}
	return false, false
}

// record keeps what the reader decided about a program for the rest of the
// session. The other map is cleared so that the two cannot both hold the same
// program and disagree about it.
func (a *approvalState) record(command string, approved bool) {
	if approved {
		a.granted[command] = true
		delete(a.refused, command)
		return
	}
	a.refused[command] = true
	delete(a.granted, command)
}

// Approve implements tools.Approver for a session, which is how a tool asks
// the reader whether a program may run.
//
// The two sources of permission are kept apart. The configuration file settles
// it, because that is where the reader wrote down what they meant. The prompt
// settles it for this session, because that is the only place a reader can be
// asked at all. A program settled by either is not asked about again.
func (s *Session) Approve(command string, args []string, dir string) bool {
	// A rule in the file is the reader having already answered, so it is
	// consulted before the prompt and without troubling anybody with it. It
	// is checked against the directory the command would run in rather than
	// the one the session was opened in, so that a rule written for the
	// project covers a build run in a subdirectory of it.
	if s.cfg != nil && s.cfg.Permits(command, dir) {
		return true
	}

	s.mu.Lock()
	decided, answered := s.approvals.remembered(command)
	s.mu.Unlock()
	if answered {
		return decided
	}

	if err := s.putQuestion(command, args, dir); err != nil {
		// A question that could not be put is a refusal. A reader who was
		// never asked has agreed to nothing.
		return false
	}
	approved := <-s.answered
	s.mu.Lock()
	s.approvals.record(command, approved)
	s.mu.Unlock()
	return approved
}

// putQuestion draws the question and waits for it to be answered.
//
// The question is put from the turn goroutine and answered from the input one,
// which is the whole difficulty in it. A turn runs on its own goroutine so that
// the input loop keeps reading while a model works, and the input loop owns the
// terminal. A question read from the turn goroutine would race the line editor
// for every key the reader pressed, and a `y` intended as an answer would be
// taken as part of a message being composed.
//
// So the turn hands the question over and waits for the answer. The answer is
// buffered, so the input goroutine never blocks on the turn having arrived to
// receive it, which it may not have when the keys are read.
func (s *Session) putQuestion(command string, args []string, dir string) error {
	line := tools.Describe(command, args)
	where := dir
	if rel, err := filepath.Rel(s.tools.dir, dir); err == nil && rel != "." &&
		!strings.HasPrefix(rel, "..") {
		where = filepath.Join(s.tools.dir, rel)
	}

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return errors.New("the session is closing")
	}
	if s.asked != nil {
		// A second question while one is open would have nowhere to put its
		// answer, and the first would never be answered.
		s.mu.Unlock()
		return errors.New("a question is already open")
	}
	s.asked = &question{
		line:     line,
		dir:      where,
		answered: s.answered,
	}
	// The question goes on the input row rather than into the pane. A question
	// written into the pane scrolls back into the history the moment a reply
	// arrives, which is about the moment a reader answering it would need to
	// read it again. The pane is the conversation; the input block is the
	// reader's own side of the screen.
	s.frame.Confirm = fmt.Sprintf("run %s in %s?", line, where)
	s.mu.Unlock()
	s.draw()

	// The turn can be stopped while the question is open. The refusal is
	// posted so that the input loop stops reading keys for a question the
	// reader has already walked away from.
	go func() { <-s.ctx.Done(); s.answerQuestion(false) }()

	return nil
}

// answerQuestion posts an answer to the question being asked, and does nothing
// where none is open.
//
// It is called with the lock released, since the input loop draws after it and
// a repaint inside the lock would hold the spinner off.
func (s *Session) answerQuestion(approved bool) {
	s.mu.Lock()
	q := s.asked
	s.asked = nil
	if q != nil {
		s.frame.Confirm = ""
		s.frame.ConfirmChoice = ""
	}
	s.mu.Unlock()
	if q == nil {
		return
	}
	q.answered <- approved
	// The row is repainted with the answer on it, so a reader who answered a
	// question can see that they did rather than watching the row vanish.
	s.draw()
}

// question is one approval waiting for an answer.
type question struct {
	// line is the command as it will be run, in the words the reader approves.
	line string
	// dir is the directory it will run in, resolved.
	dir string
	// answered receives the answer.
	answered chan bool
}

// asking reports whether a question is open.
func (s *Session) asking() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.asked != nil
}

// questionKey reads one key for the question being asked.
//
// The keys are read on the input goroutine, which is the one that owns the
// terminal. The answer is posted and the question closed, so the input loop
// returns to composing a message on its next round.
func (s *Session) questionKey() {
	s.mu.Lock()
	q := s.asked
	s.mu.Unlock()
	if q == nil {
		return
	}

	b, err := s.editor.ReadByte()
	if err != nil {
		// A reader who interrupted, or whose input ended, approved nothing.
		s.answerQuestion(false)
		return
	}
	switch b {
	case keyApproveOnce, keyApproveOnceUpper, keyApproveAll, keyApproveAllUpper:
		s.answerQuestion(true)
	default:
		// Anything else is a refusal, including escape. A reader who did not
		// mean to answer must not approve a program by pressing a key they
		// pressed for some other reason.
		s.answerQuestion(false)
	}
}

// The upper case spellings, so that a reader answering with a capital is not
// refused for the casing of the key they pressed.
const (
	keyApproveOnceUpper = 'Y'
	keyApproveAllUpper  = 'A'
)
