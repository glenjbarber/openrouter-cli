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

// The mode must be off until it is asked for, since a reader who did not ask
// to see the shape of a stream should not be shown it.
func TestVerboseOffByDefault(t *testing.T) {
	s := &Session{}
	if s.verboseOn() {
		t.Error("verboseOn() = true on a fresh session")
	}
}

func TestToggleVerbose(t *testing.T) {
	s := &Session{}
	s.toggleVerbose()
	if !s.verboseOn() {
		t.Error("toggle did not turn verbose on")
	}
	s.toggleVerbose()
	if s.verboseOn() {
		t.Error("toggle did not turn verbose off")
	}
}

// The toggle reports what it did, since a mode changed silently is a mode a
// reader cannot rely on having changed.
func TestToggleVerboseReports(t *testing.T) {
	s := &Session{conv: NewConversation()}
	s.toggleVerbose()
	s.toggleVerbose()

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "verbose on") {
		t.Errorf("pane = %q, want the mode reported on", joined)
	}
	if !strings.Contains(joined, "verbose off") {
		t.Errorf("pane = %q, want the mode reported off", joined)
	}
}

// A known stream must produce a detail that describes it, rather than one line
// per event. The stream below carries two deltas, a finish reason, and usage.
func TestStreamReportFromKnownStream(t *testing.T) {
	events := []openrouter.StreamEvent{
		{Kind: openrouter.EventDelta, Content: "Hel"},
		{Kind: openrouter.EventDelta, Content: "lo"},
		{Kind: openrouter.EventEnd, Finish: "stop"},
		{Kind: openrouter.EventUsage},
		{Kind: openrouter.EventDone, Done: true},
	}

	var r streamReport
	for _, e := range events {
		r.note(e)
	}

	summary := r.summary()
	for _, want := range []string{"2 deltas", "5 chars", "finish stop", "usage"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary = %q, want %q", summary, want)
		}
	}
	if strings.Contains(summary, "failed") {
		t.Errorf("summary = %q, want no failure noted", summary)
	}
}

// The summary must stay on one line. A detail that grows with the turn is the
// noise the summary exists to avoid, and a wrapped summary reads as more
// output than the reply it describes.
func TestStreamSummaryIsOneLine(t *testing.T) {
	var r streamReport
	for i := 0; i < 200; i++ {
		r.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "x"})
	}
	if strings.ContainsAny(r.summary(), "\n\r") {
		t.Errorf("summary = %q, want a single line", r.summary())
	}
}

// A stream that failed partway must say so, so a summary is never mistaken for
// a complete turn.
func TestStreamReportNotesFailure(t *testing.T) {
	var r streamReport
	r.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "part"})
	r.note(openrouter.StreamEvent{Kind: openrouter.EventError,
		Err: context.DeadlineExceeded})

	if !strings.Contains(r.summary(), "failed") {
		t.Errorf("summary = %q, want the failure noted", r.summary())
	}
}

// A turn that never delivered its terminating marker must be distinguishable
// from a complete one, since a reply that simply stops is otherwise ambiguous.
func TestStreamReportNotesUnterminated(t *testing.T) {
	var r streamReport
	r.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "hi"})
	r.note(openrouter.StreamEvent{Kind: openrouter.EventError,
		Err: context.DeadlineExceeded})

	if !strings.Contains(r.summary(), "unterminated") {
		t.Errorf("summary = %q, want the missing terminator noted", r.summary())
	}
}

// A stream that reported no accounting must say that, since a reader watching
// the counters would otherwise wait for figures that are never coming.
func TestStreamReportNotesAbsentUsage(t *testing.T) {
	var r streamReport
	r.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "hi"})
	r.note(openrouter.StreamEvent{Kind: openrouter.EventDone, Done: true})

	if !strings.Contains(r.summary(), "no usage") {
		t.Errorf("summary = %q, want the absence of usage noted", r.summary())
	}
}

// verboseSession returns a session with a screen that draws nowhere and a
// client pointed at a server streaming the given body, so that the send path
// can be exercised without a terminal.
func verboseSession(t *testing.T, body string) *Session {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, body)
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

// verboseStream is a stream with several deltas, a finish reason and usage.
const verboseStream = `data: {"choices":[{"delta":{"content":"Hel"}}]}` + "\n\n" +
	`data: {"choices":[{"delta":{"content":"lo"}}]}` + "\n\n" +
	`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":22}}` + "\n\n" +
	"data: [DONE]\n\n"

