package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// failedStreamSession returns a session whose client is pointed at a server
// that streams one delta and then closes without the terminating marker, which
// is a stream cut short. The client reports that through the callback and
// returns nil, so the send path below it cannot tell a cut stream from a
// completed one unless it looks.
func failedStreamSession(t *testing.T) *Session {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w,
				`data: {"choices":[{"delta":{"content":"the answer so far"}}]}`+"\n\n")
			// The handler returns here without the terminating marker, so the
			// body ends partway through the reply.
		}))
	t.Cleanup(srv.Close)

	conv := NewConversation()
	conv.SetModel("test/model")
	return &Session{
		conv:     conv,
		mainConv: conv,
		screen:   &Screen{},
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      context.Background(),
		cancel:   func() {},
	}
}

// A stream cut short has already shown its text and its error by the time the
// request returns. Appending the reply again painted it twice, so the reader
// saw the same paragraph above and below the error.
func TestCutStreamDoesNotPaintTheReplyTwice(t *testing.T) {
	s := failedStreamSession(t)
	s.send(s.ctx, s.conv, "hi")

	const answer = "the answer so far"
	n := 0
	for _, line := range s.frame.Reply {
		if strings.Contains(line, answer) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the reply appears %d times in %q, want once", n, s.frame.Reply)
	}
}

// The text before a failure is kept, which is why the error follows it rather
// than replacing it.
func TestCutStreamKeepsTheTextBeforeTheFailure(t *testing.T) {
	s := failedStreamSession(t)
	s.send(s.ctx, s.conv, "hi")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "the answer so far") {
		t.Errorf("pane = %q, want the text received before the failure kept", joined)
	}
	if !strings.Contains(joined, "(error)") {
		t.Errorf("pane = %q, want the failure reported", joined)
	}
}

// A turn that failed leaves no exchange behind. Recording the truncated answer
// would put a partial reply in the conversation for the next request to replay
// as though it were the whole of what the model said, which is the half
// exchange the record refuses to keep.
func TestCutStreamRecordsNoExchange(t *testing.T) {
	s := failedStreamSession(t)
	s.send(s.ctx, s.conv, "hi")

	// Pending carries the recorded turns and then the question it was given,
	// so the trailing user turn is the probe and everything before it is what
	// the conversation kept.
	recorded := s.conv.Pending("")
	for _, turn := range recorded[:len(recorded)-1] {
		if strings.Contains(turn.Content, "the answer so far") {
			t.Errorf("the truncated reply was recorded: %q", turn.Content)
		}
	}
}
