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
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Session is a running interface.
type Session struct {
	screen *Screen
	editor *LineEditor
	frame  Frame

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
	// mouseRequested records that the reader asked for mouse reporting, so
	// that a request made before the terminal is ready is not lost.
	mouseRequested bool
	// thread is the ephemeral conversation, nil while the main one is in
	// force. The main conversation is held in mainConv throughout, so that
	// leaving a thread restores it without a snapshot being taken here.
	thread   *Ephemeral
	mainConv *Conversation
	// delegates are the background questions still running. They are tracked so
	// that leaving does not leave one writing to a frame nobody is drawing on,
	// and so that the count can be reported.
	delegates map[*Delegate]bool
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
	// windows caches the context length of each model seen, so that the
	// threshold can be evaluated without a call per message.
	windows *contextLength

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
		screen:  screen,
		editor:  NewLineEditor(in),
		conv:    NewConversation(),
		windows: newContextLength(),
		spinner: NewSpinner(),
		out:     out,
		ctx:     ctx,
		cancel:  cancel,
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
		s.frame.Input = line
		s.draw()
	}
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
	s.mouseRequested = on
	s.screen.SetMouse(on)
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
	s.cancel()
	// Reporting is turned off before the terminal is restored, so that a
	// wheel notch is not delivered to a program that has stopped reading.
	if s.screen.Mouse() {
		s.screen.SetMouse(false)
	}
	s.screen.Close()
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
	s.frame.Reply = append(s.frame.Reply, text)
}

// Note adds a line to the reply pane, for a message the client generates such
// as a bootstrap confirmation. The line is shown inside the frame rather than
// written before it, so that it is not cleared by the first repaint.
func (s *Session) Note(format string, args ...any) {
	s.frame.Reply = append(s.frame.Reply, fmt.Sprintf(format, args...))
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
func (s *Session) Seed(path string) error {
	doc, err := bootstrap.Load(path)
	if err != nil {
		return err
	}
	s.conv.Seed(doc.Instructions)
	s.Note("bootstrap: %s (%s)", doc.Path, doc.Format)
	return nil
}

// Run reads lines until the user leaves.
//
// A line beginning with a slash is treated as a command rather than as a
// prompt, so that the interface keeps its own vocabulary separate from the
// model input.
func (s *Session) Run() error {
	s.draw()
	for {
		line, err := s.editor.ReadLine()
		s.frame.Input = ""
		s.frame.Pasted = nil
		switch {
		case errors.Is(err, ErrEndOfInput):
			return nil
		case errors.Is(err, ErrInterrupt):
			if s.frame.Input == "" {
				return ErrQuit
			}
			// An interrupt with text in hand abandons the line rather than
			// the session, which is what a shell does.
			s.frame.Input = ""
			s.draw()
			continue
		case err != nil:
			return err
		}

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
		default:
			s.send(line)
		}
		s.draw()
	}
}

