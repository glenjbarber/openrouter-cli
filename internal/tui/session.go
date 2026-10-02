package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/bootstrap"
	"github.com/glenjbarber/openrouter-cli/internal/complete"
	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Session is a running interface.
type Session struct {
	screen *Screen
	editor *LineEditor
	frame  Frame

	// approvals is what the reader has already decided about a program this
	// session, and is guarded by mu since the turn goroutine writes it and the
	// input goroutine reads it.
	approvals *approvalState
	// asked is the approval question waiting for an answer, nil where none is
	// open. It is guarded by mu, since the turn goroutine opens it and the
	// input goroutine closes it.
	asked *question
	// rules are the approval rules the session decides from, loaded at startup
	// and replaced by /permission. They are held apart from the configuration
	// because the configuration is read once and never written.
	rules []config.ApprovalRule
	// autosaveStop ends the autosave timer, and is nil while no timer runs. It
	// is guarded by mu, since the command starts and stops it and the command
	// runs on the input goroutine.
	autosaveStop chan struct{}
	// answered receives the answer to the question being asked. It is buffered
	// so that the input goroutine never blocks on the turn having arrived to
	// receive it, which it may not have when the keys are read.
	answered chan bool
	// ask puts a question to the reader and returns the answer. It is a field
	// rather than a call to askApproval so that the prompt has exactly one
	// seam: a test substitutes the keyboard, and the decision made around the
	// question is exercised without one.
	ask func(command string, args []string, dir string) bool
	// cfg is the configuration the session was started with, held for the
	// approval rules rather than for the credential. The credential was
	// copied into the client at startup and is not kept here, since a field
	// that held it would be a field a diagnostic could reach.
	cfg          *config.Config
	chosen       config.Chosen
	providerName string

	// hints is the state the hint row describes. It is guarded by mu and
	// written by the input goroutine at the points where the keys that do
	// something change, rather than being derived inside the paint path. The
	// editor belongs to the input goroutine and the line editor holds no
	// lock of its own, so reading the history from the spinner goroutine
	// while a key is being read would be a race.
	hints hintState

	// client is nil until a connection is established, so that the interface
	// can be used for composition before a key is configured.
	// mu guards the frame, which the spinner goroutine writes while the
	// request loop writes it too.
	mu     sync.Mutex
	client *openrouter.Client
	conv   *Conversation
	// spinner turns the twiddle while work is in progress.
	spinner *Spinner
	// completedPrefix is the token the held candidates were completed from,
	// and completedNames are those candidates with completedAt the one being
	// shown. They are guarded by mu, since completion runs on the input
	// goroutine and the frame is drawn from the paint path.
	completedPrefix string
	completedNames  []string
	completedAt     int
	// step is how many twiddle frames have been drawn, and is where the color
	// ramp is taken from. It is guarded by mu with the rest of the frame, since
	// it is written by the spinner goroutine and read by the paint path.
	step int
	// startedAt is when the work in progress began, and is where the elapsed
	// figure is measured from. It is zero while no work is running, which is
	// what keeps the figure off the row when there is nothing to measure.
	// It is guarded by mu with the rest of the frame, since the spinner
	// goroutine reads it on every frame.
	startedAt time.Time
	// panes is the numbered list of panes, with the main conversation as pane
	// 0 and the index of the one being shown. It is guarded by mu. The zero
	// value already holds pane 0, so a session assembled without it, as the
	// tests do, shows the main conversation.
	panes paneSet
	// scroll is how many lines the pane is scrolled up from the newest
	// output. Zero means the view is following the bottom. It is guarded by
	// mu rather than by a lock of its own, since it is read by the renderer
	// and written by the input goroutine, which is the same pair of jobs mu
	// already does for the frame.
	scroll int
	// thread is the ephemeral conversation, nil while the main one is in
	// force. The main conversation is held in mainConv throughout, so that
	// leaving a thread restores it without a snapshot being taken here.
	thread   *Ephemeral
	mainConv *Conversation
	// modelList is the catalogue being filtered, held while the filter is open.
	modelList []openrouter.Model
	// modelFilter is what has been typed to narrow it.
	modelFilter string
	// modelKeep narrows the catalogue before the filter is applied, which is
	// how the free-model listing is the same code as the full one.
	modelKeep func(openrouter.Model) bool
	// modelCycle is the set of identifiers the filter is being completed
	// through, and is nil while no completion is in progress. It is held
	// rather than recomputed, since a Tab advances through the set that the
	// filter matched when the cycle began rather than through whatever the
	// completed filter matches at the time. Every other change to the filter
	// drops it, so a cycle only ever survives the Tabs that began it.
	modelCycle []string
	// modelCycleAt is which of modelCycle the filter currently holds.
	modelCycleAt int
	// searchOpen reports whether the pane search is open. A flag is held rather
	// than inferred from the query, since an empty query is the state the search
	// starts in and would otherwise close it.
	searchOpen bool
	// search is what has been typed into the pane search.
	search string
	// searchReply is the pane as it stood when the search was opened. The
	// listing replaces it while the search is open, so a copy is kept to
	// restore rather than rebuilding from the conversation, which is folded
	// at render time and so cannot be turned back into lines here.
	searchReply []string
	// searchKinds is the record of each entry of searchReply, held aside with
	// it so that closing the search restores the color the entries had.
	searchKinds []entryKind
	// searchScroll is the offset the reader held before the search moved the
	// view. It is restored when the search closes, since a search that leaves
	// the reader somewhere else would move the view out from under them.
	searchScroll int
	// turn is the request in flight, nil while no model is working. It is
	// guarded by mu, since the input goroutine stops a turn the request
	// goroutine owns.
	turn *turnState
	// queued holds the lines committed while a model was working, which go
	// out when that request ends. It is guarded by mu and drawn above the
	// prompt, since a line the reader has sent and cannot see is a line they
	// would send again.
	queued []string
	// repaint serialises the writes to the terminal, so that two requests for
	// a repaint cannot interleave their output into the same row.
	repaint sync.Mutex
	// lastPaint is when the frame was last written to the terminal, used to
	// hold the rate down.
	lastPaint time.Time
	// paintPending records that a repaint was asked for during the interval and
	// is owed once it passes.
	paintPending bool
	// flush draws the deferred repaint once the interval has passed. It is the
	// owner of a deferred repaint, since nothing else is left to ask for it:
	// the work that wanted it may have finished and the twiddle with it, and
	// the next thing to happen would be the reader typing, which shows the
	// reply all at once rather than as it arrived. A nil value means nothing is
	// owed. It is guarded by repaint, as lastPaint and paintPending are.
	flush *time.Timer
	// closed records that the terminal has been restored, so that nothing is
	// drawn after the way out. It is guarded by repaint and is read while that
	// is held.
	closed bool
	// delegates are the background questions still running. They are tracked so
	// that leaving does not leave one writing to a frame nobody is drawing on,
	// and so that the count can be reported.
	delegates map[*Delegate]bool
	// dpane is the output pane for /delegate, guarded by mu. See delegatepane.go.
	dpane delegatePane
	// workers are the background workers still running, tracked apart from
	// the delegates since a worker holds a tool set and a delegate does not.
	workers map[*Spawn]bool
	// wpane is the output pane for /spawn, guarded by mu. See workerpane.go.
	wpane workerPane
	// workerWG counts the worker goroutines that are running, so that Close
	// waits for them rather than returning while one is still writing to the
	// frame and still holding its request open.
	workerWG sync.WaitGroup
	// delegateWG counts the delegate goroutines that are running, so that Close
	// waits for them rather than returning while one is still writing to the
	// frame and still holding its request open. A delegate is counted under mu
	// at the same time as the map is entered, which is what makes the wait
	// sound: a counter raised after Close began waiting is a counter nobody is
	// waiting for, so the flag below refuses one instead.
	delegateWG sync.WaitGroup
	// closing records that Close has begun. It is guarded by mu and is read
	// where a delegate is counted, so that a delegate is not started against a
	// session on its way out.
	closing bool
	// bellWanted reports that the terminal bell is rung when a reply arrives.
	// It is a preference read from the configuration and changed at runtime,
	// so that a user who did not ask for it never hears one.
	bellWanted bool
	// colorOn reports that color is drawn on the screen. It is off unless the
	// configuration asked for it, and /color changes it for the session only.
	colorOn bool
	// colorTheme is the base colors the configuration asks for, empty on a
	// side that follows the terminal theme.
	colorTheme config.Theme
	// out is where the bell is written, which is the interface output.
	out *os.File
	// cognito reports that this session records nothing. The mode is in force
	// because the user asked for it, or because it was left on by a crash,
	// which is reported at startup rather than honoured silently.
	cognito bool
	// verbosity is the level at which the model is asked to answer, from 0 to 6.
	// It is guarded by mu, since the request goroutine reads it to build a turn
	// and the command writes it from the input goroutine.
	verbosity int
	// verbose reports that the pane should describe the shape of each
	// streamed turn. It is a display mode rather than a recording one, so
	// nothing about the request or the conversation changes because of it.
	// It is guarded by mu, since the request goroutine reads it while the
	// input goroutine may turn it on.
	verbose bool
	// windows caches the context length of each model seen, so that the
	// threshold can be evaluated without a call per message.
	windows *contextLength
	// tools is what a request may offer the model, built once at startup from
	// the working directory. It is held rather than built per turn, since a
	// root is a descriptor opened once and a set built per turn would open one
	// per turn to contain the same directory. A session assembled without one
	// sends no tools, which is what a client built before this work did.
	tools *toolSet
	// completer completes the line being composed when Tab is pressed. It is
	// built from the command table, so the words it offers are the words the
	// dispatcher answers to rather than a list that could fall behind it.
	completer complete.Completer

	// ctx is cancelled when the session leaves, so that an in-flight request
	// is abandoned rather than left to finish against a terminal that is no
	// longer drawn on.
	ctx    context.Context
	cancel context.CancelFunc
}

// ErrQuit reports that the user asked to leave the session.
var ErrQuit = errors.New("quit")

// providerName is the service every request is sent to. It is a constant
// rather than a derived value, since the endpoint is the only provider the
// client speaks to.
const providerName = "openrouter.ai"

// The states shown in the status field. A state is a short word rather than a
// sentence, since the bar is read at a glance while a reply is arriving.
const (
	stateIdle    = "idle"
	stateWorking = "Working"
)

