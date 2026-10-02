package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// A queue is a line sent while a model is working, and escape stopping that
// model with whatever is in hand. The tests here drive it through the session
// rather than through the request function, since what is under test is which
// request is made and what is recorded, and neither is decided by the request
// function alone.

// queueServer is an endpoint that holds each request open until it is released,
// and records the user turns of every request made.
type queueServer struct {
	mu    sync.Mutex
	asked []string
	gates []chan struct{}
	// stubborn reports that the endpoint answers even after the client has
	// given up, which is the case a wait for the turn has to cover.
	stubborn bool
	// input is the writer the keys were written to. It is closed by the test
	// rather than by the goroutine that wrote the keys, since a held escape is
	// delivered as a key once the wait for the rest of it passes, and a closed
	// pipe ends the line instead.
	input io.Closer
}

// gate returns the channel a request waits on, and is how a test lets a turn
// finish the way one finishes when a model answers.
func (q *queueServer) gate() chan struct{} {
	g := make(chan struct{})
	q.mu.Lock()
	q.gates = append(q.gates, g)
	q.mu.Unlock()
	return g
}

// endInput closes the input, which is what makes Run return.
func (q *queueServer) endInput() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.input != nil {
		q.input.Close()
		q.input = nil
	}
}

// release lets every request held open finish.
func (q *queueServer) release() {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, g := range q.gates {
		select {
		case <-g:
		default:
			close(g)
		}
	}
}

// record takes the body of a request and keeps the turn it was asked.
//
// Only the last user turn is kept, since that is the request as it was made.
// The turns in front of it are the conversation, which every later request
// carries, and a test reading them all would be reading the history.
func (q *queueServer) record(r *http.Request) {
	var body struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	asked := ""
	for _, m := range body.Messages {
		if m.Role == "user" {
			asked = m.Content
		}
	}
	q.mu.Lock()
	q.asked = append(q.asked, asked)
	q.mu.Unlock()
}

// userTurns returns what each request made so far was asked, in order.
func (q *queueServer) userTurns() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.asked...)
}

// count reports how many requests have been made.
func (q *queueServer) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.asked)
}

// queueSession returns a session pointed at an endpoint that holds each request
// open until it is released, with the callbacks Start installs so that a key
// can be delivered the way a terminal would deliver it.
//
// The keys are written once and the pipe is closed, so a session driven through
// Run leaves as soon as they have been read. A test that needs the session to
// keep working past that drives the session directly instead.
func queueSession(t *testing.T, keys string) (*Session, *queueServer, func() string) {
	t.Helper()

	q := &queueServer{}
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
				return
			}
			q.record(r)
			if q.stubborn {
				// A stream that ignores the cancellation and keeps sending. A
				// turn reading it cannot settle when the session is
				// cancelled, so anything that waits for the turn has to wait
				// for the stream rather than for the cancel.
				for i := 0; i < 10; i++ {
					time.Sleep(50 * time.Millisecond)
					io.WriteString(w, `data: {"choices":[{"delta":{"content":"x"}}]}`+"\n\n")
					w.(http.Flusher).Flush()
				}
				io.WriteString(w, "data: [DONE]\n\n")
				return
			}
			gate := q.gate()
			// The request waits for the test to let it finish, or for the
			// reader to stop the model, whichever comes first. A request that
			// ignored the cancellation would hang the test rather than leave
			// it passing.
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
			io.WriteString(w, auditStream)
		}))
	t.Cleanup(srv.Close)

	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}
	// The file stands in for the terminal on both ends, since a session that
	// restores a screen restores the descriptor it was handed.
	screen := &Screen{out: out, in: out, height: 24, width: 80}
	capture := func() string {
		b, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatalf("reading the capture: %v", err)
		}
		return string(b)
	}

	conv := NewConversation()
	conv.SetModel("test/model")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s := &Session{
		conv:     conv,
		mainConv: conv,
		screen:   screen,
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      ctx,
		cancel:   cancel,
	}
	t.Cleanup(s.Close)

	if keys != "" {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatalf("creating the input pipe: %v", err)
		}
		t.Cleanup(func() {
			reader.Close()
			q.endInput()
		})
		s.editor = NewLineEditor(reader)
		s.editor.OnChange = func(line string) {
			s.mu.Lock()
			s.frame.Input = line
			s.mu.Unlock()
			s.draw()
		}
		q.input = writer
		go func() { io.WriteString(writer, keys) }()
	}
	return s, q, capture
}

