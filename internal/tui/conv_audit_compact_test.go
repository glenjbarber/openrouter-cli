package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// blockingSession returns a session whose server answers the model list at
// once and then holds a completion open until the test releases it, so that
// work in progress can be looked at while it is still running.
func blockingSession(t *testing.T, body string,
	reached chan<- struct{}, release <-chan struct{}) *Session {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				io.WriteString(w,
					`{"data":[{"id":"test/model","context_length":8192}]}`)
				return
			}
			reached <- struct{}{}
			<-release
			io.WriteString(w, body)
		}))
	t.Cleanup(srv.Close)

	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}

	conv := NewConversation()
	conv.SetModel("test/model")
	return &Session{
		conv:     conv,
		mainConv: conv,
		screen:   &Screen{out: out, height: 24, width: 80},
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      context.Background(),
		cancel:   func() {},
	}
}

// A compaction is work in progress and costs a request of its own, so the
// twiddle turns for it. It is stopped afterwards, since a frame drawn after
// the work ended would paint over the result.
func TestCompactionTurnsTheTwiddle(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	s := blockingSession(t, auditStream, reached, release)
	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.compact(true)
	}()

	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("the compaction request never reached the server")
	}
	if !s.spinner.Active() {
		t.Error("the twiddle is not turning while a compaction is in progress")
	}
	close(release)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the compaction did not finish")
	}
	if s.spinner.Active() {
		t.Error("the twiddle is still turning after the compaction finished")
	}
	if !s.conv.HasSummary() {
		t.Error("the compaction did not leave a summary behind")
	}
	if s.frame.Status.State != stateIdle {
		t.Errorf("state = %q, want %q once the compaction is over", s.frame.Status.State, stateIdle)
	}
}

// A summary that fails leaves the conversation exactly as it was, since a
// half-summarised history is worse than a long one. The server here answers a
// completion with a failure while the model list still resolves.
func TestFailedCompactionLeavesTheConversationAlone(t *testing.T) {
	s, _ := auditSession(t, "")
	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")
	before := s.conv.Turns()
	beforeText := s.conv.Pending("")

	s.compact(true)

	if s.conv.Turns() != before {
		t.Errorf("turns = %d, want the conversation left at %d", s.conv.Turns(), before)
	}
	after := s.conv.Pending("")
	if len(after) != len(beforeText) {
		t.Fatalf("pending = %d turns, want %d", len(after), len(beforeText))
	}
	for i := range beforeText {
		if after[i] != beforeText[i] {
			t.Errorf("turn %d = %+v, want %+v", i, after[i], beforeText[i])
		}
	}
	if s.conv.HasSummary() {
		t.Error("a summary was left behind by a compaction that failed")
	}
}