// Start opens the interface on the given files.
//
// A caller that redirects the output receives ErrNotTerminal and is expected to
// fall back to ordinary line-oriented output rather than having a frame
// written into the capture.
func Start(out, in *os.File, title string) (*Session, error) {
	screen, err := NewScreen(out, in)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		screen:    screen,
		editor:    NewLineEditor(in),
		conv:      NewConversation(),
		windows:   newContextLength(),
		spinner:   NewSpinner(),
		out:       out,
		ctx:       ctx,
		cancel:    cancel,
		completer: complete.New(candidates()),
		frame: Frame{
			Title: title,
			Status: Status{
				Provider: providerName,
				State:    stateIdle,
				Host:     hostname(),
			},
		},
	}
	// The tools are built before anything is asked of the model, and a set
	// that could not be built is reported in the pane rather than refused. A
	// reader whose working directory cannot be opened still has a working
	// chat client, in the manner of a session with no API key: the interface
	// opens and says what is missing, where refusing to open would leave
	// nothing on screen explaining it. The report is made once, at startup,
	// since repeating it on every turn would fill the pane with a line about
	// something that is not going to change.
	s.approvals = newApprovalState()
	s.answered = make(chan bool, 1)
	// The level a session starts at is the one the configuration file names,
	// which is a preference rather than a setting: it is about the conversation
	// rather than about the client. A session with no file, or one naming a
	// level that is not one, starts at the default.
	s.verbosity = clampVerbosity(cfgVerbosity(s.cfg))
	if chosen, err := config.LoadChosen(); err == nil {
		s.chosen = chosen
	}
	// The rules are read at startup rather than on each call, since they change
	// only through /permission and a call asking the filesystem what is
	// permitted is a call on the path of every program run.
	if rules, err := config.LoadRules(nil); err == nil {
		s.rules = rules
	}
	// The autosave timer runs for the whole session rather than being started
	// by the command, since a reader who asked for it once should not have to
	// ask again after a /new.
	s.setAutosave(true)
	s.tools = toolsAt(workingDir(), s)
	if absence := s.tools.absence(); absence != "" {
		s.addReply(absence)
	}
	// The terminal is in raw mode with echo disabled, so the composed line is
	// drawn by the interface rather than by the line discipline. Without this
	// the keystrokes are held until the line is submitted and appear to do
	// nothing at all.
	s.mainConv = s.conv
	s.editor.OnPaste = func(lines []string) {
		s.mu.Lock()
		s.frame.Pasted = lines
		s.mu.Unlock()
		s.draw()
	}
	s.editor.OnChange = func(line string) {
		s.mu.Lock()
		s.frame.Input = line
		// A notice is about a keystroke, so typing past it clears it. A
		// reader who has moved on from what the notice said should not have
		// to dismiss it, and one who has not will see it again on the next
		// completion.
		s.frame.Notice = ""
		// The held candidates are dropped too, since a prefix the reader has
		// typed past is not the one they were offered for.
		s.completedPrefix = ""
		s.completedNames = nil
		s.completedAt = 0
		s.mu.Unlock()
		s.draw()
	}
	// Tab completes the line rather than inserting a tab into it, since a
	// reader pressing it is asking what a word might be rather than asking
	// for whitespace. The editor holds no pane, so what to say about the
	// candidates is settled here and the editor is handed back the line to
	// compose.
	s.editor.OnTab = func(line string) string { return s.completeLine(line) }
	// A key arriving while a question is open is offered to the question
	// before the editor acts on it. The editor owns the terminal while a line
	// is being composed, so the loop below does not regain control until a key
	// arrives, and without this the key is taken as part of a message the
	// reader was not writing. It is the whole of the focus: the question is
	// drawn at once, and the key that answers it is routed here.
	s.editor.OnAsk = s.takeAsk
	// A shifted arrow pages the pane. It is handled here rather than in the
	// editor because a page is a screenful and only the session knows how tall
	// the pane is; the editor knows the composed line and nothing of the frame.
	//
	// Without this the keys are queued and nothing acts on them: the editor
	// hands a key it has no use for to OnKey, and a nil OnKey is a key that
	// does nothing at all.
	s.editor.OnKey = s.handleKey
	// The wheel is read on the same goroutine as the keys, since a report
	// arrives in the same stream. The callback moves the view and repaints,
	// which is what makes the scroll happen while the line is still being
	// composed rather than only after it is sent.
	s.editor.OnMouse = func(direction int) {
		s.scrollBy(direction)
	}
	return s, nil
}

// handleKey acts on a special key the line editor handed on: a page, or a move
// to the next or previous window.
func (s *Session) handleKey(final byte) {
	switch final {
	case keyPageUp:
		s.page(-1)
	case keyPageDown:
		s.page(1)
	case keyWindowNext:
		s.stepPane(1)
	case keyWindowPrev:
		s.stepPane(-1)
	}
}

// scrollBy moves the view one wheel notch in the given direction.
//
// The offset is held across a repaint deliberately: a new reply arriving while
// page moves the view by a screenful.
//
// A page is the height of the pane rather than a fixed count, since a fixed
// count is a page on one terminal and a third of one on another. It is a little
// under the height rather than all of it, so that the row the reader was reading
// before the page is still on screen afterwards, which is what makes a page a
// step rather than a jump.
//
// The offset is clamped by the renderer to leave a full pane of history, so a
// page past the top settles on the oldest lines rather than leaving one line at
// the top of an otherwise empty frame.
func (s *Session) page(direction int) {
	height, _ := s.screen.Size()
	rows := height - headerRowCount - inputRowsBare - 1
	if rows < 1 {
		rows = 1
	}

	// The offset is moved by the figure rather than a notch at a time, since a
	// notch is a scroll of the mouse and a page is a scroll of the keyboard and
	// neither is the other. Going down stops at the bottom rather than
	// clamping short of it, which is what a page down from part way through the
	// history is meant to do.
	s.mu.Lock()
	switch {
	case direction < 0:
		s.scroll += rows
	case s.scroll > rows:
		s.scroll -= rows
		if s.scroll < 0 {
			s.scroll = 0
		}
	default:
		s.scroll = 0
	}
	s.mu.Unlock()
	s.draw()
}

// the reader is scrolled back must not pull the view down, or the history
// they are reading moves under them.
func (s *Session) scrollBy(direction int) {
	s.mu.Lock()
	s.scroll = scrolledBy(s.scroll, direction)
	s.mu.Unlock()
	s.draw()
}

// scrollAtBottom reports whether the view is following the newest output.
func (s *Session) scrollAtBottom() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scroll == 0
}

// resetScroll returns the view to the newest output.
//
// It is called only where the history itself is discarded, since a view left
// scrolled back over a pane that no longer holds what it was showing would
// otherwise be showing an offset into nothing.
func (s *Session) resetScroll() {
	s.mu.Lock()
	s.scroll = 0
	s.mu.Unlock()
}

// setMouse turns mouse reporting on or off at the reader's request.
func (s *Session) setMouse(on bool) {
	s.screen.SetMouse(on)
	// The hint row names the wheel only while reporting is on, since the
	// wheel is what reporting drives. The bit is taken under the lock,
	// because the row is also drawn from the spinner goroutine.
	s.mu.Lock()
	s.hints.mouse = on
	s.mu.Unlock()
}

// SetMouse turns mouse reporting on or off.
//
// It is the way a session is started with reporting already on, for a reader
// who wants the wheel without running /mouse first. The choice is not made
// here, since capturing the mouse is what stops a drag from selecting text.
func (s *Session) SetMouse(on bool) {
	s.setMouse(on)
}

// Close restores the terminal.
func (s *Session) Close() {
	// The session is marked closed before anything is cancelled, so that a
	// repaint already in flight, a deferred one waiting on its timer, and the
	// twiddle turning at the time all find a terminal that has been put back
	// and draw nothing.
	s.repaint.Lock()
	s.closed = true
	s.stopFlush()
	s.repaint.Unlock()

	// The flag is raised before the wait rather than after it, since the wait is
	// only sound while nothing can raise the counter behind it.
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	// The twiddle is stopped here rather than left to the request that owns it,
	// since a request abandoned by the cancellation below may take a moment to
	// unwind and would keep the interface turning until it does.
	s.spinner.Stop()
	s.cancel()

	// The turn in flight is waited for on the same terms as a delegate. The
	// cancellation above reaches it, since a turn takes its context from the
	// session, and it unwinds by settling the pane, the counters and the
	// twiddle. Nothing is drawn, because closed is already set, but the
	// write is the reason to wait rather than to race.
	s.mu.Lock()
	turn := s.turn
	s.mu.Unlock()
	if turn != nil {
		<-turn.done
	}

	// The delegates are waited for before the terminal is put back. The
	// cancellation above reaches them, since they are made from the session
	// context, and each one unwinds by writing its answer to the frame and
	// drawing it. Nothing is drawn, because closed is already set, but the
	// write is the reason to wait rather than to race: leaving returns to the
	// shell and the goroutine is still running against a frame nothing will
	// read and a terminal that has been handed back.
	s.delegateWG.Wait()
	// The workers are waited for on the same terms as the delegates, and for
	// the same reason: a worker holds its own tool set and its own request, and
	// returning while one is still writing to the frame would leave a
	// goroutine writing to a terminal that has been handed back.
	s.workerWG.Wait()

	// Reporting is turned off before the terminal is restored, so that a
	// wheel notch is not delivered to a program that has stopped reading.
	if s.screen.Mouse() {
		s.screen.SetMouse(false)
	}
	s.screen.Close()
}

// addReply appends the given lines to the reply pane.
//
// The lock is taken because the paint path copies the frame while the twiddle
// turns and while a deferred repaint is drawn, and a copy taken while an
// append is in progress reads the slice header as it is being written. Every
// other writer of the frame takes the lock for the same reason.
//
// The lines are plain text. Nothing is drawn around them, since a selection
// out of the pane has to yield the text with no escape sequence in it.
func (s *Session) addReply(lines ...string) {
	if len(lines) == 0 {
		return
	}
	s.addReplyTagged(entryKind{}, lines...)
}

// clearReply empties the reply pane.
func (s *Session) clearReply() {
	s.mu.Lock()
	s.replaceReply(nil)
	s.mu.Unlock()
}

// appendLines adds text to the reply pane, one entry per line.
//
// The split happens here rather than at render time because a fenced code
// block spans lines. Splitting first leaves each fence marker on its own, so
// the renderer sees no block and folds code that must not be folded. The
// renderer folds what it is given, so what it is given has to carry the whole
// reply.
//
// The text is the client's own, a listing or a confirmation, and is recorded as
// a notice. Text a model wrote goes through appendModelText instead.
func (s *Session) appendLines(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	s.addReplyKind(kindNotice, text)
}

// appendModelText adds text a model wrote to the reply pane, recorded as a
// reply so that nothing about it is taken from what it says.
func (s *Session) appendModelText(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	s.addReplyKind(kindReply, text)
}

