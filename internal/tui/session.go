package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
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
	// out is where the bell is written, which is the interface output.
	out *os.File
	// cognito reports that this session records nothing. The mode is in force
	// because the user asked for it, or because it was left on by a crash,
	// which is reported at startup rather than honoured silently.
	cognito bool
	// verbose reports that the pane should describe the shape of each
	// streamed turn. It is a display mode rather than a recording one, so
	// nothing about the request or the conversation changes because of it.
	// It is guarded by mu, since the request goroutine reads it while the
	// input goroutine may turn it on.
	verbose bool
	// windows caches the context length of each model seen, so that the
	// threshold can be evaluated without a call per message.
	windows *contextLength
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
		s.mu.Unlock()
		s.draw()
	}
	// Tab completes the line rather than inserting a tab into it, since a
	// reader pressing it is asking what a word might be rather than asking
	// for whitespace. The editor holds no pane, so what to say about the
	// candidates is settled here and the editor is handed back the line to
	// compose.
	s.editor.OnTab = func(line string) string { return s.completeLine(line) }
	// The wheel is read on the same goroutine as the keys, since a report
	// arrives in the same stream. The callback moves the view and repaints,
	// which is what makes the scroll happen while the line is still being
	// composed rather than only after it is sent.
	s.editor.OnMouse = func(direction int) {
		s.scrollBy(direction)
	}
	return s, nil
}

// scrollBy moves the view one wheel notch in the given direction.
//
// The offset is held across a repaint deliberately: a new reply arriving while
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
	s.mu.Lock()
	s.frame.Reply = append(s.frame.Reply, lines...)
	s.mu.Unlock()
}

// clearReply empties the reply pane.
func (s *Session) clearReply() {
	s.mu.Lock()
	s.frame.Reply = nil
	s.mu.Unlock()
}

