package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The parser is exercised against hand-written streams rather than against a
// server wherever the point is what the parser does with a body, since a
// server writes a well-formed stream for free and proves nothing.

func parseStream(t *testing.T, body string) []StreamEvent {
	t.Helper()
	c := New("https://example.invalid", "k")
	var events []StreamEvent
	if err := c.readStream(strings.NewReader(body), func(e StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("readStream: %v", err)
	}
	return events
}

// The kinds of the events, which is what a caller watching the stream reads.
func kinds(events []StreamEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

func joined(events []StreamEvent) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString(e.Content)
	}
	return b.String()
}

// A well-formed stream: every kind is delivered, in order, and the terminating
// marker ends it.
func TestParserReadsAWellFormedStream(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: [DONE]`,
		``,
		`data: {"choices":[{"delta":{"content":"after"}}]}`,
		``,
	}, "\n")

	events := parseStream(t, body)
	if got := strings.Join(kinds(events), ","); got != "delta,delta,done" {
		t.Errorf("kinds = %v, want the two deltas and the terminator", events)
	}
	if joined(events) != "Hello" {
		t.Errorf("content = %q, want %q and nothing past the terminator", joined(events), "Hello")
	}
}

// A stream cut short before the marker is reported rather than presented as a
// complete reply, and the text that did arrive is kept.
func TestParserReportsAStreamWithNoTerminator(t *testing.T) {
	events := parseStream(t, `data: {"choices":[{"delta":{"content":"half"}}]}`+"\n\n")
	if joined(events) != "half" {
		t.Errorf("content = %q, want the text kept", joined(events))
	}
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Errorf("last = %+v, want the truncation reported", last)
	}
}

// A comment is a keep-alive and carries no payload, wherever it falls.
func TestParserIgnoresComments(t *testing.T) {
	body := ": ping\n" +
		`data: {"choices":[{"delta":{"content":"a"}}]}` + "\n\n" +
		": another\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if got := strings.Join(kinds(events), ","); got != "delta,done" {
		t.Errorf("kinds = %v, want a keep-alive to carry nothing", kinds(events))
	}
	if joined(events) != "a" {
		t.Errorf("content = %q, want %q", joined(events), "a")
	}
}

// A field this parser does not know is passed over, so a stream carrying one
// still reads.
func TestParserIgnoresUnknownFields(t *testing.T) {
	body := "event: ping\nid: 42\nretry: 1000\n" +
		`: keep-alive` + "\n\n" +
		`data: {"choices":[{"delta":{"content":"a"}}],"id":"gen-1","object":"chat.completion.chunk"}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if got := strings.Join(kinds(events), ","); got != "delta,done" {
		t.Errorf("kinds = %v, want an unknown field to carry nothing", kinds(events))
	}
	if joined(events) != "a" {
		t.Errorf("content = %q, want %q", joined(events), "a")
	}
}

// A data field holding something that is not JSON is reported rather than
// skipped, since a stream that cannot be read is a stream that was cut.
func TestParserReportsInvalidJSON(t *testing.T) {
	events := parseStream(t, "data: {not json\n\ndata: [DONE]\n\n")
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Fatalf("last = %+v, want the failure reported", last)
	}
	if !strings.Contains(last.Err.Error(), "decoding a stream event") {
		t.Errorf("err = %q, want it to name the decoding", last.Err)
	}
	for _, e := range events {
		if e.Done {
			t.Error("a stream that failed to decode was reported as complete")
		}
	}
}