// Note adds a line to the reply pane, for a message the client generates such
// as a bootstrap confirmation. The line is shown inside the frame rather than
// written before it, so that it is not cleared by the first repaint.
func (s *Session) Note(format string, args ...any) {
	s.addReply(fmt.Sprintf(format, args...))
	s.draw()
}

// Configure installs the credential and endpoint for the session.
//
// The client is built here rather than at start, so that the interface works
// before a connection exists and reports a missing key as an ordinary message
// rather than refusing to open.
// Configure applies the configuration the session was started with.
//
// The whole configuration is taken rather than the three values the session
// needs to reach a model, because the approval rules sit in the same file and
// would otherwise have to be read again from somewhere else. A nil
// configuration leaves the session asking about everything, which is what a
// session assembled without one should do.
func (s *Session) Configure(cfg *config.Config) {
	s.cfg = cfg
	if cfg != nil {
		s.adoptConfiguredModel(cfg.Model)
	}
	if cfg == nil {
		s.updateStatus()
		return
	}
	// The model is adopted whether or not a credential is present, so that
	// the status bar reflects the file from the first repaint.
	if cfg.APIKey != "" {
		s.client = openrouter.New(cfg.URLBase, cfg.APIKey)
	}
	// The status bar is refreshed here as well as in the conversation, since
	// a model taken from the file must appear on the first repaint rather than
	// only after the first exchange.
	s.updateStatus()
}

// Seed loads a bootstrap document into the conversation.
//
// The document is handed in rather than named here, since it was read once at
// startup to be validated. Reading it a second time would let the document that
// was checked be a different one from the document that is seeded.
func (s *Session) Seed(doc *bootstrap.Document) {
	if doc == nil {
		return
	}
	if doc.Session != nil {
		s.loadSaved(doc.Session)
		return
	}
	s.conv.Seed(doc.Instructions)
	s.Note("bootstrap: %s (%s)", doc.Path, doc.Format)
}

// Run reads lines until the user leaves.
//
// A line beginning with a slash is treated as a command rather than as a
// prompt, so that the interface keeps its own vocabulary separate from the
// model input.
func (s *Session) Run() error {
	s.draw()
	for {
		if s.searching() {
			// The search takes every key while it is open, so that typing does
			// not reach the line editor behind it.
			s.searchKey()
			continue
		}
		if s.filtering() {
			// The filter takes every key while it is open, so that typing does
			// not reach the line editor behind it.
			s.filterKey()
			continue
		}
		if s.asking() {
			// A question takes every key while it is open, and it takes them
			// here rather than on the goroutine that asked it, since this is
			// the one that owns the terminal. A question read from the turn
			// would race the line editor for every key the reader pressed.
			s.questionKey()
			continue
		}

		line, err := s.editor.ReadLine()
		// The composed line is taken before the frame is cleared of it. The
		// editor reports the line after every keystroke, so the frame holds
		// what was in hand when the interrupt arrived. Clearing it first
		// left nothing to test, which made the test below always true and
		// ended the session on every interrupt.
		composed := s.frame.Input
		s.mu.Lock()
		s.frame.Input = ""
		s.frame.Pasted = nil
		s.mu.Unlock()
		switch {
		case errors.Is(err, ErrEndOfInput):
			return nil
		case errors.Is(err, ErrInterrupt):
			// An interrupt while a model is working stops it and sends what is
			// in hand, rather than leaving the session. The line is the update
			// the model was working from, so it is sent rather than abandoned.
			if s.working() {
				s.stopTurn(composed)
				s.draw()
				continue
			}
			if composed == "" {
				return ErrQuit
			}
			// An interrupt with text in hand abandons the line rather than
			// the session, which is what a shell does.
			s.draw()
			continue
		case err != nil:
			return err
		}

		// A submitted line is remembered by the editor, which is what gives the
		// up and down arrows an action. The bit is set from the goroutine that
		// owns the editor rather than read out of it at paint time, since the row
		// is also drawn from the spinner goroutine. It is only ever set: history
		// is not forgotten within a session, so no site has to clear it and
		// missing one cannot leave the row naming a key that does nothing.
		s.mu.Lock()
		s.hints.history = true
		s.mu.Unlock()

		// The view is not moved here. A reply arriving while the reader is
		// scrolled back must not yank the view down, or the history they
		// are reading moves under them, and the only thing that returns the
		// view to the newest output is scrolling down to it.
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "/"):
			if quit := s.command(trimmed); quit {
				return ErrQuit
			}
		case s.working():
			// A line sent while the model is working is held rather than
			// refused. Enter queues it and the model carries on, and the line
			// goes out as an update to the request if the reader stops it, or
			// as a question of its own once the request has been answered.
			s.queueLine(trimmed)
		default:
			s.startTurn(line, s.conv)
		}
		s.draw()
	}
}

// command is one entry in the interface vocabulary.
//
// The table is the single place the names are written. The dispatcher looks
// the typed name up in it, the help is rendered from it, and the completer
// offers it, so the three cannot drift apart as separate lists would.
type command struct {
	// names are the words that select the command. A command with a synonym
	// carries more than one, such as /quit and /exit, and the completer
	// offers each of them so a prefix such as /ex can be completed.
	names []string
	// usage is how the command appears in the help. It is empty unless the
	// help shows an argument hint beside the name, as in /model [NAME].
	usage string
	// description is the one-line summary shown in the help and beside a
	// candidate when a prefix matches more than one.
	description string
	// idleOnly refuses the command while a model is working, since it changes
	// the conversation the request in flight is holding. A turn records its
	// answer into the conversation it was asked in, so a conversation cleared
	// underneath one would collect an exchange nobody asked it to keep.
	idleOnly bool
	// run performs the command with the words that follow it, and reports
	// whether the session should end.
	run func(s *Session, args []string) bool
	// hidden are names that select the command but are neither listed in the
	// help nor offered by the completer. Only lookupCommand reads them, so a
	// spelling that is accepted when typed does not widen the vocabulary the
	// reader is shown.
	hidden []string
}

// commands is the interface vocabulary, in the order the help lists it.
//
// The order is the help order rather than an alphabetical one, since the help
// is where the list is read and the completer lists the matches in the order
// they are declared.
//
// The table is filled in init rather than in a variable initializer because
// /help renders the help from this same table. A variable initializer that
// reached cmdHelp would reach helpText and back to this variable, which Go
// reports as an initialization cycle. init breaks the cycle without resorting
// to a second copy of the names, which is the thing the table exists to
// prevent.
var commands []command

func init() {
	commands = []command{
		{names: []string{"/help"}, description: "this list", run: (*Session).cmdHelp},
		{names: []string{"/connect"}, description: "test the connection and report the key", run: (*Session).cmdConnect},
		{names: []string{"/key"}, description: "report the usage against the key", run: (*Session).cmdKey},
		{names: []string{"/search"}, description: "search the pane, filtered as it is typed", run: (*Session).cmdSearch},
		{names: []string{"/models"}, description: "list the models, filtered as it is typed", run: (*Session).cmdModels},
		{names: []string{"/freemodels"}, description: "list the models that cost nothing, filtered as typed", run: (*Session).cmdFreeModels},
		{names: []string{"/model"}, usage: "/model [NAME]", description: "show or choose the model, without an argument to list", run: (*Session).cmdModel},
		{names: []string{"/new"}, description: "clear the conversation", run: (*Session).cmdNew, idleOnly: true},
		{names: []string{"/bell"}, description: "ring the terminal bell on reply, on or off", run: (*Session).cmdBell},
		{names: []string{"/color"}, hidden: []string{"/colour"}, usage: "/color [on|off]", description: "turn color on or off, for this session", run: (*Session).cmdColor},
		{names: []string{"/cognito"}, description: "record nothing, on or off", run: (*Session).cmdCognito},
		{names: []string{"/verbosity"}, usage: "/verbosity [0-6]", description: "how much the model is asked to answer with", run: (*Session).cmdVerbosity},
		{names: []string{"/verbose"}, description: "report the shape of each streamed turn, on or off", run: (*Session).cmdVerbose},
		{names: []string{"/delegate"}, usage: "/delegate QUESTION", description: "ask a question alongside, without recording it", run: (*Session).cmdDelegate},
		{names: []string{"/pane"}, usage: "/pane [main|delegate|spawn]", description: "show the conversation, the /delegate output or the /spawn output", run: (*Session).cmdPane},
		{names: []string{"/spawn"}, usage: "/spawn QUESTION", description: "answer a question in a worker given the tools", run: (*Session).cmdSpawn},
		{names: []string{"/btw"}, description: "start a thread branched from this conversation", run: (*Session).cmdBtw, idleOnly: true},
		{names: []string{"/main"}, description: "leave the thread and return to the conversation", run: (*Session).cmdMain, idleOnly: true},
		{names: []string{"/compact"}, description: "summarise the conversation and start again", run: (*Session).cmdCompact, idleOnly: true},
		{names: []string{"/save"}, usage: "/save [NAME]", description: "write the conversation to a file of its own", run: (*Session).cmdSave},
		{names: []string{"/load"}, usage: "/load NAME", description: "resume a conversation saved with /save", run: (*Session).cmdLoad, idleOnly: true},
		{names: []string{"/mouse"}, description: "turn mouse reporting on or off, for wheel scrolling", run: (*Session).cmdMouse},
		{names: []string{"/clear"}, description: "clear the pane", run: (*Session).cmdClear, idleOnly: true},
		{names: []string{"/info"}, description: "report the session settings", run: (*Session).cmdInfo},
		{names: []string{"/copy"}, description: "copy the conversation to the clipboard", run: (*Session).cmdCopy},
		{names: []string{"/permission"}, usage: "/permission [add|remove] [DIR] PROG...", description: "grant or refuse programs in a directory", run: (*Session).cmdPermission},
		{names: []string{"/autosave"}, usage: "/autosave [on|off|now]", description: "write the conversation without being asked", run: (*Session).cmdAutosave},
		{names: []string{"/approve"}, usage: "/approve [ask|allow|refuse]", description: "report or set whether programs run without asking", run: (*Session).cmdApprove, idleOnly: true},
		{names: []string{"/tools"}, description: "list the tools the model is given, and the root they are in", run: (*Session).cmdTools},
		{names: []string{"/quit", "/exit"}, description: "leave the interface", run: (*Session).cmdQuit},
	}
}