// appendLines adds text to the reply pane, one entry per line.
//
// The split happens here rather than at render time because a fenced code
// block spans lines. Splitting first leaves each fence marker on its own, so
// the renderer sees no block and folds code that must not be folded. The
// renderer folds what it is given, so what it is given has to carry the whole
// reply.
func (s *Session) appendLines(text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	s.addReply(text)
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
func (s *Session) Configure(baseURL, apiKey, model string) {
	// The model is adopted whether or not a credential is present, so that
	// the status bar reflects the file from the first repaint.
	if model != "" && s.conv.Model() == "" {
		s.conv.SetModel(model)
	}
	if apiKey != "" {
		s.client = openrouter.New(baseURL, apiKey)
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
		{names: []string{"/cognito"}, description: "record nothing, on or off", run: (*Session).cmdCognito},
		{names: []string{"/verbose"}, description: "report the shape of each streamed turn, on or off", run: (*Session).cmdVerbose},
		{names: []string{"/delegate"}, usage: "/delegate QUESTION", description: "ask a question alongside, without recording it", run: (*Session).cmdDelegate},
		{names: []string{"/btw"}, description: "start a thread branched from this conversation", run: (*Session).cmdBtw, idleOnly: true},
		{names: []string{"/main"}, description: "leave the thread and return to the conversation", run: (*Session).cmdMain, idleOnly: true},
		{names: []string{"/compact"}, description: "summarise the conversation and start again", run: (*Session).cmdCompact, idleOnly: true},
		{names: []string{"/mouse"}, description: "turn mouse reporting on or off, for wheel scrolling", run: (*Session).cmdMouse},
		{names: []string{"/clear"}, description: "clear the pane", run: (*Session).cmdClear, idleOnly: true},
		{names: []string{"/info"}, description: "report the session settings", run: (*Session).cmdInfo},
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
	s.addReply("unknown command: " + name)
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
	res := s.completer.Complete(line, len(line))
	switch res.Kind {
	case complete.Unique:
		return res.Line
	case complete.Ambiguous:
		s.showCandidates(res)
	case complete.NoMatch:
		s.appendLines("nothing matches " + res.Prefix)
	case complete.NotApplicable:
		s.appendLines("nothing to complete here")
	}
	return ""
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
	lines := make([]string, 0, len(res.Candidates)+1)
	lines = append(lines, res.Set+" matching "+res.Prefix)
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
		s.addReply("connect failed: " + err.Error())
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
		s.addReply("usage failed: " + err.Error())
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
		s.frame.Reply = nil
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
		s.appendLines("models failed: " + err.Error())
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
	s.frame.Reply = lines
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
		s.frame.Reply = nil
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
	s.conv.SetModel(name)
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
	parts := append([]string(nil), s.queued...)
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
		s.addReply("> "+line, msg)
		return
	}
	if conv.Model() == "" {
		s.addReply("> "+line, "(no model is selected: /model NAME)")
		return
	}

	s.addReply("> " + line)
	s.updateStatus()
	s.draw()

	// The check runs before the request is sent, since a request past the
	// window is refused outright and the turn is lost with it.
	s.maybeCompact()

	// The twiddle turns for the whole request, including the wait for the
	// first token, since that wait is where the interface would otherwise
	// look frozen.
	s.beginWork()
	defer s.endWork()

	var reply strings.Builder
	// The shape of the turn is gathered only when it was asked for, so that a
	// session without the mode does no work for it. The value is read once
	// here rather than per event, since this is the goroutine that owns the
	// turn and a mode turned on mid-request would otherwise show half a
	// summary.
	var report streamReport
	verbose := s.verboseOn()
	// The streaming fields are cleared on every exit from the request,
	// including a failure, so that a stale partial reply is not left on
	// screen behind the error.
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

	// failed records that the stream reported a failure of its own partway
	// through. A request that fails this way still returns nil, so the reply
	// below would otherwise be appended a second time, and the exchange
	// recorded as though the turn had ended cleanly.
	failed := false

	// stopReport appends what had arrived when the reader stopped the model,
	// and says that it was stopped. The text is kept, since it is usually
	// worth reading, and the notice is there so that a partial answer is not
	// read as the whole of what the model said. A stream cut short by the
	// cancellation is not a fault, and reporting it as one would tell the
	// reader that something broke when they had asked for it.
	stopReport := func() {
		if reply.Len() > 0 {
			s.addReply(strings.TrimRight(reply.String(), "\n"))
		}
		s.addReply("(stopped)")
	}

	err := s.client.Chat(ctx, openrouter.ChatRequest{
		Model:    conv.Model(),
		Messages: conv.Pending(line),
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
			// A stream that failed partway still produced text, and that text
			// is kept: it is usually more useful than an error alone.
			failed = true
			if reply.Len() > 0 {
				s.addReply(strings.TrimRight(reply.String(), "\n"))
			}
			s.addReply("(error) " + e.Err.Error())
			return
		}
		if e.Done {
			return
		}
		reply.WriteString(e.Content)
		s.stream(reply.String())
	})

	if err != nil {
		if ctx.Err() != nil {
			// The request was abandoned before a reply began, which is what a
			// reader who stopped it at once gets.
			stopReport()
			return
		}
		s.addReply("(error) " + err.Error())
		return
	}

	// The text and the error have already been shown above. Appending the
	// reply again would paint it twice, and recording the exchange would
	// leave a truncated answer in the conversation for the next request to
	// replay as though it were the whole of what the model said. A turn that
	// failed leaves no exchange behind, which is what the record under API
	// settles: a user turn travels with the request but is not recorded
	// until a reply has finished arriving.
	if failed {
		return
	}

	text := reply.String()
	if text == "" {
		s.addReply("(the model returned nothing)")
		return
	}
	// The reply is appended rather than written over the last line, since
	// that line is the question. Overwriting it loses the exchange and makes
	// the pane show a reply with nothing that prompted it.
	s.appendLines(text)
	// The bell rings once the reply has finished arriving rather than when the
	// request was sent, since the point of it is to say the answer is ready.
	s.ringBell()
	if s.thread != nil {
		s.thread.Note()
	}
	conv.Record(line, text)
}

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
	s.frame.Status.Provider = providerName
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
	s.mu.Unlock()
}

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
	s.mu.Unlock()
	s.spinner.Start(func(frame string) {
		s.mu.Lock()
		s.frame.Spinner = frame
		s.mu.Unlock()
		s.draw()
	})
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
	s.screen.Draw(Render(frame, height, width))
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