// With the mode on, the turn must leave a description of its own shape behind
// the reply.
func TestSendReportsStreamShapeWhenVerbose(t *testing.T) {
	s := verboseSession(t, verboseStream)
	s.verbose = true

	s.send(s.ctx, s.conv, "hi")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "[stream]") {
		t.Errorf("pane = %q, want the stream detail", joined)
	}
	for _, want := range []string{"2 deltas", "finish stop", "usage 11 in / 22 out"} {
		if !strings.Contains(joined, want) {
			t.Errorf("pane = %q, want %q", joined, want)
		}
	}
}

// With the mode off, the pane must be exactly what it was before the mode
// existed. This is the regression guard for the whole feature.
func TestSendUnchangedWhenVerboseOff(t *testing.T) {
	on := verboseSession(t, verboseStream)
	on.verbose = true
	on.send(on.ctx, on.conv, "hi")
	withVerbose := append([]string{}, on.frame.Reply...)

	off := verboseSession(t, verboseStream)
	off.send(off.ctx, off.conv, "hi")
	withoutVerbose := append([]string{}, off.frame.Reply...)

	if strings.Join(withVerbose, "\n") == strings.Join(withoutVerbose, "\n") {
		t.Fatal("the mode made no difference to the pane")
	}
	for _, line := range withoutVerbose {
		if strings.Contains(line, "[stream]") {
			t.Errorf("line = %q, want no stream detail when the mode is off", line)
		}
	}
	// The reply itself must be identical either way, since the mode is a
	// display preference and not a change to what was asked for.
	var replyOn, replyOff []string
	for _, l := range withVerbose {
		if l == "Hello" {
			replyOn = append(replyOn, l)
		}
	}
	for _, l := range withoutVerbose {
		if l == "Hello" {
			replyOff = append(replyOff, l)
		}
	}
	if len(replyOn) != 1 || len(replyOff) != 1 {
		t.Errorf("reply on = %v, off = %v, want the same reply either way",
			replyOn, replyOff)
	}
}

// The detail is one line added after the reply, so a turn must not otherwise
// grow the pane by the number of events it carried.
func TestStreamDetailDoesNotGrowThePane(t *testing.T) {
	few := `data: {"choices":[{"delta":{"content":"a"}}]}` + "\n\n" +
		"data: [DONE]\n\n"
	many := strings.Repeat(
		`data: {"choices":[{"delta":{"content":"x"}}]}`+"\n\n", 50) +
		"data: [DONE]\n\n"

	small := verboseSession(t, few)
	small.verbose = true
	small.send(small.ctx, small.conv, "hi")

	large := verboseSession(t, many)
	large.verbose = true
	large.send(large.ctx, large.conv, "hi")

	// One reply line, the question, the reply, and the single detail line,
	// so the rows differ by the reply length alone rather than by the number
	// of events.
	smallDetail := countDetail(t, small.frame.Reply)
	largeDetail := countDetail(t, large.frame.Reply)
	if smallDetail != 1 || largeDetail != 1 {
		t.Errorf("detail lines = %d and %d, want one each", smallDetail, largeDetail)
	}
}

// countDetail returns how many lines of the pane carry the stream detail.
func countDetail(t *testing.T, reply []string) int {
	t.Helper()
	n := 0
	for _, l := range reply {
		if strings.HasPrefix(l, "[stream]") {
			n++
		}
	}
	return n
}

// The summary must stay a bounded number of rows however long the turn was,
// since a detail that grows with the stream is the noise it exists to avoid.
func TestStreamSummaryIsBounded(t *testing.T) {
	short := streamReport{}
	short.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "x"})
	short.note(openrouter.StreamEvent{Kind: openrouter.EventEnd, Finish: "length"})

	long := streamReport{}
	for i := 0; i < 5000; i++ {
		long.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "x"})
	}

	shortRows := len(WrapBlock("[stream] "+short.summary(), 80))
	longRows := len(WrapBlock("[stream] "+long.summary(), 80))
	if shortRows != longRows {
		t.Errorf("rows = %d and %d, want the summary to fold the same either way",
			shortRows, longRows)
	}
	if shortRows > 2 {
		t.Errorf("rows = %d, want the summary kept to a couple of rows", shortRows)
	}
}

// A single event reads in the singular, since "1 deltas" reads as a fault in
// the summary rather than as a count.
func TestStreamSummaryPlural(t *testing.T) {
	var r streamReport
	r.note(openrouter.StreamEvent{Kind: openrouter.EventDelta, Content: "x"})
	if !strings.Contains(r.summary(), "1 delta") {
		t.Errorf("summary = %q, want the singular", r.summary())
	}
}