// waitWorking waits for a turn to be registered, bounded so that a session
// which never starts one fails rather than hangs.
func waitWorking(t *testing.T, s *Session, want bool) {
	t.Helper()
	waitFor(t, func() bool { return s.working() == want },
		"the session is working=%v, want %v", s.working(), want)
}

// waitIdle waits for every turn to have finished and for the queue to have
// drained, which is what a session that has caught up looks like.
func waitIdle(t *testing.T, s *Session) {
	t.Helper()
	waitFor(t, func() bool { return !s.working() && len(queueOf(s)) == 0 },
		"the session did not come back to idle")
}

// waitRequests waits for a number of requests to have arrived at the endpoint,
// bounded. A turn is registered before its request is made, so a test that
// acted on the turn alone would be acting before the request existed.
func waitRequests(t *testing.T, q *queueServer, n int) {
	t.Helper()
	waitFor(t, func() bool { return q.count() >= n },
		"%d requests arrived, want %d", q.count(), n)
}

// queueOf returns the queue as it stands.
func queueOf(s *Session) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queued...)
}

// auditPane returns the reply pane as it stands.
func auditPane(s *Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.frame.Reply, "\n")
}

// waitFor waits for a condition, bounded so that a failure fails rather than
// hangs.
func waitFor(t *testing.T, ok func() bool, msg string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf(msg, args...)
}

// A line sent while a model is working is held rather than refused, and the
// model carries on. Each queued line is sent on its own once the request ahead
// of it has been answered, since two lines are two questions and sending them
// together would ask them as one.
func TestALineSentWhileAModelWorksIsQueued(t *testing.T) {
	s, q, _ := queueSession(t, "")

	s.startTurn("the question", s.conv)
	waitWorking(t, s, true)
	waitRequests(t, q, 1)

	s.queueLine("an update")
	s.queueLine("another one")
	waitFor(t, func() bool { return len(queueOf(s)) == 2 },
		"the queue holds %q, want the two lines that were sent", queueOf(s))

	if !s.working() {
		t.Error("queuing a line stopped the model, want the model to carry on")
	}

	q.release()
	waitFor(t, func() bool { return q.count() == 2 },
		"the first queued line was not sent once the request ahead of it was answered")
	if turns := q.userTurns(); turns[1] != "[QUEUED] an update" {
		t.Errorf("the second request carried %q, want the first queued line alone", turns[1])
	}

	q.release()
	waitFor(t, func() bool { return q.count() == 3 },
		"the second queued line was not sent after the first was answered")
	if turns := q.userTurns(); turns[2] != "[QUEUED] another one" {
		t.Errorf("the third request carried %q, want the second queued line alone", turns[2])
	}

	q.release()
	waitIdle(t, s)
}

// The queue is drawn above the prompt and marked as queued, since a line the
// reader cannot see is a line they would send again.
func TestTheQueueIsDrawnAboveThePrompt(t *testing.T) {
	s, _, capture := queueSession(t, "")

	s.queueLine("the correction to the question")
	s.draw()

	frame := auditLastFrame(t, capture)
	prompt, queued := -1, -1
	for i, row := range strings.Split(frame, "\n") {
		if strings.HasPrefix(row, "> ") && prompt < 0 {
			prompt = i
		}
		if strings.Contains(row, "the correction to the question") && queued < 0 {
			queued = i
		}
	}
	if queued < 0 {
		t.Fatalf("the queued message is not on the screen.\n%s", frame)
	}
	if !strings.Contains(frame, queuedMarker) {
		t.Errorf("the queued message is not marked as queued.\n%s", frame)
	}
	if prompt >= 0 && queued > prompt {
		t.Errorf("the queue is drawn below the prompt.\n%s", frame)
	}
}