// command handles a slash command, reporting whether the session should end.
func (s *Session) command(line string) bool {
	args := strings.Fields(line)
	name := args[0]

	switch name {
	case "/help":
		s.appendLines(helpText())
	case "/clear":
		s.conv.Reset()
		s.frame.Reply = nil
		s.resetScroll()
	case "/quit", "/exit":
		return true
	case "/connect":
		s.connect()
	case "/key":
		s.showUsage()
	case "/models":
		s.listModels()
	case "/model":
		s.chooseModel(args[1:])
	case "/new":
		s.conv.Reset()
		s.frame.Reply = nil
		s.resetScroll()
		s.Note("conversation cleared")
	case "/bell":
		s.toggleBell()
	case "/cognito":
		s.toggleCognito()
	case "/delegate":
		s.startDelegate(strings.Join(args[1:], " "))
	case "/btw":
		s.beginThread()
	case "/main":
		s.endThread()
	case "/compact":
		s.compact(true)
	case "/mouse":
		s.toggleMouse()
	case "/info":
		s.showInfo()
	default:
		s.frame.Reply = append(s.frame.Reply, "unknown command: "+name)
	}
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

// helpText lists the commands.
func helpText() string {
	return strings.Join([]string{
		"/help              this list",
		"/connect           test the connection and report the key",
		"/key               report the usage against the key",
		"/models            list the models the endpoint offers",
		"/model [NAME]      show or choose the model, without an argument to list",
		"/new               clear the conversation",
		"/bell              ring the terminal bell on reply, on or off",
		"/cognito           record nothing, on or off",
		"/delegate QUESTION  ask a question alongside, without recording it",
		"/btw               start a thread branched from this conversation",
		"/main              leave the thread and return to the conversation",
		"/compact           summarise the conversation and start again",
		"/mouse             turn mouse reporting on or off, for wheel scrolling",
		"/clear             clear the pane",
		"/info              report the session settings",
		"/quit, /exit       leave the interface",
	}, "\n")
}

// connect establishes the connection by contacting the key endpoint.
//
// The key endpoint is used rather than a model call because it is cheap and it
// distinguishes a bad key from a bad model, which is the first thing worth
// knowing when nothing works.
func (s *Session) connect() {
	if msg := s.credentialProblem(); msg != "" {
		s.frame.Reply = append(s.frame.Reply, msg)
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
		s.frame.Reply = append(s.frame.Reply, "connect failed: "+err.Error())
		return
	}

	s.conv.usage = *usage
	s.updateStatus()
	s.frame.Reply = append(s.frame.Reply,
		fmt.Sprintf("connected: usage %g of %g", usage.Usage, usage.Limit))
}

// showUsage reports the usage against the key.
func (s *Session) showUsage() {
	if msg := s.credentialProblem(); msg != "" {
		s.frame.Reply = append(s.frame.Reply, msg)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	usage, err := s.client.KeyUsage(ctx)
	if err != nil {
		s.frame.Reply = append(s.frame.Reply, "usage failed: "+err.Error())
		return
	}
	s.conv.usage = *usage
	s.updateStatus()

	line := fmt.Sprintf("usage %g of %g", usage.Usage, usage.Limit)
	if f := usage.FreeModelRequests; f != nil {
		line += fmt.Sprintf(", free models %g of %g", f.Used, f.Limit)
	}
	s.frame.Reply = append(s.frame.Reply, line)
}

// listModels reports the models the endpoint offers.
func (s *Session) listModels() {
	if msg := s.credentialProblem(); msg != "" {
		s.frame.Reply = append(s.frame.Reply, msg)
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	models, err := s.client.Models(ctx)
	if err != nil {
		s.frame.Reply = append(s.frame.Reply, "models failed: "+err.Error())
		return
	}
	if len(models) == 0 {
		s.frame.Reply = append(s.frame.Reply, "the endpoint offers no models")
		return
	}

	// Sorted so that the list is the same between runs, since an unsorted
	// list cannot be scanned.
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	shown := models
	const maxShown = 20
	if len(shown) > maxShown {
		shown = shown[:maxShown]
	}
	for _, m := range shown {
		s.frame.Reply = append(s.frame.Reply, m.ID)
	}
	if len(models) > maxShown {
		s.frame.Reply = append(s.frame.Reply,
			fmt.Sprintf("... and %d more", len(models)-maxShown))
	}
}

// chooseModel shows or sets the model.
func (s *Session) chooseModel(args []string) {
	if len(args) == 0 {
		if s.conv.Model() == "" {
			s.frame.Reply = append(s.frame.Reply, "no model is selected: /model NAME")
			return
		}
		s.frame.Reply = append(s.frame.Reply, "model: "+s.conv.Model())
		return
	}

	name := strings.Join(args, " ")
	// The identifier is everything before the first slash, so that a full
	// slug may be given without the vendor prefix being required.
	s.conv.SetModel(name)
	s.updateStatus()
	s.frame.Reply = append(s.frame.Reply, "model: "+s.conv.Model())
}

// credentialProblem reports why a request cannot be sent.
//
// An absent key is caught here rather than by the backend, since OpenRouter
// answers a request with no credential the same way it answers an invalid one,
// reporting the key as rejected when in truth none was sent. A configuration
// with no key is an ordinary state rather than a fault.
func (s *Session) credentialProblem() string {
	if s.client == nil {
		return "no API key is configured: set OPENROUTER_API_KEY in " +
			"~/.openrouter-cli.json"
	}
	if s.client.HasKey() {
		return ""
	}
	return "the configuration file holds no OPENROUTER_API_KEY: " +
		"a request cannot be sent without one"
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

// send forwards a line to the model and shows the reply.
//
// The reply is appended a token at a time rather than once at the end, so that
// a slow model does not leave the interface apparently idle while it works.
func (s *Session) send(line string) {
	if msg := s.credentialProblem(); msg != "" {
		s.frame.Reply = append(s.frame.Reply, "> "+line, msg)
		return
	}
	if s.conv.Model() == "" {
		s.frame.Reply = append(s.frame.Reply,
			"> "+line, "(no model is selected: /model NAME)")
		return
	}

	s.frame.Reply = append(s.frame.Reply, "> "+line)
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
	// The streaming fields are cleared on every exit from the request,
	// including a failure, so that a stale partial reply is not left on
	// screen behind the error.
	defer func() {
		s.frame.Busy = false
		s.frame.Partial = ""
		s.frame.Status.State = stateIdle
	}()

	err := s.client.Chat(s.ctx, openrouter.ChatRequest{
		Model:    s.conv.Model(),
		Messages: s.conv.Pending(line),
	}, func(e openrouter.StreamEvent) {
		if e.Usage != nil {
			s.conv.AddTokens(e.Usage.PromptTokens, e.Usage.CompletionTokens)
			s.updateStatus()
		}
		if e.Err != nil {
			// A stream that failed partway still produced text, and that text
			// is kept: it is usually more useful than an error alone.
			if reply.Len() > 0 {
				s.frame.Reply = append(s.frame.Reply, strings.TrimRight(reply.String(), "\n"))
			}
			s.frame.Reply = append(s.frame.Reply, "(error) "+e.Err.Error())
			return
		}
		if e.Done {
			return
		}
		reply.WriteString(e.Content)
		s.stream(reply.String())
	})

	if err != nil {
		s.frame.Reply = append(s.frame.Reply, "(error) "+err.Error())
		return
	}

	text := reply.String()
	if text == "" {
		s.frame.Reply = append(s.frame.Reply, "(the model returned nothing)")
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
	s.conv.Record(line, text)
}

// stream shows a reply while it is arriving.
//
// The last pane line carries the partial text rather than a separate status
// line, so that the reply grows in place the way a terminal conversation is
// normally read.
func (s *Session) stream(partial string) {
	s.frame.Busy = true
	s.frame.Status.State = stateWorking
	s.frame.Partial = partial
	s.draw()
}

// updateStatus refreshes the fields the client knows.
func (s *Session) updateStatus() {
	s.frame.Status.Provider = providerName
	s.frame.Status.Model = orDash(s.conv.Model())
	s.frame.Status.State = stateIdle
	s.frame.Status.Host = hostname()

	u := s.conv.Usage()
	if u.Limit > 0 {
		s.frame.Status.Credits = fmt.Sprintf("%.2f/%.0f", u.Usage, u.Limit)
	}

	// The window is looked up on every repaint, which is cheap because it is
	// cached per model and only fetched when the model has not been seen. The
	// figure is shown as a percentage, since what a reader wants to know is how
	// close the conversation is to needing a compaction rather than the raw
	// count.
	if window := s.windows.lookup(s.ctx, s, s.conv.Model()); window > 0 {
		share := float64(s.conv.EstimatedTokens()) / float64(window) * 100
		s.frame.Status.Context = fmt.Sprintf("%.0f%%", share)
	} else {
		s.frame.Status.Context = ""
	}
	if s.conv.TokensIn() > 0 {
		s.frame.Status.TokensIn = tokenCount(s.conv.TokensIn())
	}
	if s.conv.TokensOut() > 0 {
		s.frame.Status.TokensOut = tokenCount(s.conv.TokensOut())
	}
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
func (s *Session) beginWork() {
	s.spinner.Start(func(frame string) {
		s.mu.Lock()
		s.frame.Spinner = frame
		s.mu.Unlock()
		s.draw()
	})
}

// endWork stops the twiddle and clears it from the frame.
func (s *Session) endWork() {
	s.spinner.Stop()
	s.mu.Lock()
	s.frame.Spinner = ""
	s.mu.Unlock()
	s.draw()
}

// draw repaints the frame.
func (s *Session) draw() {
	s.mu.Lock()
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
	s.mu.Unlock()

	height, width := s.screen.Size()
	s.screen.Draw(Render(frame, height, width))
}

// hint reports what to do next, which differs depending on what is missing.
func (s *Session) hint() string {
	switch {
	case s.credentialProblem() != "":
		return "No API key is configured. Set OPENROUTER_API_KEY in ~/.openrouter-cli.json,\n" +
			"then run /connect."
	case s.conv.Model() == "":
		return "No model is selected. Run /models to see what is offered,\n" +
			"then /model NAME to choose one."
	default:
		return "Type a message and press Enter to send it to " + s.conv.Model() + ".\n" +
			"/help lists the commands."
	}
}
