package tui

import (
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

	ask := s.ask
	if ask == nil {
		// A session assembled without a seam has no reader to ask, and no
		// answer is not permission.
		return false
	}
	approved := ask(command, args, dir)
	s.mu.Lock()
	s.approvals.record(command, approved)
	s.mu.Unlock()
	return approved
}

// askApproval puts the question to the reader and waits for an answer.
//
// The question is drawn in the pane rather than on the prompt line, since the
// prompt line belongs to the message being composed and overwriting it would
// lose a half typed line to a question about something else. The keys are read
// one at a time from the editor, which is the same reader the search uses, so
// a question takes every key while it is open and cannot be answered by a
// stray Enter left over from the message that triggered it.
func (s *Session) askApproval(command string, args []string, dir string) bool {
	line := tools.Describe(command, args)
	where := dir
	if rel, err := filepath.Rel(s.tools.dir, dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		where = filepath.Join(s.tools.dir, rel)
	}

	s.addReply(fmt.Sprintf("[ask] run %s in %s?", line, where))
	s.addReply("     y once   a all this session   n no")
	s.draw()

	b, err := s.editor.ReadByte()
	if err != nil {
		// A question that could not be put is a refusal. A reader who was
		// never asked has agreed to nothing, and the alternative here would be
		// to run the program on the grounds that no answer came back.
		return false
	}
	switch b {
	case keyApproveOnce, keyApproveOnceUpper, keyApproveAll, keyApproveAllUpper:
		return true
	default:
		// Anything else is a refusal, including escape. A reader who did not
		// mean to answer must not approve a program by pressing a key they
		// pressed for some other reason.
		return false
	}
}

// The upper case spellings, so that a reader answering with a capital is not
// refused for the casing of the key they pressed.
const (
	keyApproveOnceUpper = 'Y'
	keyApproveAllUpper  = 'A'
)
