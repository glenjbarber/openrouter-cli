package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The reply arrives, the twiddle stops, and the frame that carries the reply
// in place with the status back to idle is never written. The final repaint of
// a turn lands inside the rate bound, so it is deferred rather than drawn, and
// it is owed to a stream that has finished and a twiddle that has been
// stopped. The reader is left looking at the last twiddle frame until they type
// something, at which point the whole reply appears at once.
//
// The screen here is a real one writing to a file, so what reaches the
// terminal is read back rather than inferred from the fields on the session.
// Nothing here stubs out the coalescing, since the coalescing is the defect.
func TestReplyIsPaintedWhenTheTurnEnds(t *testing.T) {
	s, capture := auditSession(t, auditStream)

	s.send(s.ctx, s.conv, "hello")

	frame := auditLastFrame(t, capture)
	if !strings.Contains(frame, "Hello") {
		t.Errorf("the last frame drawn does not carry the reply.\n%s", frame)
	}
	if !strings.Contains(frame, "Status: idle") {
		t.Errorf("the last frame drawn does not carry the idle state.\n%s", frame)
	}
	if strings.Contains(frame, "Working") {
		t.Errorf("the last frame drawn still reports work in progress.\n%s", frame)
	}
	if f := auditTwiddleIn(frame); f != "" {
		t.Errorf("the last frame drawn carries the twiddle %q, want none.\n%s", f, frame)
	}
}

// A repaint deferred by the rate bound must still reach the terminal, since
// nothing else is left to ask for it once the request that wanted it is over.
// A wheel notch, a keystroke and the end of a delegate all ask for a repaint
// that the bound may defer, and each of them is the last thing that happens.
func TestDeferredRepaintReachesTheScreenWithoutAnotherRequest(t *testing.T) {
	s, capture := auditSession(t, auditStream)

	s.appendLines("first")
	s.draw()
	s.appendLines("second")
	s.draw()
	s.appendLines("third")
	s.draw()

	// Nothing asks for a repaint from here. The frame on screen must settle
	// on the newest state by itself.
	time.Sleep(4 * minPaintInterval)

	frame := auditLastFrame(t, capture)
	if !strings.Contains(frame, "third") {
		t.Errorf("the last frame drawn is not the newest state, so a repaint "+
			"deferred by the rate bound was never delivered.\n%s", frame)
	}
}

// A turn refused before it is sent is still a turn that ended, and the reader
// is owed the frame saying so. The two early exits are separate because they
// report different things, and a session missing a key is the ordinary case
// this path exists for.
func TestRefusedTurnIsPainted(t *testing.T) {
	s, capture := auditSession(t, auditStream)
	s.client = nil

	s.send(s.ctx, s.conv, "hello")

	frame := auditLastFrame(t, capture)
	if !strings.Contains(frame, "no API key is configured") {
		t.Errorf("the last frame drawn does not report the missing key.\n%s", frame)
	}
}

// The same holds for a turn refused because no model has been chosen, which is
// a different refusal arriving on a different exit.
func TestTurnWithNoModelIsPainted(t *testing.T) {
	s, capture := auditSession(t, auditStream)
	s.conv.SetModel("")

	s.send(s.ctx, s.conv, "hello")

	frame := auditLastFrame(t, capture)
	if !strings.Contains(frame, "no model is selected") {
		t.Errorf("the last frame drawn does not report the missing model.\n%s", frame)
	}
}

// A turn that fails must also end on a painted frame. A frame left showing
// work in progress after the request is over says the client is still working,
// which is the one thing it is not.
func TestFailedTurnIsPainted(t *testing.T) {
	s, capture := auditSession(t, "")

	s.send(s.ctx, s.conv, "hello")

	frame := auditLastFrame(t, capture)
	if !strings.Contains(frame, "(error)") {
		t.Errorf("the last frame drawn does not carry the failure.\n%s", frame)
	}
	if strings.Contains(frame, "Working") {
		t.Errorf("the last frame drawn still reports work in progress.\n%s", frame)
	}
	if f := auditTwiddleIn(frame); f != "" {
		t.Errorf("the last frame drawn carries the twiddle %q, want none.\n%s", f, frame)
	}
}

// Nothing may be written to the terminal once it has been restored, since a
// repaint that lands afterwards draws over a shell. The deferred repaint is the
// path that could do it, since it is the one that runs on its own.
func TestNothingIsPaintedAfterClose(t *testing.T) {
	s, capture := auditSession(t, auditStream)

	s.draw()
	s.appendLines("after")
	s.draw()
	s.Close()

	at := len(capture())
	time.Sleep(4 * minPaintInterval)
	if got := len(capture()); got != at {
		t.Errorf("the terminal was written %d bytes after Close", got-at)
	}
}

// auditStream is a stream carrying a reply in two deltas, a finish reason and
// usage, which is the shape a real turn arrives in.
const auditStream = `data: {"choices":[{"delta":{"content":"Hel"}}]}` + "\n\n" +
	`data: {"choices":[{"delta":{"content":"lo"}}]}` + "\n\n" +
	`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":22}}` + "\n\n" +
	"data: [DONE]\n\n"

// auditSession returns a session pointed at a server streaming the given body,
// with a screen that writes to a file, and a function reading back what the
// terminal would have received.
//
// The screen is a real one rather than a stub, since the coalescing, the
// rendering and the writes are what is under test, and a stub would leave the
// defect in place while the test passed.
func auditSession(t *testing.T, body string) (*Session, func() string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
				return
			}
			if body == "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			io.WriteString(w, body)
		}))
	t.Cleanup(srv.Close)

	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}
	// The size is set rather than read, since a file is not a terminal and the
	// size query would fail and leave the frame without any rows.
	screen := &Screen{out: out, height: 24, width: 80}
	capture := func() string {
		b, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatalf("reading the capture: %v", err)
		}
		return string(b)
	}

	conv := NewConversation()
	conv.SetModel("test/model")
	return &Session{
		conv:     conv,
		mainConv: conv,
		screen:   screen,
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      context.Background(),
		cancel:   func() {},
	}, capture
}

// auditLastFrame returns the rows of the last frame drawn on the screen.
//
// A frame is written as a home, its rows, a home, and the cursor position, so
// splitting the capture on the home leaves the rows of the last frame in the
// element before the trailing empty one.
func auditLastFrame(t *testing.T, capture func() string) string {
	t.Helper()
	parts := strings.Split(capture(), seqHome)
	if len(parts) < 2 {
		t.Fatalf("no frame was drawn at all")
	}
	return parts[len(parts)-2]
}

// auditTwiddleIn returns the twiddle character the frame carries, or an empty
// string when it carries none. The twiddle is plain text in the frame, since a
// selection has to yield it as such.
func auditTwiddleIn(frame string) string {
	for _, f := range spinnerFrames {
		if strings.Contains(frame, f) {
			return f
		}
	}
	return ""
}
