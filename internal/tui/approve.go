package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/config"
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
	// mode is what happens to a call that is not settled by a rule in the file
	// or by an answer given earlier in the session.
	mode approvalMode
}

// The modes the approval can be in.
//
// They are named for what they do to a question rather than for the reader who
// set them, since a reader setting a mode is answering for themselves and the
// model is what the mode governs.
type approvalMode int

const (
	// modeAsk puts every call to the reader.
	modeAsk approvalMode = iota
	// modeAllow approves without asking. A file rule still governs what may be
	// proposed at all, so this does not widen the allowlist.
	modeAllow
	// modeRefuse refuses every call without asking. A model told a program was
	// refused learns that, rather than being left to ask again.
	modeRefuse
)

// String renders the mode as the status bar and the command report it.
func (m approvalMode) String() string {
	switch m {
	case modeAllow:
		return "allow"
	case modeRefuse:
		return "refuse"
	default:
		return "ask"
	}
}

// newApprovalState returns the state a session begins with: asking about
// everything, permitting nothing and refusing nothing, so that the first call
// of each program is put to the reader.
func newApprovalState() *approvalState {
	return &approvalState{
		granted: make(map[string]bool),
		refused: make(map[string]bool),
		mode:    modeAsk,
	}
}

// setMode changes the mode and says what it changed.
//
// The remembered answers are cleared, since an answer given while asking is an
// answer to one question and a mode that stops asking makes them meaningless. A
// reader who moves from refusing to allowing would otherwise find every
// program they had once refused still refused.
func (a *approvalState) setMode(m approvalMode) {
	a.mode = m
	clear(a.granted)
	clear(a.refused)
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
	mode := s.approvals.mode
	s.mu.Unlock()
	if answered {
		return decided
	}

	// The mode settles the call before a question is put, so that a reader who
	// has said to stop being asked is not asked. The allowlist is not widened:
	// a program outside it is refused by the tool before the mode is reached,
	// so allowing a mode says nothing about what may be proposed.
	switch mode {
	case modeAllow:
		s.mu.Lock()
		s.approvals.record(command, true)
		s.mu.Unlock()
		return true
	case modeRefuse:
		return false
	}

	if err := s.putQuestion(command, args, dir); err != nil {
		// A question that could not be put is a refusal. A reader who was
		// never asked has agreed to nothing.
		return false
	}
	approved, ok := s.awaitAnswer()
	if !ok {
		// Nothing answered the question, so nothing approved it. A turn that
		// waited here for ever would be a turn the reader reads as a hang,
		// which is the one failure this path cannot have.
		//
		// The question is closed on the way out, so that the input loop does
		// not go on taking keys for a question that will never be answered.
		s.answerQuestion(false)
		return false
	}
	s.mu.Lock()
	s.approvals.record(command, approved)
	s.mu.Unlock()
	return approved
}

// awaitAnswer waits for the answer to the question being asked.
//
// The wait ends when the session ends as well as when the answer arrives,
// since a question left open at exit would otherwise strand the turn on a
// channel nobody is left to post to.
func (s *Session) awaitAnswer() (bool, bool) {
	select {
	case approved := <-s.answered:
		return approved, true
	case <-s.ctx.Done():
		return false, false
	}
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

// cmdApprove reports or changes the approval mode.
//
// The mode is a session preference and is not written to the configuration
// file. A file is somewhere a permission outlives the reading of it, and
// anything that would run every program without a question is not something to
// leave behind in a file that a later run opens without being told what it
// holds. A permission the reader means to keep is a rule under
// OPENROUTER_TOOLS, which is asked for rather than switched on.
//
// The mode is refused while a model is working, since a turn in flight holds the
// question it asked and changing the answer under it would settle a call the
// reader never saw.
func (s *Session) cmdApprove(args []string) bool {
	if s.working() {
		s.addReply("(/approve is refused while a model is working, since the turn in " +
			"flight is holding a question; wait for the answer and try again)")
		return false
	}
	if len(args) == 0 {
		s.appendLines(strings.Join(s.approvalListing(), "\n"))
		return false
	}

	want := strings.ToLower(strings.Join(args, " "))
	var mode approvalMode
	switch want {
	case "ask", "every", "every time":
		mode = modeAsk
	case "allow", "all", "yes", "approve":
		mode = modeAllow
	case "refuse", "no", "reject", "never":
		mode = modeRefuse
	default:
		s.addReply("(/approve takes ask, allow or refuse; not " + want + ")")
		return false
	}

	s.mu.Lock()
	s.approvals.setMode(mode)
	s.mu.Unlock()
	s.updateStatus()

	switch mode {
	case modeAllow:
		s.appendLines("approval is off: every program the model may run will run " +
			"without a question. It stays that way until /approve ask, or until " +
			"the session ends.")
	case modeRefuse:
		s.appendLines("approval is off: every program will be refused, and the " +
			"model is told so rather than left to ask again.")
	default:
		s.appendLines("approval is on: every program is put to you before it runs.")
	}
	return false
}

// approvalListing says what the shell will do with a call, and what the reader
// has already decided.
func (s *Session) approvalListing() []string {
	s.mu.Lock()
	mode := s.approvals.mode
	granted, refused := len(s.approvals.granted), len(s.approvals.refused)
	s.mu.Unlock()

	lines := []string{
		"approval: " + mode.String() + " (y approves once, a approves for the " +
			"session, anything else refuses; a mode of allow or refuse is asked " +
			"of nobody)",
	}
	if !s.tools.offersShell() {
		return lines
	}

	var rules []config.ApprovalRule
	if s.cfg != nil {
		rules = s.cfg.Tools
	}
	if permitted := config.PermittedCommands(rules, s.tools.dir); len(permitted) > 0 {
		lines = append(lines, "  permitted by the configuration file here: "+
			strings.Join(permitted, ", "))
	} else {
		lines = append(lines, "  no rule covers this directory, so the file "+
			"permits nothing on its own")
	}
	switch {
	case granted > 0:
		lines = append(lines, "  granted for this session: "+itoa(granted)+" programs")
	case refused > 0:
		lines = append(lines, "  refused for this session: "+itoa(refused)+" programs")
	}
	return lines
}