// Escape with a line in hand stops the model and sends the line as an update to
// the request it was answering. The update travels as part of that request
// rather than as a question after it, since the model was asked the first
// question and the line is the correction to it.
//
// The keys go through the editor and the session loop rather than through the
// stop, since what is under test is that a reader pressing escape mid-answer is
// answered the way the feature says.
func TestEscapeStopsTheModelAndSendsTheLineAsAnUpdate(t *testing.T) {
	s, q, _ := queueSession(t, "the question\ran update\x1b")

	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	waitFor(t, func() bool { return q.count() == 2 },
		"the update was never sent: %d requests were made", q.count())
	turns := q.userTurns()
	want := "the question\n\nan update"
	if turns[1] != want {
		t.Errorf("the update was sent as %q, want %q", turns[1], want)
	}

	// The stopped turn is not recorded, since it never produced an answer. A
	// partial answer recorded as though it were a whole one would be replayed
	// to the model on the next request.
	if got := len(s.conv.messages); got != 0 {
		t.Errorf("the conversation holds %d turns, want none: a stopped turn is not an exchange", got)
	}

	q.endInput()
	if err := waitRun(t, done); err != nil {
		t.Errorf("the session returned %v", err)
	}
	q.release()
	waitIdle(t, s)
	// The turn in flight is waited for on the way out, so this also covers
	// Close not returning while a request is still unwinding.
	s.Close()
}

// Escape with nothing in hand and something queued sends the queued line, since
// the queue is what is in hand.
func TestEscapeSendsAQueuedLineWhenTheInputIsEmpty(t *testing.T) {
	s, q, _ := queueSession(t, "the question\ran update\r\x1b")

	done := make(chan error, 1)
	go func() { done <- s.Run() }()

	waitFor(t, func() bool { return q.count() == 2 },
		"the queued line was never sent: %d requests were made", q.count())
	turns := q.userTurns()
	if turns[1] != "the question\n\n[QUEUED] an update" {
		t.Errorf("the update was sent as %q, want the queued line as an update", turns[1])
	}

	q.endInput()
	if err := waitRun(t, done); err != nil {
		t.Errorf("the session returned %v", err)
	}
	q.release()
	waitIdle(t, s)
	s.Close()
}

// Escape with nothing in hand and nothing queued stops the model and sends
// nothing. It is the stop on its own, which is the one case where the reader
// has not asked for an answer.
func TestEscapeWithNothingInHandStopsTheModelAndSendsNothing(t *testing.T) {
	s, q, _ := queueSession(t, "")

	s.startTurn("the question", s.conv)
	waitRequests(t, q, 1)

	s.stopTurn("")

	// The stop settles before the reader is answered, so a second request here
	// would be one the reader did not ask for.
	time.Sleep(100 * time.Millisecond)
	if n := q.count(); n != 1 {
		t.Errorf("%d requests were made, want only the one that was stopped", n)
	}
	if s.working() {
		t.Error("the session still reports work in progress after the stop")
	}
	q.release()
	waitIdle(t, s)
}

// A stop claims the queue under the lock, so a turn that ends at the same moment
// cannot also drain it. Without that, the reader would be sent the queued line
// once by the stop and again by the turn finishing.
func TestAQueuedLineIsNotSentTwiceWhenTheModelIsStopped(t *testing.T) {
	s, q, _ := queueSession(t, "")

	s.startTurn("the question", s.conv)
	waitRequests(t, q, 1)
	s.queueLine("an update")
	s.stopTurn("")

	// The stop has sent the update, so a third request here would be the queue
	// being drained a second time.
	waitRequests(t, q, 2)
	time.Sleep(50 * time.Millisecond)
	if n := q.count(); n != 2 {
		t.Fatalf("%d requests were made, want the question and the update", n)
	}
	if turns := q.userTurns(); turns[1] != "the question\n\n[QUEUED] an update" {
		t.Errorf("the update was sent as %q, want the queued line", turns[1])
	}

	q.release()
	waitIdle(t, s)
	time.Sleep(50 * time.Millisecond)
	if n := q.count(); n != 2 {
		t.Errorf("%d requests were made, want two: the queue was sent once and then again", n)
	}
}