// command handles a slash command, reporting whether the session should end.
//
// The name is looked up in the table rather than switched on, so that the set
// of names is written in one place. A name the table does not carry is
// reported rather than refused, since what the reader typed looks like a
// command and is worth answering.
func (s *Session) command(line string) bool {
	args := strings.Fields(line)
	name := args[0]

	if c := lookupCommand(name); c != nil {
		if c.idleOnly && s.working() {
			// The refusal says what to do about it, since a command that
			// answered nothing would read as the interface having swallowed
			// the line.
			s.addReply("(" + name + " is refused while the model is working, press Esc to stop it first)")
			return false
		}
		return c.run(s, args[1:])
	}
	s.addReplyKind(kindFailure, "unknown command: "+name)
	return false
}

// lookupCommand returns the entry answering to the given name, or nil.
//
// The search is over the declared names rather than a switch, so that a
// synonym is declared once beside the command it belongs to rather than
// written a second time in the arm that runs it.
func lookupCommand(name string) *command {
	for i, c := range commands {
		for _, n := range c.names {
			if n == name {
				return &commands[i]
			}
		}
		for _, n := range c.hidden {
			if n == name {
				return &commands[i]
			}
		}
	}
	return nil
}

// cmdHelp lists the commands.
func (s *Session) cmdHelp([]string) bool {
	s.appendLines(helpText())
	return false
}

// cmdClear empties the pane and the conversation behind it.
func (s *Session) cmdClear([]string) bool {
	s.conv.Reset()
	s.clearReply()
	s.resetScroll()
	return false
}

// cmdQuit leaves. The end of the session is reported through the return rather
// than taken here, so that the dispatcher is the single place that decides it.
func (s *Session) cmdQuit([]string) bool { return true }

// cmdConnect tests the connection and reports the key.
func (s *Session) cmdConnect([]string) bool {
	s.connect()
	return false
}

// cmdKey reports the usage against the key.
func (s *Session) cmdKey([]string) bool {
	s.showUsage()
	return false
}

// cmdSearch opens the pane search.
func (s *Session) cmdSearch([]string) bool {
	s.beginSearch()
	return false
}

// cmdModels opens the catalogue.
func (s *Session) cmdModels([]string) bool {
	s.beginModelList(nil)
	return false
}

// cmdFreeModels opens the catalogue narrowed to the models that cost nothing.
//
// It is the same listing under a predicate rather than a second listing, so a
// change to one is a change to both.
func (s *Session) cmdFreeModels([]string) bool {
	s.beginModelList(func(m openrouter.Model) bool { return m.Free() })
	return false
}

// cmdModel shows or chooses the model.
func (s *Session) cmdModel(args []string) bool {
	s.chooseModel(args)
	return false
}

// cmdNew clears the conversation and says so, which /clear does not since it
// clears the very pane the report would be written to.
func (s *Session) cmdNew([]string) bool {
	s.conv.Reset()
	s.clearReply()
	s.resetScroll()
	s.Note("conversation cleared")
	return false
}

// cmdBell turns the terminal bell on or off.
func (s *Session) cmdBell([]string) bool {
	s.toggleBell()
	return false
}

// cmdCognito turns recording off or on.
func (s *Session) cmdCognito([]string) bool {
	s.toggleCognito()
	return false
}

// cmdVerbose turns the stream report on or off.
func (s *Session) cmdVerbose([]string) bool {
	s.toggleVerbose()
	return false
}

// cmdDelegate asks a question alongside the conversation.
//
// The words that follow are rejoined, since a delegate is a question rather
// than a flag and a reader who typed it as prose means it as prose.
func (s *Session) cmdDelegate(args []string) bool {
	s.startDelegate(strings.Join(args, " "))
	return false
}

// cmdBtw starts a thread branched from the conversation.
func (s *Session) cmdBtw([]string) bool {
	s.beginThread()
	return false
}

// cmdMain leaves the thread and returns to the conversation.
func (s *Session) cmdMain([]string) bool {
	s.endThread()
	return false
}

// cmdCompact summarises the conversation and starts again.
func (s *Session) cmdCompact([]string) bool {
	s.compact(true)
	return false
}

// cmdMouse turns mouse reporting on or off.
func (s *Session) cmdMouse([]string) bool {
	s.toggleMouse()
	return false
}

// cmdInfo reports the session settings.
func (s *Session) cmdInfo([]string) bool {
	s.showInfo()
	return false
}

// cmdTools reports what the model is given and where it is contained.
//
// It records nothing and is not refused while a model is working, since it
// reads a set built at startup and nothing that a turn in flight changes. A
// reader who wants to know what a model can reach should not have to stop it
// to find out.
// cmdTools reports the tools, their schemas, and what the shell will ask about.
//
// The approval rules are listed with the tools rather than left to the
// configuration file, since this is the one place a reader can find out what the
// client is willing to run and on what terms. A reader who cannot see which
// programs run without a question has no way to know what to expect when a
// model asks for one.
func (s *Session) cmdTools([]string) bool {
	lines := s.tools.toolsListing()
	lines = append(lines, s.approvalListing()...)
	s.appendLines(strings.Join(lines, "\n"))
	return false
}

// toggleMouse turns mouse reporting on or off.
//
// It exists because the decision cannot be made once for everyone. Capturing
// the mouse is what makes the wheel scroll, and it is also what stops a drag
// from selecting text, so the reader chooses.
func (s *Session) toggleMouse() {
	on := !s.screen.Mouse()
	s.setMouse(on)
	if on {
		s.appendLines("mouse reporting is on: the wheel scrolls, and a drag " +
			"no longer selects text. /mouse turns it off again.")
	} else {
		s.appendLines("mouse reporting is off: the wheel is left to the " +
			"terminal and text is selectable again.")
	}
}

// helpColumn is the width a command name is padded to in the help.
//
// It is one less than the longest name carrying an argument hint, since the
// hint is part of how a command is written but not of the name that selects
// it. A name that overruns the column pushes its description along rather
// than losing it, which is what a hand-aligned list does.
const helpColumn = 17

// helpText lists the commands, one per line, rendered from the same table the
// dispatcher and the completer read.
//
// The text is rendered rather than written out, so that a command added to the
// table appears here without a second edit. That is the drift the table
// exists to prevent, since a hand-written list here is exactly the copy that
// falls behind.
func helpText() string {
	lines := make([]string, 0, len(commands))
	for _, c := range commands {
		lines = append(lines, fmt.Sprintf("%-*s  %s", helpColumn, c.usageLine(), c.description))
	}
	return strings.Join(lines, "\n")
}

// usageLine returns how the command is written in the help, which is its names
// joined unless an argument hint is declared beside them.
func (c command) usageLine() string {
	if c.usage != "" {
		return c.usage
	}
	return strings.Join(c.names, ", ")
}

// candidates returns the command table as completer candidates, one per name.
//
// A command with a synonym becomes more than one candidate, since either word
// may be typed and the completer has to offer both to complete a prefix that
// reaches either.
func candidates() []complete.Candidate {
	out := make([]complete.Candidate, 0, len(commands))
	for _, c := range commands {
		for _, n := range c.names {
			out = append(out, complete.Candidate{Name: n, Description: c.description})
		}
	}
	return out
}

// completeLine offers the completion of the line and returns the line to
// compose in its place.
//
// An empty return leaves the line as it is, which is what an ambiguous prefix
// and one that matches nothing both do: neither should guess at a word. What
// was found is written to the pane in those cases, since a Tab that changed
// nothing and said nothing would read as a dead key. The repaint is left to
// the editor, which reports the line after every key, so that a report with no
// change to the line is drawn by the same path that draws any other keystroke.
func (s *Session) completeLine(line string) string {
	// The caret is placed at the end of the last word rather than at the end of
	// the line. A completion ends the line with a space, and a caret past that
	// space is in an argument rather than in the word, so completing it again
	// would find nothing to complete and the reader could never cycle.
	res := s.completer.Complete(line, lastWordEnd(line))
	// A cycle is held against the prefix rather than the line, since a line
	// that has been completed carries a whole word and completing that again
	// would ask about a prefix the reader never typed. The first Tab on a
	// prefix completes to a word; each Tab after it moves to the next match,
	// which is what a reader pressing Tab again is asking for.
	//
	// The cycle is consulted before the kind, so that a line already completed
	// to a whole word still cycles rather than completing uniquely to itself.
	// That is the case the second Tab falls in: the line now names one command,
	// so completing it yields that command again and the reader would never
	// reach the second match at all.
	if s.cycleNext(res) {
		// The candidates are listed again as well as cycled, so the position
		// in the set is on screen while the reader moves through it. The model
		// filter does the same: it re-renders its listing on every cycle so
		// the count in its heading follows the choice.
		s.showCandidates(res)
		return s.cycleLine(res)
	}

	switch res.Kind {
	case complete.Unique:
		s.notice("")
		s.clearCycle()
		return withTrailingSpace(res.Line)
	case complete.Ambiguous:
		s.notice("")
		// The first Tab completes to the first match rather than only listing
		// them, since a reader pressing Tab is asking for a word. The rest are
		// listed as well, since a reader who wants the choices rather than a
		// word still wants them.
		s.holdCycle(res)
		s.showCandidates(res)
		return s.cycleLine(res)
	case complete.NoMatch:
		s.notice("nothing matches " + res.Prefix)
	case complete.NotApplicable:
		s.notice("nothing to complete here")
	}
	return ""
}

// withTrailingSpace adds the space that separates a word from what follows it.
//
// The space is added once the token is a whole word rather than while it is
// still being typed, since a reader midway through "/mod" would see the line
// jump under them on every keystroke. It is added on completion, where the
// reader has said they meant that word.
func withTrailingSpace(line string) string {
	if line == "" || strings.HasSuffix(line, " ") {
		return line
	}
	return line + " "
}

// cycleNext moves to the next held candidate and reports whether there was one.
//
// It is what a second Tab on an unchanged prefix does. The prefix is part of
// the key: a reader who typed more since the last Tab is asking something else,
// and cycling would complete to a word that no longer matches what they wrote.
func (s *Session) cycleNext(res complete.Result) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.completedNames) == 0 {
		return false
	}
	// The cycle continues while the line still names one of the candidates it
	// was drawn from. That covers both the line as the reader typed it and the
	// line as a completion left it, since a completed word is one of its own
	// candidates and the prefix has grown into a word. A line that names
	// something else has been typed since, and cycling from it would complete
	// to a word the reader did not ask for.
	if res.Prefix != s.completedPrefix && !s.holdingName(res.Prefix) {
		return false
	}
	// The cycle wraps round, as the model filter does, so that a reader
	// pressing Tab past the last match comes back to the first. A reader who
	// has seen the whole set and presses Tab again is asking to start over
	// rather than being handed a key that has stopped doing anything.
	s.completedAt = (s.completedAt + 1) % len(s.completedNames)
	return true
}

