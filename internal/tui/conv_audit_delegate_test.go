package tui

import (
	"context"
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

// The conversation a delegate branched from is replaced while the delegate is
// still running, since a thread can start and the in-cognito mode can be
// turned on. The delegate goroutine must not read it to find out which model to
// ask, because the input goroutine is the one replacing it.
func TestDelegateDoesNotReadTheConversationWhileItRuns(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	s := blockingSession(t, auditStream, reached, release)
	s.conv.Record("a question", "an answer")

	s.startDelegate("what next")

	// The input goroutine goes on starting and leaving threads while the
	// delegate works, which is what replaces the conversation. No channel
	// passes between the two here, which is the point: the goroutine reading
	// the conversation while this runs is an unordered access to it.
	for i := 0; i < 200; i++ {
		s.beginThread()
		s.endThread()
	}

	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for s.delegatePending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.delegatePending(); got != 0 {
		t.Fatalf("%d delegates still running after the release", got)
	}
}

// A delegate that produced nothing says so. An empty line in the pane is
// indistinguishable from a question that was never asked, and the reader has
// no other way to tell.
func TestDelegateSaysWhenNothingCameBack(t *testing.T) {
	// A stream that terminates at once carries no text.
	s, _ := auditSession(t, "data: [DONE]\n\n")
	s.conv.Record("a question", "an answer")

	s.startDelegate("what next")

	deadline := time.Now().Add(10 * time.Second)
	for s.delegatePending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.delegatePending(); got != 0 {
		t.Fatalf("%d delegates still running", got)
	}

	s.mu.Lock()
	lines := append([]string{}, s.frame.Reply...)
	s.mu.Unlock()

	found := false
	for _, line := range lines {
		if line == "" {
			t.Errorf("the pane carries an empty line: %q", lines)
		}
		if strings.Contains(line, "returned nothing") {
			found = true
		}
	}
	if !found {
		t.Errorf("pane = %q, want the absence of an answer stated", lines)
	}
}

// heldSession returns a session whose server answers the model list at once and
// then holds the first completion open, together with a function that releases
// it.
//
// The hold is released by a cleanup registered after the one that shuts the
// server down, since cleanups run in reverse: a handler still inside the server
// keeps it from shutting down, so a hold with nothing to release it blocks the
// teardown and the test times out there rather than failing at its assertion.
// The release is idempotent so that the test can let it go as soon as it has
// what it wanted without having to know whether the teardown got there first.
func heldSession(t *testing.T, reached chan<- struct{}) (*Session, func()) {
	t.Helper()

	var mu sync.Mutex
	blocked := false
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				io.WriteString(w,
					`{"data":[{"id":"test/model","context_length":8192}]}`)
				return
			}
			mu.Lock()
			first := !blocked
			blocked = true
			mu.Unlock()
			if first {
				reached <- struct{}{}
				// The request is held rather than answered. The client is not
				// waiting on this handler to return: cancelling the request is
				// what ends its side, and it is what Close does to a delegate.
				<-release
			}
			io.WriteString(w, auditStream)
		}))
	t.Cleanup(srv.Close)
	t.Cleanup(releaseOnce)

	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}

	conv := NewConversation()
	conv.SetModel("test/model")
	// The context is a real one rather than the background the other test
	// helpers use, since cancelling it is what ends the delegate's request, and
	// a cancel that does nothing would leave Close waiting for a delegate that
	// never unwinds.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Session{
		conv:     conv,
		mainConv: conv,
		screen:   &Screen{out: out, height: 24, width: 80},
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      ctx,
		cancel:   cancel,
	}, releaseOnce
}

// Closing the session returns to the shell, so a delegate still running is one
// still writing to a frame nobody will read and still holding a request open
// against a terminal that has been handed back. Close cancels the delegate and
// then waits for it to finish unwinding.
func TestCloseWaitsForARunningDelegate(t *testing.T) {
	reached := make(chan struct{}, 1)
	s, release := heldSession(t, reached)
	defer release()
	s.conv.Record("a question", "an answer")

	s.startDelegate("what next")

	select {
	case <-reached:
	case <-time.After(30 * time.Second):
		t.Fatal("the delegate never reached the server")
	}

	// The frame lock is held before Close is started rather than taken
	// afterwards. A delegate writes its answer under it, so Close cannot get
	// past its own lock step while the test holds it, and taking the lock first
	// keeps the comparison fair: a delegate unwinds within microseconds of
	// being cancelled, so a Close that does wait can be over before a test that
	// took the lock afterwards gets to it.
	//
	// What the window establishes is that Close reaches for the lock at all.
	// What the check below it establishes is the wait itself: a delegate still
	// tracked at the moment Close returns means Close went back to the shell
	// with a goroutine still running against a frame nobody will read.
	s.mu.Lock()
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		s.Close()
	}()

	select {
	case <-returned:
		s.mu.Unlock()
		t.Fatal("Close returned with a delegate still running")
	case <-time.After(300 * time.Millisecond):
	}
	s.mu.Unlock()

	select {
	case <-returned:
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return once the delegate had been cancelled")
	}

	if got := s.delegatePending(); got != 0 {
		t.Errorf("%d delegates still tracked after Close returned", got)
	}
}

// A delegate started after Close has begun waiting would be one nobody is
// waiting for, since the counter it raises comes too late for the wait to see
// it. It is refused rather than run.
func TestADelegateIsRefusedOnceTheSessionIsClosing(t *testing.T) {
	s, _ := auditSession(t, auditStream)
	s.conv.Record("a question", "an answer")

	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()

	s.startDelegate("what next")

	if got := s.delegatePending(); got != 0 {
		t.Errorf("%d delegates were started against a closing session", got)
	}

	s.mu.Lock()
	lines := append([]string{}, s.frame.Reply...)
	s.mu.Unlock()

	found := false
	for _, line := range lines {
		if strings.Contains(line, "closing") {
			found = true
		}
	}
	if !found {
		t.Errorf("pane = %q, want the refusal to be stated", lines)
	}
}