// An accounting chunk carrying no content is delivered on its own, since it is
// the only place the figures arrive.
func TestParserDeliversAccountingWithNoContent(t *testing.T) {
	body := `data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":7}}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if got := strings.Join(kinds(events), ","); got != "usage,done" {
		t.Fatalf("kinds = %v, want the accounting alone", kinds(events))
	}
	if events[0].Usage == nil || events[0].Usage.PromptTokens != 5 ||
		events[0].Usage.CompletionTokens != 7 {
		t.Errorf("usage = %+v, want 5/7", events[0].Usage)
	}
}

// A content chunk carrying no accounting reports none, since a zero would
// clear an accumulated total.
func TestParserOmitsAbsentAccounting(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"a"}}],"usage":null}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	for _, e := range events {
		if e.Usage != nil {
			t.Errorf("usage = %+v, want nil when the chunk carries none", e.Usage)
		}
	}
	if joined(events) != "a" {
		t.Errorf("content = %q, want %q", joined(events), "a")
	}
}

// The delta is null on a chunk the endpoint sends for a tool call, which is a
// shape this parser must not treat as a failure.
func TestParserToleratesANullDelta(t *testing.T) {
	body := `data: {"choices":[{"delta":null,"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if got := strings.Join(kinds(events), ","); got != "finish,done" {
		t.Errorf("kinds = %v, want a null delta to carry no content", kinds(events))
	}
	if joined(events) != "" {
		t.Errorf("content = %q, want none", joined(events))
	}
}

// The marker is written without a space after the field name by some servers,
// and is read either way.
func TestParserReadsTheMarkerWithoutASpace(t *testing.T) {
	events := parseStream(t, "data:[DONE]\n\n")
	if len(events) != 1 || !events[0].Done {
		t.Errorf("events = %+v, want the terminator", events)
	}
}

// A failure reported inside the stream arrives in a different shape from a
// truncation, and both must end the stream without being reported as complete.
func TestParserReportsAnErrorCarriedInTheStream(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"part"}}]}` + "\n\n" +
		`data: {"error":{"message":"upstream refused"}}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if joined(events) != "part" {
		t.Errorf("content = %q, want the text before the failure kept", joined(events))
	}
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Fatalf("last = %+v, want the reported error", last)
	}
	if !strings.Contains(last.Err.Error(), "upstream refused") {
		t.Errorf("err = %q, want the message", last.Err)
	}
	for _, e := range events {
		if e.Done {
			t.Error("a stream that reported an error was reported as complete")
		}
	}
}

// A delta split across two reads is one delta. A terminal writes a whole line
// in one piece, but a link can divide one, and a parser that read a byte at a
// time would hand half a delta to the caller as the start of the next.
func TestParserJoinsADeltaSplitAcrossReads(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("the test server does not flush")
			return
		}
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"split`)
		flusher.Flush()
		time.Sleep(20 * time.Millisecond)
		io.WriteString(w, `_reply"}}]}`+"\n\ndata: [DONE]\n\n")
		flusher.Flush()
	})

	var got strings.Builder
	var done bool
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Done {
			done = true
		}
		got.WriteString(e.Content)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "split_reply" {
		t.Errorf("content = %q, want the two halves joined", got.String())
	}
	if !done {
		t.Error("no Done event was delivered")
	}
}

// A line longer than the scanner buffer ends the stream, since the buffer is
// finite and the line is not. The text that arrived before it is still worth
// having, so the failure is reported the way every other failure on this path
// is reported: through the callback, where a caller already knows to keep what
// it has. Returning it instead would reach a caller that drops the reply it has
// already drawn on the screen.
type cutMidEvent struct {
	parts []string
	fail  error
}

func (r *cutMidEvent) Read(p []byte) (int, error) {
	if len(r.parts) == 0 {
		return 0, r.fail
	}
	n := copy(p, r.parts[0])
	r.parts[0] = r.parts[0][n:]
	if r.parts[0] == "" {
		r.parts = r.parts[1:]
	}
	return n, nil
}

// overlongBody is a complete event followed by a line far longer than any
// delta a model writes, delivered in pieces and then cut.
func overlongBody() *cutMidEvent {
	return &cutMidEvent{
		parts: []string{
			`data: {"choices":[{"delta":{"content":"kept"}}]}` + "\n\n",
			strings.Repeat("y", 4096),
			strings.Repeat("z", 9<<20),
		},
		fail: errors.New("connection reset by peer"),
	}
}

func TestParserReportsAFailedReadAndKeepsTheText(t *testing.T) {
	c := New("https://example.invalid", "k")
	body := overlongBody()

	var events []StreamEvent
	err := c.readStream(body, func(e StreamEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("readStream returned %v, want the failure reported to the callback", err)
	}
	if joined(events) != "kept" {
		t.Errorf("content = %q, want the text before the failure kept", joined(events))
	}
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Fatalf("last = %+v, want the failed read reported", last)
	}
	for _, e := range events {
		if e.Done {
			t.Error("a stream whose read failed was reported as complete")
		}
	}
}

// A server that answers with a status other than 200 is reported with the
// detail it gave, and the body of that response is read for the message rather
// than parsed as a stream.
func TestChatReportsANonSuccessStatus(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, `<html>maintenance until 12:00</html>`)
	})

	var events int
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) { events++ })
	if err == nil {
		t.Fatal("Chat accepted a 503, want an error")
	}
	if !strings.Contains(err.Error(), "maintenance until 12:00") {
		t.Errorf("err = %q, want the detail", err)
	}
	if events != 0 {
		t.Errorf("the callback ran %d times on an error response, want 0", events)
	}
}