// holdCycle remembers the candidates a prefix matched, for the Tabs after it.
func (s *Session) holdCycle(res complete.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedPrefix = res.Prefix
	s.completedNames = candidateNames(res)
	s.completedAt = 0
}

// clearCycle drops what is held, which is what typing past it does.
func (s *Session) clearCycle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completedPrefix = ""
	s.completedNames = nil
	s.completedAt = 0
}

// cycleLine is the line the held candidate is completed into.
//
// It is empty where none is held, which is the case where the line is left as
// it stands.
func (s *Session) cycleLine(res complete.Result) string {
	s.mu.Lock()
	names, at := s.completedNames, s.completedAt
	s.mu.Unlock()
	if len(names) == 0 || at >= len(names) {
		return ""
	}
	// The candidate replaces the whole token rather than being put after the
	// prefix, since the prefix is what the token was and the reader is looking
	// at a word rather than at a fragment. A candidate that carries its own
	// leading slash is used as it stands, which is why the prefix is not put in
	// front of it: the names come from the command table and already begin the
	// way the line does.
	return withTrailingSpace(names[at])
}

// showCandidates lists what a prefix matched, so that an ambiguous prefix can
// be narrowed rather than guessed at.
//
// The listing is written as ordinary text rather than drawn, since a selection
// out of the pane has to yield plain text with no escape sequence set around
// the match. The names are aligned so the descriptions line up beside them,
// and a row too wide for the pane is folded by the renderer rather than cut
// here.
func (s *Session) showCandidates(res complete.Result) {
	width := 0
	for _, c := range res.Candidates {
		if len(c.Name) > width {
			width = len(c.Name)
		}
	}

	s.mu.Lock()
	at := s.completedAt
	held := len(s.completedNames)
	s.mu.Unlock()

	head := res.Set + " matching " + res.Prefix
	if held > 0 {
		// The position is stated in the heading rather than marked against a
		// row, as it is for the model filter. The line already holds the
		// choice, so a marker would repeat what the reader can see, while the
		// heading says which of the set they are on.
		head += fmt.Sprintf(" [%d of %d]", at+1, held)
	}

	lines := make([]string, 0, len(res.Candidates)+1)
	lines = append(lines, head)
	for _, c := range res.Candidates {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, c.Name, c.Description))
	}
	s.appendLines(strings.Join(lines, "\n"))
}

// connect establishes the connection by contacting the key endpoint.
//
// The key endpoint is used rather than a model call because it is cheap and it
// distinguishes a bad key from a bad model, which is the first thing worth
// knowing when nothing works.
func (s *Session) connect() {
	if msg := s.credentialProblem(); msg != "" {
		s.addReply(msg)
		return
	}

	// A short deadline is used so that an unreachable endpoint reports
	// promptly rather than leaving the interface apparently hung.
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	s.beginWork()
	defer s.endWork()

	usage, err := s.client.KeyUsage(ctx)
	if err != nil {
		s.addReplyKind(kindFailure, "connect failed: "+err.Error())
		return
	}

	s.conv.usage = *usage
	s.updateStatus()
	s.addReply(fmt.Sprintf("connected: usage %g of %g", usage.Usage, usage.Limit))
}

// showUsage reports the usage against the key.
func (s *Session) showUsage() {
	if msg := s.credentialProblem(); msg != "" {
		s.addReply(msg)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	usage, err := s.client.KeyUsage(ctx)
	if err != nil {
		s.addReplyKind(kindFailure, "usage failed: "+err.Error())
		return
	}
	s.conv.usage = *usage
	s.updateStatus()

	line := fmt.Sprintf("usage %g of %g", usage.Usage, usage.Limit)
	if f := usage.FreeModelRequests; f != nil {
		line += fmt.Sprintf(", free models %g of %g", f.Used, f.Limit)
	}
	s.addReply(line)
}

// filtering reports whether the model filter is open.
func (s *Session) filtering() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modelList != nil
}

// filterKey reads one key for the filter.
func (s *Session) filterKey() {
	b, err := s.editor.ReadByte()
	if err != nil {
		s.mu.Lock()
		s.modelList = nil
		s.modelFilter = ""
		s.dropModelCycle()
		s.replaceReply(nil)
		s.mu.Unlock()
		return
	}
	s.modelListKey(b)
}

// beginModelList fetches the catalogue and opens the filter.
//
// The list is filtered as it is typed rather than submitted, since a catalogue
// is long enough that narrowing it by hand beats reading it. A filter that
// matches nothing says so rather than showing an empty pane.
func (s *Session) beginModelList(keep func(openrouter.Model) bool) {
	if msg := s.credentialProblem(); msg != "" {
		s.appendLines(msg)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	models, err := s.client.Models(ctx)
	if err != nil {
		s.addReplyKind(kindFailure, "models failed: "+err.Error())
		return
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	s.mu.Lock()
	s.modelList = models
	s.modelKeep = keep
	s.dropModelCycle()
	s.mu.Unlock()

	s.showModelFilter()
}

// showModelFilter draws the catalogue narrowed by what has been typed.
func (s *Session) showModelFilter() {
	s.pane()
	s.draw()
}

// pane builds the filtered listing without drawing it.
//
// The listing is built separately from the drawing so that it can be examined
// without a screen, which is what the tests do.
func (s *Session) pane() {
	s.mu.Lock()
	filter := s.modelFilter
	models := s.modelList
	keep := s.modelKeep
	cycling := s.modelCycle != nil
	at := s.modelCycleAt
	cycleLen := len(s.modelCycle)
	s.mu.Unlock()

	shown, total := modelMatches(models, keep, filter)

	// The frame is replaced rather than appended, so that narrowing the list
	// does not leave the previous listing on screen above it.
	head := "models"
	if filter != "" {
		head = "models matching " + filter
	}
	if keep != nil {
		head = "free " + head
	}
	if cycling {
		// The position is stated in the heading rather than marked against
		// a row. The filter holds the identifier that is selected, so a
		// marker would repeat what the filter already says, while the
		// heading says which of the set it is that the reader is on.
		head += fmt.Sprintf(" [%d of %d]", at+1, cycleLen)
	}

	lines := []string{head}
	for _, m := range shown {
		lines = append(lines, m.ID)
	}
	if total > maxShown {
		lines = append(lines, fmt.Sprintf("... and %d more", total-maxShown))
	}
	if len(shown) == 0 {
		lines = append(lines, "nothing matches")
	}
	lines = append(lines, "filter: "+filter+"_", "Tab cycles, Enter to choose, Esc to leave")

	s.mu.Lock()
	s.replaceReply(lines)
	s.mu.Unlock()
}

// maxShown is how many models the listing draws before it reports the rest.
const maxShown = 20

// modelMatches returns the models a filter keeps, capped at what the pane will
// draw, along with the number it matched in all.
//
// The cut is made here rather than in the drawing so that the completer cycles
// through the models the reader can see. A candidate that was never on the
// screen is one they cannot tell from any other.
func modelMatches(models []openrouter.Model, keep func(openrouter.Model) bool, filter string) ([]openrouter.Model, int) {
	lower := strings.ToLower(filter)
	var shown []openrouter.Model
	total := 0
	for _, m := range models {
		if keep != nil && !keep(m) {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(m.ID), lower) {
			continue
		}
		total++
		if len(shown) < maxShown {
			shown = append(shown, m)
		}
	}
	return shown, total
}

// modelIDs names the identifiers of a set of models.
func modelIDs(models []openrouter.Model) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.ID)
	}
	return out
}

// dropModelCycle ends a completion in progress.
//
// It is called wherever the filter changes for any other reason, since the
// candidates were generated from what has now been typed past.
func (s *Session) dropModelCycle() {
	s.modelCycle = nil
	s.modelCycleAt = 0
}

// completeModelFilter puts the next matching model into the filter.
//
// The first Tab completes, and the ones after it advance through what the
// filter matched at the time, wrapping at the end of it. Completing against the
// filter as it has been completed would be one Tab long, since a whole
// identifier matches only itself.
//
// A cycle ends as soon as the filter is changed for any other reason, so a Tab
// after a keystroke begins a new set from what is now typed rather than
// resuming a set the reader has moved on from.
//
// The cycle stops at the models the pane shows, since the point of it is to
// walk a set the reader is looking at rather than the whole catalogue.
func (s *Session) completeModelFilter() {
	s.mu.Lock()
	if s.modelCycle == nil {
		shown, _ := modelMatches(s.modelList, s.modelKeep, s.modelFilter)
		if len(shown) == 0 {
			// Nothing is said here, since the listing already reports that
			// nothing matches. A key that changed nothing and said nothing
			// would read as a dead key, and there is nothing to complete.
			s.mu.Unlock()
			return
		}
		s.modelCycle = modelIDs(shown)
		s.modelCycleAt = 0
	} else {
		s.modelCycleAt = (s.modelCycleAt + 1) % len(s.modelCycle)
	}
	s.modelFilter = s.modelCycle[s.modelCycleAt]
	s.mu.Unlock()

	s.pane()
	s.draw()
}