// A turn that was stopped reports itself as stopped rather than as a fault the
// reader caused and asked for.
func TestAStoppedTurnIsReportedAsStopped(t *testing.T) {
	s, _, _ := queueSession(t, "")

	// The request is made against a context that is already done, which is the
	// state a reader who stops a model at once is in.
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()
	s.send(ctx, s.conv, "the question")

	pane := auditPane(s)
	if !strings.Contains(pane, "(stopped)") {
		t.Errorf("the pane does not report the turn as stopped.\n%s", pane)
	}
	if strings.Contains(pane, "(error)") {
		t.Errorf("a stop the reader asked for is reported as a fault.\n%s", pane)
	}
}

// A command that changes the conversation is refused while a model is working,
// since a turn records its answer into the conversation it was asked in. The
// refusal says what to do about it, since a command that answered nothing would
// read as the interface having swallowed the line.
func TestAConversationCommandIsRefusedWhileAModelIsWorking(t *testing.T) {
	s, _, _ := queueSession(t, "")

	s.appendLines("a line the reader wrote")
	s.startTurn("the question", s.conv)
	waitWorking(t, s, true)

	if quit := s.command("/new"); quit {
		t.Fatal("/new left the session, want it refused")
	}
	pane := auditPane(s)
	if !strings.Contains(pane, "refused while the model is working") {
		t.Errorf("the refusal is not in the pane.\n%s", pane)
	}
	if !strings.Contains(pane, "a line the reader wrote") {
		t.Errorf("/new cleared the pane while a model was working.\n%s", pane)
	}

	// The same command works once the model is finished, since the refusal is
	// about the turn in flight and not about the command.
	s.mu.Lock()
	turn := s.turn
	s.mu.Unlock()
	turn.cancel()
	<-turn.done
	waitIdle(t, s)

	s.command("/new")
	if !strings.Contains(auditPane(s), "conversation cleared") {
		t.Errorf("/new did not run once the model was idle.\n%s", auditPane(s))
	}
}

// Close waits for the turn in flight, since a turn that is still unwinding
// writes to the pane and the counters after the terminal has been handed back.
func TestCloseWaitsForTheTurnInFlight(t *testing.T) {
	s, q, _ := queueSession(t, "")
	q.stubborn = true

	s.startTurn("the question", s.conv)
	waitRequests(t, q, 1)

	s.mu.Lock()
	turn := s.turn
	s.mu.Unlock()
	if turn == nil {
		t.Fatal("no turn was registered")
	}

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()

	// What is established is that nothing is left in flight once Close has
	// returned. The wait cannot be timed here: the cancellation Close makes
	// reaches the request, and a request that has been cancelled unwinds at
	// once, so a Close that did not wait would look the same from the outside.
	// What a missing wait would leave behind is the turn itself, still running
	// against a terminal that has been handed back.
	select {
	case <-closed:
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return")
	}
	select {
	case <-turn.done:
	default:
		t.Error("Close returned with a turn still running")
	}
	s.mu.Lock()
	still := s.turn
	s.mu.Unlock()
	if still != nil {
		t.Error("Close returned with a turn still registered")
	}
}

// A turn is not started against a session on its way out, since it would be a
// request made against a terminal nothing is drawing on, and a turn Close is
// not waiting for.
func TestATurnIsNotStartedOnceTheSessionIsClosing(t *testing.T) {
	s, q, _ := queueSession(t, "")

	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	s.startTurn("the question", s.conv)
	time.Sleep(100 * time.Millisecond)
	if s.working() {
		t.Error("a turn was started against a session that is closing")
	}
	if n := q.count(); n != 0 {
		t.Errorf("%d requests were made, want none", n)
	}
}

// amend keeps the question and the follow-up apart, and leaves the request
// alone when there is no follow-up to fold in.
func TestAmend(t *testing.T) {
	if got := amend("the question", "an update"); got != "the question\n\nan update" {
		t.Errorf("amend gave %q", got)
	}
	if got := amend("the question", "  "); got != "the question" {
		t.Errorf("amend with nothing to fold in gave %q", got)
	}
}

// waitRun reports what Run returned, bounded.
func waitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the session did not leave")
		return nil
	}
}