// An error response with no body at all says so rather than reporting an empty
// message, which would read as a blank reason.
func TestChatReportsAnErrorWithNoBody(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil {
		t.Fatal("Chat accepted a 502, want an error")
	}
	if !strings.Contains(err.Error(), "no detail was given") {
		t.Errorf("err = %q, want the absent detail stated", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("err = %q, want the status named", err)
	}
}

// The status of a successful reply is examined without the body being read
// first, since reading it would consume the reply before the parser saw it. A
// server that reports a 200 and then writes the stream is the case; if the
// status were checked by reading, the deltas would be gone.
func TestChatDoesNotConsumeTheStreamToCheckTheStatus(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `data: {"choices":[{"delta":{"content":"only once"}}]}`+"\n\n")
		io.WriteString(w, "data: [DONE]\n\n")
	})

	var got strings.Builder
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) { got.WriteString(e.Content) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "only once" {
		t.Errorf("content = %q, want the stream to survive the status check", got.String())
	}
}

// An accounting figure too large for the platform is clamped rather than
// refused, since a decode failure in this chunk ends the stream and throws away
// a reply that has already arrived.
func TestParserClampsAnAccountingFigureThatWillNotFit(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"kept"}}],"usage":{"prompt_tokens":1e30,"completion_tokens":-1e30}}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if joined(events) != "kept" {
		t.Errorf("content = %q, want the reply kept", joined(events))
	}
	var usage *streamUsage
	for _, e := range events {
		if e.Usage != nil {
			usage = e.Usage
		}
	}
	if usage == nil {
		t.Fatal("no usage was delivered")
	}
	if usage.PromptTokens <= 0 {
		t.Errorf("prompt = %d, want the figure clamped to the platform maximum", usage.PromptTokens)
	}
	if usage.CompletionTokens >= 0 {
		t.Errorf("completion = %d, want the figure clamped to the platform minimum", usage.CompletionTokens)
	}
}

// The accounting may be written as a string or in exponent form, and neither is
// refused, since the endpoint is not under this repository's control.
func TestParserReadsAccountingInOtherShapes(t *testing.T) {
	for _, body := range []string{
		`data: {"choices":[],"usage":{"prompt_tokens":"12","completion_tokens":"1e3"}}`,
		`data: {"choices":[],"usage":{"prompt_tokens":1.2e1,"completion_tokens":1e3}}`,
	} {
		events := parseStream(t, body+"\n\ndata: [DONE]\n\n")
		usage := events[0].Usage
		if usage == nil {
			t.Fatalf("%s delivered no usage", body)
		}
		if usage.PromptTokens != 12 || usage.CompletionTokens != 1000 {
			t.Errorf("%s decoded as %+v, want 12/1000", body, usage)
		}
	}
}

// An accounting object carrying no figures at all is delivered as a reported
// zero rather than dropped, since the pointer is what distinguishes it from a
// chunk that carries no accounting.
func TestParserDeliversAnEmptyAccountingObject(t *testing.T) {
	events := parseStream(t, `data: {"choices":[],"usage":{}}`+"\n\ndata: [DONE]\n\n")
	usage := events[0].Usage
	if usage == nil {
		t.Fatal("no usage was delivered")
	}
	if *usage != (streamUsage{}) {
		t.Errorf("usage = %+v, want a reported zero", *usage)
	}
}

// A count is delivered as a number that fits the platform, unchanged.
func TestParserReadsAWholeCount(t *testing.T) {
	var u streamUsage
	if err := json.Unmarshal([]byte(`{"prompt_tokens":123456,"completion_tokens":789}`), &u); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if u.PromptTokens != 123456 || u.CompletionTokens != 789 {
		t.Errorf("usage = %+v, want 123456/789", u)
	}
}

// A finish reason on a choice with no delta still ends the turn, and the reason
// is the one thing about a stream a reader cannot see in the text.
func TestParserCarriesAFinishReason(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"a"},"finish_reason":"length"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if len(events) != 3 {
		t.Fatalf("events = %+v, want the finish, the delta and the terminator", events)
	}
	// The finish is announced before the delta of the same choice, since it
	// says the turn is over and the delta belongs to the turn that ended.
	if events[0].Kind != EventEnd || events[0].Finish != "length" {
		t.Errorf("event = %+v, want the finish reason", events[0])
	}
	if events[1].Kind != EventDelta || events[1].Content != "a" {
		t.Errorf("event = %+v, want the delta", events[1])
	}
}