// trimLastRuneString removes the final character of a string.
//
// It exists because the editor trims a builder it owns, and the filter is held
// on the session rather than in the editor.
func trimLastRuneString(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

// modelListKey acts on a key typed into the model filter.
func (s *Session) modelListKey(b byte) {
	switch b {
	case keyEscape:
		s.mu.Lock()
		s.modelList = nil
		s.modelFilter = ""
		s.dropModelCycle()
		s.replaceReply(nil)
		s.mu.Unlock()
		s.draw()
	case keyTab:
		// Tab completes the filter and cycles through what it matched. The
		// key reaches here rather than the line editor behind it, since the
		// filter takes every key while it is open.
		s.completeModelFilter()
	case keyEnter:
		s.mu.Lock()
		s.modelList = nil
		filter := s.modelFilter
		s.modelFilter = ""
		s.dropModelCycle()
		s.mu.Unlock()
		s.draw()
		if filter != "" {
			s.chooseModel([]string{filter})
		}
	case keyBackspace, keyDelete:
		s.mu.Lock()
		s.modelFilter = trimLastRuneString(s.modelFilter)
		s.dropModelCycle()
		s.mu.Unlock()
		s.pane()
		s.draw()
	default:
		if b < 0x20 {
			return
		}
		s.mu.Lock()
		s.modelFilter += string(b)
		s.dropModelCycle()
		s.mu.Unlock()
		s.pane()
		s.draw()
	}
}

// chooseModel shows or sets the model.
func (s *Session) chooseModel(args []string) {
	if len(args) == 0 {
		if s.conv.Model() == "" {
			s.addReply("no model is selected: /model NAME")
			return
		}
		s.addReply("model: " + s.conv.Model())
		return
	}

	name := strings.Join(args, " ")
	// The identifier is everything before the first slash, so that a full
	// slug may be given without the vendor prefix being required.
	s.setModel(name)
	s.updateStatus()
	s.addReply("model: " + s.conv.Model())
}

// credentialProblem reports why a request cannot be sent.
//
// An absent key is caught here rather than by the backend, since OpenRouter
// answers a request with no credential the same way it answers an invalid one,
// reporting the key as rejected when in truth none was sent. A configuration
// with no key is an ordinary state rather than a fault.
func (s *Session) credentialProblem() string {
	if s.client == nil {
		return "no API key is configured: set OPENROUTER_API_KEY in " + configWhere()
	}
	if s.client.HasKey() {
		return ""
	}
	return "the configuration file holds no OPENROUTER_API_KEY: " +
		"a request cannot be sent without one"
}

// configWhere names where the client looks for its configuration.
//
// The locations are listed rather than one path named, since the loader checks
// them in order and a reader whose file is at the second one was being told to
// edit the first, which is a file the loader never read and which may hold
// nothing at all.
func configWhere() string {
	paths := config.SearchPaths()
	switch len(paths) {
	case 0:
		return "the configuration file"
	case 1:
		return paths[0]
	default:
		return strings.Join(paths[:len(paths)-1], " or ") + " or " + paths[len(paths)-1]
	}
}

// showInfo reports the session settings.
func (s *Session) showInfo() {
	s.appendLines("model:    " + orDash(s.conv.Model()))
	s.appendLines("endpoint: " + orDash(s.endpoint()))
	s.appendLines("key:      " + keyState(s.client))
}

// endpoint returns the configured endpoint.
func (s *Session) endpoint() string {
	if s.client == nil {
		return ""
	}
	return s.client.BaseURL()
}

// turnState is one request in flight.
//
// A turn carries its own context, so that stopping a model stops that request
// rather than the session, and its own done channel, so that the input loop can
// wait for the request to settle before sending an update to it. The
// conversation and the request text are taken when the turn is started, since
// an answer belongs to the conversation the question was asked in, and a
// follow-up amends the text as it was sent rather than whatever it has since
// become.
type turnState struct {
	// ctx is cancelled to stop the request.
	ctx    context.Context
	cancel context.CancelFunc
	// done is closed once the request has settled, after the pane, the
	// counters and the twiddle have been left in the state the turn ended in.
	done chan struct{}
	// line is the request text as it was sent.
	line string
	// conv is the conversation the request was made from.
	conv *Conversation
	// stopped records that the reader took this turn over with an update. It
	// is written under mu, which is the same lock that registers a turn, so
	// that a turn ended by the reader and a turn ended by the model cannot
	// both act on what was queued behind it.
	stopped bool
}

// startTurn sends the line to the model and returns at once.
//
// The request runs on a goroutine of its own, which is what makes a message
// queueable and a model stoppable at all: with the request on the input
// goroutine there is nothing reading the keys while a model works, and
// everything typed, escape included, waits for the reply to finish.
//
// The turn is registered before the goroutine starts, so that a reader who
// stops it at once is stopping a turn that is already recorded rather than one
// nobody is holding.
func (s *Session) startTurn(line string, conv *Conversation) {
	s.mu.Lock()
	if s.closing {
		// The session is on its way out, so a request begun now would be made
		// against a terminal nothing is drawing on. A turn started during the
		// teardown would also be a turn Close is not waiting for.
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	t := &turnState{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		line:   line,
		conv:   conv,
	}
	s.turn = t
	s.mu.Unlock()

	go s.runTurn(t)
}

// runTurn carries out one request and settles it.
//
// The bookkeeping is deferred before the channel is closed, since the defers
// run in the reverse order they are registered. A reader waiting to send an
// update to this turn is waiting for it to be settled, and a turn that called
// itself done while still writing to the pane would have the update drawn
// underneath it.
func (s *Session) runTurn(t *turnState) {
	defer close(t.done)
	defer s.settleTurn(t)
	s.send(t.ctx, t.conv, t.line)
}

// settleTurn clears a finished turn and sends what was queued behind it.
//
// The queue is drained only where the turn ended on its own. A turn the reader
// stopped sends its own update, and the stopped bit was set under the lock
// that registered the turn, so the two cannot both claim the queue.
func (s *Session) settleTurn(t *turnState) {
	s.mu.Lock()
	mine := s.turn == t
	stopped := t.stopped
	if mine {
		s.turn = nil
	}
	var next string
	if mine && !stopped && len(s.queued) > 0 {
		// One queued line is sent on its own. Two are two questions, and
		// sending them together would ask them as one.
		next, s.queued = s.queued[0], s.queued[1:]
	}
	s.mu.Unlock()

	if next != "" {
		// The turn that was ahead of it has been answered, so the line that
		// was queued behind it is a question of its own rather than an update
		// to a request nobody is making any more.
		s.startTurn(next, t.conv)
	}
}

// working reports whether a model is working.
func (s *Session) working() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turn != nil
}

// queueLine holds a line until the request in flight has ended.
func (s *Session) queueLine(line string) {
	s.mu.Lock()
	s.queued = append(s.queued, line)
	s.mu.Unlock()
}

// stopTurn stops the model and sends what is in hand as an update to the
// request it was answering.
//
// The turn is claimed under the lock, which is what settles who acts on the
// queue. A turn that ended by itself a moment earlier would otherwise drain the
// queue while this sent an update, and the reader would be sent the same
// message twice. The wait is for the turn to settle rather than for the
// request to stop, since a turn that has not finished unwinding would still be
// writing to the pane after the update was sent.
func (s *Session) stopTurn(update string) {
	s.mu.Lock()
	t := s.turn
	if t == nil {
		s.mu.Unlock()
		return
	}
	t.stopped = true
	// The queue goes ahead of the line being composed, since it is what was
	// committed first.
	parts := queuedLines(s.queued)
	s.queued = nil
	s.mu.Unlock()

	if trimmed := strings.TrimSpace(update); trimmed != "" {
		parts = append(parts, trimmed)
	}

	t.cancel()
	<-t.done

	// Nothing in hand is a stop and nothing more. The queue is empty in that
	// case, since the queue was taken above.
	if len(parts) == 0 {
		return
	}
	s.startTurn(amend(t.line, strings.Join(parts, "\n\n")), t.conv)
}

// amend folds a follow-up into the text of the request it updates.
//
// The two travel as one request rather than as a question and an answer to it,
// since the model was asked the first and the follow-up is the correction to
// it. A blank line separates them, which keeps the question and the correction
// apart without marking either, since a marker would be read as part of what
// was asked.
func amend(line, followup string) string {
	if strings.TrimSpace(followup) == "" {
		return line
	}
	return line + "\n\n" + followup
}

// send forwards a line to the model and shows the reply.
//
// The reply is appended a token at a time rather than once at the end, so that
// a slow model does not leave the interface apparently idle while it works.
//
// The context and the conversation are the ones the turn was started with
// rather than the session's, so that a reader can stop a turn without leaving
// and so that the answer is recorded where the question was asked.
func (s *Session) send(ctx context.Context, conv *Conversation, line string) {
	// Every exit from a turn ends on a painted frame, including the two early
	// refusals and every error path. The deferral is registered before either
	// refusal is tested, so a turn that never reached the request is as painted
	// as one that did, and it is the last thing to run so that it paints the
	// state the exits above left behind.
	defer s.paintFinal()

	if msg := s.credentialProblem(); msg != "" {
		s.addReply("> " + line)
		s.addReplyKind(kindNotice, msg)
		return
	}
	if conv.Model() == "" {
		s.addReply("> " + line)
		s.addReplyKind(kindNotice, "(no model is selected: /model NAME)")
		return
	}

	// A blank row separates what was asked from what is coming back.
	// Without it a reply reads as a continuation of the line that asked for
	// it, and two exchanges one after another are not told apart at a
	// glance. The two refusals above are left without it, since the reason
	// they give belongs with the line that provoked it rather than under
	// it.
	s.addReply("> "+line, "")
	s.updateStatus()
	s.draw()

	// The turns this turn has built. They are committed to the conversation
	// once, at the end, and only when the turn ended cleanly, which is the
	// guarantee the record under API settles: a user turn travels with the
	// request but is not recorded until a reply has finished arriving. Buffering
	// them rather than writing each round as it goes is what keeps a turn the
	// reader stopped from leaving a tool result behind for the model to be
	// told about as though it had asked for one and been answered.
	pending := []openrouter.Message{{Role: openrouter.RoleUser, Content: line}}

	// The tools are gathered once for the turn rather than per round. A turn
	// that is recording is the same turn for every request it makes, and one
	// that is not recording sends no tools on any of them.
	specs := s.turnTools(conv)

	// The twiddle turns for the whole turn, including the wait for the first
	// token, since that wait is where the interface would otherwise look frozen.
	s.beginWork()
	defer s.endWork()

	// The shape of the turn is gathered only when it was asked for, so that a
	// session without the mode does no work for it. It is read once here rather
	// than per event, since this is the goroutine that owns the turn and a mode
	// turned on mid-turn would otherwise show half a summary under a reply that
	// did not produce it. The figure covers the whole turn rather than one
	// round, since a turn that called tools made several requests and the
	// deltas of all of them are what happened.
	var report streamReport
	verbose := s.verboseOn()
	// The streaming fields are cleared on every exit from the turn, including
	// a failure, so that a stale partial reply is not left on screen behind the
	// error.
	defer func() {
		s.mu.Lock()
		s.frame.Busy = false
		s.frame.Partial = ""
		s.frame.Status.State = stateIdle
		s.mu.Unlock()
		if verbose {
			s.addReply("[stream] " + report.summary())
		}
	}()

	for round := 0; round < maxToolRounds; round++ {
		// The check runs before every request, since a request past the window
		// is refused outright and the round is lost with it. It is given the
		// turns about to be sent, since a tool result is usually larger than
		// the question that asked for it and an estimate taken once at the
		// start of the turn would not see one.
		s.maybeCompactFor(pending)

		var reply strings.Builder
		var calls []openrouter.ToolCall
		// failed records that the stream reported a failure of its own partway
		// through. A request that fails this way still returns nil, so the
		// reply below would otherwise be appended a second time, and the
		// exchange recorded as though the turn had ended cleanly.
		failed := false

		// stopReport appends what had arrived when the reader stopped the
		// model, and says that it was stopped. The text is kept, since it is
		// usually worth reading, and the notice is there so that a partial
		// answer is not read as the whole of what the model said. A stream cut
		// short by the cancellation is not a fault, and reporting it as one
		// would tell the reader that something broke when they had asked for it.
		stopReport := func() {
			if reply.Len() > 0 {
				s.addReplyKind(kindReply, strings.TrimRight(reply.String(), "\n"))
			}
			s.addReplyKind(kindFailure, "(stopped)")
		}

		err := s.client.Chat(ctx, openrouter.ChatRequest{
			Model:    conv.Model(),
			Messages: conv.PendingMessages(pending, s.verbosityLevel()),
			Tools:    specs,
		}, func(e openrouter.StreamEvent) {
			report.note(e)
			if e.Usage != nil {
				s.conv.AddTokens(e.Usage.PromptTokens, e.Usage.CompletionTokens)
				s.updateStatus()
			}
			if e.Err != nil {
				if ctx.Err() != nil {
					stopReport()
					return
				}
				// A stream that failed partway still produced text, and that
				// text is kept: it is usually more useful than an error alone.
				failed = true
				if reply.Len() > 0 {
					s.addReplyKind(kindReply, strings.TrimRight(reply.String(), "\n"))
				}
				s.addReplyKind(kindFailure, "(error) "+e.Err.Error())
				return
			}
			if e.Done {
				return
			}
			if len(e.ToolCalls) > 0 {
				// The transport gathers the fragments of a call and delivers
				// the whole set once, so there is nothing to reassemble here.
				// A turn that called a tool usually says nothing around the
				// call, so the text is empty and the calls are the turn.
				calls = e.ToolCalls
				return
			}
			reply.WriteString(e.Content)
			s.stream(reply.String())
		})

		if err != nil {
			if ctx.Err() != nil {
				stopReport()
				return
			}
			s.addReplyKind(kindFailure, "(error) "+err.Error())
			return
		}

		// The text and the error have already been shown above. Appending the
		// reply again would paint it twice, and recording the exchange would
		// leave a truncated answer in the conversation for the next request to
		// replay as though it were the whole of what the model said. A turn
		// that failed leaves no exchange behind.
		if failed {
			return
		}

		// The turn of this round is recorded as it came: the text it said and
		// the calls it made. A model that calls a tool and says nothing around
		// it produces a turn with empty content and the calls on it, which is
		// what the provider expects to see.
		pending = append(pending, openrouter.Message{
			Role:      openrouter.RoleAssistant,
			Content:   reply.String(),
			ToolCalls: calls,
		})

		if len(calls) == 0 {
			text := reply.String()
			if text == "" {
				s.addReply("(the model returned nothing)")
				return
			}
			// The reply is appended rather than written over the last line,
			// since that line is the question. Overwriting it loses the
			// exchange and makes the pane show a reply with nothing that
			// prompted it.
			s.appendModelText(withResponseRule(text))
			// The bell rings once the reply has finished arriving rather
			// than when the request was sent, since the point of it is to say
			// the answer is ready.
			s.ringBell()
			if s.thread != nil {
				s.thread.Note()
			}
			conv.RecordMessages(pending)
			// The conversation is written once it has been recorded, so a file
			// on disk never holds an exchange the conversation has not
			// accepted. A failure here is not reported into the pane: the
			// reader asked a question and got an answer, and a line about a
			// disk below the answer reads as though the answer were in doubt.
			s.autosaveAfterTurn()
			return
		}

		// A round that called tools keeps its own text on the screen. The
		// partial is cleared when the next round begins, so text that is only
		// shown while it arrives would vanish under the answer to the calls it
		// asked to make.
		if reply.Len() > 0 {
			s.appendModelText(reply.String())
		}

		if round == maxToolRounds-1 {
			// The turn is stopped rather than cut short silently. A reader
			// shown a reply with no explanation has been given a turn that
			// ended for a reason nothing said.
			s.addReplyKind(kindFailure, "(stopped: the model is still asking for tools, "+
				"and the turn reached its limit of "+itoa(maxToolRounds)+" requests)")
			return
		}

		for _, call := range calls {
			result := s.tools.run(call)
			s.drawToolCall(result)
			// A call that failed still produces a turn. A request carrying no
			// answer to a call is refused by most providers, and a turn that
			// produced nothing at all is a turn that stalls, which a reader
			// experiences as a hang.
			pending = append(pending, openrouter.Message{
				Role:       openrouter.RoleTool,
				ToolCallID: call.ID,
				Content:    toolContent(result),
			})
		}
	}
}

// itoa renders a count for a message, since the limit is reported in words
// rather than left for the reader to count in the source.
func itoa(n int) string { return strconv.Itoa(n) }

// stream shows a reply while it is arriving.
//
// The last pane line carries the partial text rather than a separate status
// line, so that the reply grows in place the way a terminal conversation is
// normally read.
func (s *Session) stream(partial string) {
	s.mu.Lock()
	s.frame.Busy = true
	s.frame.Status.State = stateWorking
	s.frame.Partial = partial
	s.mu.Unlock()

	// The repaint is coalesced like any other. A deferred one is owned by the
	// timer rather than by the next delta, so a reply that stops arriving is
	// still drawn rather than left owed to a stream that has finished.
	s.draw()
}

// updateStatus refreshes the fields the client knows.
func (s *Session) updateStatus() {
	// The figures are worked out before the lock is taken. Looking the window
	// up can reach the network on the first request for a model, and holding
	// the lock across that would stop the twiddle turning for the length of
	// the call, which is the one thing the twiddle is there to show.
	model := s.conv.Model()
	host := hostname()

	var credits string
	u := s.conv.Usage()
	if u.Limit > 0 {
		credits = fmt.Sprintf("%.2f/%.0f", u.Usage, u.Limit)
	}

	// The window is looked up on every repaint, which is cheap because it is
	// cached per model and only fetched when the model has not been seen. The
	// figure is shown as a percentage, since what a reader wants to know is how
	// close the conversation is to needing a compaction rather than the raw
	// count.
	var share string
	if window := s.windows.lookup(s.ctx, s, model); window > 0 {
		share = fmt.Sprintf("%.0f%%",
			float64(s.conv.EstimatedTokens())/float64(window)*100)
	}

	var tokensIn, tokensOut string
	if s.conv.TokensIn() > 0 {
		tokensIn = tokenCount(s.conv.TokensIn())
	}
	if s.conv.TokensOut() > 0 {
		tokensOut = tokenCount(s.conv.TokensOut())
	}

	// The assignment is taken under the lock, since the paint path copies the
	// whole frame under it and a status field written while that copy is being
	// taken is a field read halfway written.
	s.mu.Lock()
	s.frame.Status.Provider = s.providerLabel()
	s.frame.Status.Model = orDash(model)
	// The state is read from the frame rather than set to idle. The
	// accounting arrives on the last chunk of a turn, so writing idle here
	// reported a request finished while its reply was still arriving and the
	// twiddle still turning.
	if s.frame.Busy {
		s.frame.Status.State = stateWorking
	} else {
		s.frame.Status.State = stateIdle
	}
	s.frame.Status.Host = host
	s.frame.Status.Credits = credits
	s.frame.Status.Context = share
	s.frame.Status.TokensIn = tokensIn
	s.frame.Status.TokensOut = tokensOut
	s.frame.Status.Approval = s.approvalLabel()
	s.mu.Unlock()
}

// approvalLabel says what the client will do with a call it has not been told
// about in advance.
//
// The mode is read first, since it is what a reader has just asked for. A mode
// of allow or refuse settles every call on its own, and a bar showing a partial
// figure beside it would understate both: a reader who switched to allow needs
// to see that nothing will be asked, and one who switched to refuse needs to
// see that nothing will run.
// The caller holds the lock, since the label is read where the rest of the
// status is written and that is already inside the lock.
//
// A session built without the state, which is a test assembling one field at a
// time, is reported as asking. That is the mode a session begins in and the one
// that asks most, so it is the safe reading of a session that has not said
// otherwise.
// The caller holds the lock, since the label is read where the rest of the
// status is written.
func (s *Session) approvalLabel() string {
	if s.approvals == nil {
		return modeAsk.String()
	}
	mode := s.approvals.mode
	if mode != modeAsk {
		return mode.String()
	}
	// A session without tools has no directory for a rule to cover, which is a
	// test assembling one field at a time or a session whose working directory
	// could not be opened. Nothing is permitted either way.
	if s.tools == nil || s.tools.dir == "" {
		return modeAsk.String()
	}
	permitted := config.PermittedCommands(s.rules, s.tools.dir)
	switch {
	case len(permitted) == 0:
		return modeAsk.String()
	case s.approvalsAllPermitted(permitted):
		return modeAllow.String()
	default:
		return statePartial
	}
}

// approvalsAllPermitted reports whether every permitted program has been
// granted for this session as well.
//
// The file permits some programs and the reader may have granted more at the
// keyboard, so a bar showing the file alone would understate what the model can
// do. A session where the file settles everything is shown as settling
// everything, since that is what a reader watching it wants to know.
// The caller holds the lock, for the same reason approvalLabel does.
func (s *Session) approvalsAllPermitted(permitted []string) bool {
	if s.approvals == nil {
		return false
	}
	for _, name := range permitted {
		if !s.approvals.granted[name] {
			return false
		}
	}
	return true
}

// The values the approval field takes. They are short because a status bar is
// narrow, and they name the mode rather than the programs.
const (
	// stateAsk means every program is asked about.
	stateAsk = "ask"
	// stateAllow means the programs a file names run without asking.
	stateAllow = "allow"
	// statePartial means some programs were granted at the keyboard on top of
	// what a file names, and the rest are still asked about.
	statePartial = "partial"
)

// orDash returns the value, or a dash when it is empty.
func orDash(v string) string {
	if v == "" {
		return placeholder
	}
	return v
}

// keyState reports whether a credential is present, without revealing it.
func keyState(c *openrouter.Client) string {
	if c == nil {
		return "not configured"
	}
	return "configured"
}

// beginWork starts the twiddle.
//
// The state is set here rather than by the first streamed delta, since the
// wait for that first delta is the longest part of a turn and is where the
// interface would otherwise look finished while it waited.
func (s *Session) beginWork() {
	s.mu.Lock()
	s.frame.Busy = true
	s.frame.Status.State = stateWorking
	s.startedAt = time.Now()
	s.mu.Unlock()
	s.spinner.Start(func(frame string) {
		// The step advances once per frame, which is what makes the color
		// move at the rate the twiddle does rather than at a rate of its own.
		// The figure and the step are written under one lock, so a repaint
		// can never draw a color belonging to a different frame than the one
		// it is drawing.
		s.mu.Lock()
		s.frame.Spinner = frame
		s.frame.Tint = twiddleTint(s.step)
		s.frame.Elapsed = elapsedSince(s.startedAt)
		s.step++
		s.mu.Unlock()
		s.draw()
	})
}

// elapsedSince renders how long work has been running, or nothing where none
// has.
//
// The figure is kept short and grows by width rather than by precision: a
// turn a reader is watching takes seconds, and one that runs long enough to
// need minutes has stopped being a wait they are counting. Showing seconds
// from the first rather than adding them later is what keeps the row from
// changing width as the figures change length, which would shift the row under
// the eye that is already on it.
//
// A zero time means no work is running, and the row carries nothing. A clock
// running from zero would count from the epoch, and a row reading that would be
// worse than no row at all.
func elapsedSince(start time.Time) string {
	if start.IsZero() {
		return ""
	}
	d := time.Since(start)
	if d < 0 {
		// A clock that has been set back would otherwise show a negative
		// figure, which is a thing that cannot have happened.
		return "0s"
	}
	seconds := int(d.Seconds())
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	default:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	}
}

// endWork stops the twiddle and clears it from the frame.
//
// The busy bit is cleared here rather than by each caller that started the
// work, since a caller that forgot would leave the interface reporting work in
// progress for the rest of the session. A connection test clears it the same
// way a request does, because it is work in progress either way.
func (s *Session) endWork() {
	s.spinner.Stop()
	s.mu.Lock()
	s.frame.Spinner = ""
	// The tint is cleared with the twiddle rather than left behind, since a
	// tint with no twiddle would color whichever row it was pointed at.
	s.frame.Tint = ""
	s.frame.Elapsed = ""
	s.startedAt = time.Time{}
	s.frame.Busy = false
	s.frame.Status.State = stateIdle
	s.mu.Unlock()
	s.draw()
}

// draw repaints the frame.
func (s *Session) draw() {
	s.paint()
}

// paint renders the frame, coalescing the requests.
//
// A reply arrives a token at a time and the twiddle turns on its own clock, so
// without coalescing the frame is redrawn many times a second and the text
// visibly jumps. Requests that arrive within the interval are folded into one
// repaint at the end of it, which bounds the rate at which the screen changes
// however fast the input arrives.
//
// A repaint that is asked for while one is pending serves both callers, so the
// last state drawn is always the most recent one.
func (s *Session) paint() {
	s.repaint.Lock()
	defer s.repaint.Unlock()

	if s.closed {
		return
	}

	// A repaint within the interval is deferred rather than refused, so the
	// state drawn at the end of it is the most recent one and nothing is lost.
	if since := time.Since(s.lastPaint); since < minPaintInterval {
		s.paintPending = true
		s.flushIn(minPaintInterval - since)
		return
	}

	s.paintNow()
	s.lastPaint = time.Now()
}