// A stream of many choices is delivered in order, since the text is written
// from these events and an out of order reply reads as nonsense.
func TestParserKeepsSeveralChoicesInOrder(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"one"}},{"delta":{"content":"two"}}]}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if joined(events) != "onetwo" {
		t.Errorf("content = %q, want the choices in order", joined(events))
	}
}

// The usage is asked for explicitly and the stream is asked for explicitly,
// since the endpoint sends neither unless it is told to.
func TestChatAsksForAStreamAndForAccounting(t *testing.T) {
	var sent map[string]any
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&sent)
		io.WriteString(w, "data: [DONE]\n\n")
	})

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if sent["stream"] != true {
		t.Errorf("stream = %v, want true in the request", sent["stream"])
	}
	if _, ok := sent["usage"]; !ok {
		t.Errorf("request = %v, want the accounting asked for", sent)
	}
}

// Every event carries a kind, since a caller cannot otherwise tell a delta from
// the accounting that arrives on the same chunk.
func TestEveryEventCarriesAKind(t *testing.T) {
	bodies := []string{
		`data: {"choices":[{"delta":{"content":"a"}}]}` + "\n\ndata: [DONE]\n\n",
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}` + "\n\ndata: [DONE]\n\n",
		`data: {not json` + "\n\n",
		`data: {"error":{"message":"no"}}` + "\n\n",
		`data: {"choices":[{"delta":{"content":"a"}}]}` + "\n\n",
		"",
	}
	for _, body := range bodies {
		for _, e := range parseStream(t, body) {
			if e.Kind == "" {
				t.Errorf("an event of %q carried no kind: %+v", body, e)
			}
		}
	}
}

// The read deadline on a stream is the client's, not the parser's, so a reply
// that keeps arriving is delivered however long it takes.
func TestStreamIsReadWithTheCallersContext(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		for _, word := range []string{"one", "two", "three"} {
			fmt.Fprintf(w, `data: {"choices":[{"delta":{"content":%q}}]}`+"\n\n", word)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
		io.WriteString(w, "data: [DONE]\n\n")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var got strings.Builder
	err := c.Chat(ctx, ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) { got.WriteString(e.Content) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "onetwothree" {
		t.Errorf("content = %q, want all three deltas", got.String())
	}
}

// A single line longer than the buffer ends the reply, since the buffer is
// finite and the line is not. The text that arrived before it is kept and the
// end is reported through the callback, so a reader is never left with a reply
// that stops mid-sentence and nothing saying that it did. The limit is named in
// the source so that this test exceeds the figure the parser actually uses
// rather than a copy of it that could drift.
func TestParserReportsALineLongerThanTheBuffer(t *testing.T) {
	c := New("https://example.invalid", "k")
	body := oneDelta + "\n\n" +
		"data: " + strings.Repeat("y", maxStreamLine) + "\n\n" +
		"data: [DONE]\n\n"

	var events []StreamEvent
	if err := c.readStream(strings.NewReader(body), func(e StreamEvent) {
		events = append(events, e)
	}); err != nil {
		t.Fatalf("readStream returned %v, want the failure reported to the callback", err)
	}
	if joined(events) != "x" {
		t.Errorf("content = %q, want the text before the long line kept", joined(events))
	}
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Fatalf("last = %+v, want the long line reported", last)
	}
	if !strings.Contains(last.Err.Error(), "reading the stream") {
		t.Errorf("err = %q, want it to name the failed read", last.Err)
	}
	for _, e := range events {
		if e.Done {
			t.Error("a reply cut by a long line was reported as complete")
		}
	}
}

// A line that fits the buffer is read whole however long it is, since the bound
// is there to stop an endless line rather than to cut a long one.
func TestParserReadsALineAsLongAsTheBufferAllows(t *testing.T) {
	content := strings.Repeat("a", 1<<20)
	body := `data: {"choices":[{"delta":{"content":"` + content + `"}}]}` + "\n\n" +
		"data: [DONE]\n\n"

	events := parseStream(t, body)
	if len(joined(events)) != len(content) {
		t.Errorf("content is %d characters, want the whole delta of %d",
			len(joined(events)), len(content))
	}
	if last := events[len(events)-1]; !last.Done {
		t.Errorf("last = %+v, want the terminator after a long but readable line", last)
	}
}