// flushIn arranges for a deferred repaint to be drawn in the given time.
//
// The timer is the owner of the deferred repaint. Deferring one and leaving it
// unowned is what loses a frame: the work that asked for it finishes, the
// twiddle with it, and the next repaint comes from the reader typing, so the
// reply appears all at once rather than as it arrived. One timer serves every
// repaint deferred before it fires, since the frame is read at the moment it is
// drawn and the newest state is the one worth drawing.
//
// The caller holds repaint.
func (s *Session) flushIn(d time.Duration) {
	if s.flush != nil {
		return
	}
	s.flush = time.AfterFunc(d, s.flushDue)
}

// flushDue draws a repaint that has waited out the interval.
func (s *Session) flushDue() {
	s.repaint.Lock()
	defer s.repaint.Unlock()

	s.flush = nil
	if s.closed || !s.paintPending {
		return
	}
	s.paintPending = false
	s.paintNow()
	s.lastPaint = time.Now()
}

// paintFinal draws the frame at the end of a turn, whatever the rate is.
//
// The bound on the repaint rate keeps a fast reply readable, and it is not
// changed here. What it must not decide is whether the end of a turn is drawn at
// all. Nothing is left to ask for that repaint once the work is over, so a
// repaint deferred here is owed to nobody and the reply in place, the status
// back to idle and the cleared twiddle stay off the screen until the reader
// types. Every exit from a turn ends on one of these, so a turn that failed is
// as painted as a turn that succeeded.
func (s *Session) paintFinal() {
	s.repaint.Lock()
	defer s.repaint.Unlock()

	s.stopFlush()
	s.paintPending = false
	if s.closed {
		return
	}
	s.paintNow()
	s.lastPaint = time.Now()
}

// stopFlush cancels a repaint that is owed. The caller holds repaint.
func (s *Session) stopFlush() {
	if s.flush != nil {
		s.flush.Stop()
		s.flush = nil
	}
}

// minPaintInterval is the shortest time between two repaints.
//
// The figure is chosen so that a fast reply reads as text arriving rather than
// as a flickering block. A terminal repaints far faster than the eye resolves,
// so drawing every token only produces movement that cannot be read.
const minPaintInterval = 40 * time.Millisecond

// paintNow renders the frame without coalescing.
func (s *Session) paintNow() {
	s.mu.Lock()
	// A frame drawn after the terminal has been restored lands on whatever is
	// behind the interface, which is the shell. Repaint is held throughout, so
	// the check is made against the same guard Close set it under.
	if s.closed {
		s.mu.Unlock()
		return
	}
	// The pane carries a hint only while it is empty. Once a conversation has
	// started the hint is in the way, and the opening instructions are what
	// should be read.
	if len(s.frame.Reply) == 0 {
		s.frame.Hint = s.hint()
	} else {
		s.frame.Hint = ""
	}
	// The frame is copied before the lock is released, so that rendering,
	// which writes to the terminal, does not hold it. Holding the lock across
	// the write would serialise the spinner against the request loop, which
	// is the opposite of what the spinner is for.
	frame := s.frame
	frame.Scroll = s.scroll
	s.applyDelegatePane(&frame)
	s.applyWorkerPane(&frame)
	// The queue is copied rather than shared, since the renderer draws it
	// after the lock is released and the input goroutine is what appends to
	// it. A frame built from a slice being appended to would show a queue
	// that is neither what was queued nor empty.
	frame.Queued = append([]string(nil), s.queued...)
	// The hint row is built here rather than at each site that changes the
	// state, so that it cannot fall behind a state the row does not know
	// about. The busy bit is read from the frame, which the request loop
	// writes under the same lock, rather than being tracked separately and
	// risking the two disagreeing.
	hints := s.hints
	hints.busy = s.frame.Busy
	// The overlay is derived here rather than recorded at each site that
	// opens or closes one, since the flags are already guarded and six call
	// sites would be six chances to leave one of them out.
	switch {
	case s.asked != nil:
		// A question outranks the search and the filter. A reader who has
		// been asked whether a program may run is answering that, and a key
		// reaching a search behind it would be a key answering nothing.
		hints.overlay = hintConfirm
	case s.searchOpen:
		hints.overlay = hintSearch
	case s.modelList != nil:
		hints.overlay = hintListing
	default:
		hints.overlay = hintCompose
	}
	frame.Hints = hints.hints()
	s.mu.Unlock()

	height, width := s.screen.Size()
	// The palette is resolved before the frame is folded, since whether color
	// is on decides whether the markdown spans of a reply are worked out at all.
	pal := s.framePalette()
	frame.styleReplies = pal != nil
	rows, spans, drawn, twiddle := renderStyled(frame, height, width)
	// The offset the renderer drew at is adopted back into the session, so
	// that scrolling up further than there is history does not leave the
	// session holding an offset the pane cannot show. The change is made
	// only when the offset is the one the frame was drawn from, so a wheel
	// notch arriving while the frame was being drawn is not undone.
	if drawn != frame.Scroll {
		s.mu.Lock()
		if s.scroll == frame.Scroll {
			s.scroll = drawn
		}
		s.mu.Unlock()
	}
	// The color is pointed at the row the renderer reported. A frame with no
	// twiddle reports none, and the screen draws it exactly as it always has.
	//
	s.screen.DrawFrame(rows, framePaint{
		box:      frame.ConfirmBox,
		spans:    spans,
		pal:      pal,
		twiddle:  twiddle,
		sequence: frame.Tint,
		figure:   frame.Spinner,
	})
}

// hint reports what to do next, which differs depending on what is missing.
func (s *Session) hint() string {
	switch {
	case s.credentialProblem() != "":
		return "No API key is configured. Set OPENROUTER_API_KEY in " +
			configWhere() + ",\nthen run /connect."
	case s.conv.Model() == "":
		return "No model is selected. Run /models to see what is offered,\n" +
			"then /model NAME to choose one."
	default:
		return "Type a message and press Enter to send it to " + s.conv.Model() + ".\n" +
			"/help lists the commands."
	}
}

// clearNotice drops what is said about the line being composed.
//
// It is separate from the draw so that a keystroke can clear the notice without
// a repaint of its own, which would be a frame per character for no reason.
func (s *Session) clearNotice() {
	s.mu.Lock()
	s.frame.Notice = ""
	s.mu.Unlock()
}

// notice says something about the line being composed, on the input block.
//
// The notice is cleared when the reader types, so it does not sit above the
// prompt saying something that is no longer true. It is cleared on a successful
// completion for the same reason: the reader asked and was answered, and a
// complaint left above the line would read as a complaint about the answer.
func (s *Session) notice(text string) {
	s.mu.Lock()
	s.frame.Notice = text
	screen := s.screen
	s.mu.Unlock()
	// A session with no screen is one assembled by a test with nothing but a
	// completer. There is nothing to draw on, and asking for it would panic
	// rather than report the notice.
	if screen == nil {
		return
	}
	s.draw()
}

// candidateNames is the matches of a result, in the order they were offered.
func candidateNames(res complete.Result) []string {
	names := make([]string, 0, len(res.Candidates))
	for _, c := range res.Candidates {
		names = append(names, c.Name)
	}
	return names
}

// holdingName reports whether a token is one of the candidates being cycled.
//
// A completed line is one of them, which is how a second Tab reaches the
// candidates even though the line it is given names a whole command rather than
// the prefix they were drawn from.
func (s *Session) holdingName(name string) bool {
	return slices.Contains(s.completedNames, strings.TrimSpace(name))
}

// lastWordEnd is where the last word of a line ends.
//
// It is the caret a completion leaves behind: the line ends with the space that
// separates the word from what follows, and the word is what the next Tab acts
// on. A caret at the end of the line would be past the space and in nothing.
func lastWordEnd(line string) int {
	trimmed := strings.TrimRight(line, " \t")
	return len(trimmed)
}

// verbosityLevel is the level the session is asking for, taken under the lock.
//
// It is read on the request goroutine and written by the command on the input
// one, so it is taken under the same lock as the rest of what a turn reads.
func (s *Session) verbosityLevel() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verbosity
}
