package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// toolEvents streams a fixed body and returns every event delivered, in order.
// A call arrives on an event of its own, so the whole set is what the tool tests
// read rather than the kinds alone.
func toolEvents(t *testing.T, body string) []StreamEvent {
	t.Helper()

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var events []StreamEvent
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "read .env"}},
	}, func(e StreamEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return events
}

// eventsOfKind returns the events carrying one kind, in order.
func eventsOfKind(events []StreamEvent, kind string) []StreamEvent {
	var out []StreamEvent
	for _, e := range events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// A call arrives in pieces: the identifier and the name come once, and the
// arguments are spread across the deltas after it. The caller is handed the
// whole call once, since the transport is the one place that knows how the
// pieces were framed.
func TestStreamDeliversAToolCallOnce(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function",` +
			`"function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\".e"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"nv\"}"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	calls := eventsOfKind(events, EventTool)
	if len(calls) != 1 {
		t.Fatalf("kinds = %v, want one tool event", kinds(events))
	}
	if got := calls[0].ToolCalls; len(got) != 1 {
		t.Fatalf("ToolCalls = %+v, want one call", got)
	}
	call := calls[0].ToolCalls[0]
	if call.ID != "call_1" || call.Type != ToolTypeFunction {
		t.Errorf("call = %+v, want the identifier and type of the first fragment", call)
	}
	if call.Function.Name != "read_file" {
		t.Errorf("name = %q, want %q", call.Function.Name, "read_file")
	}
	if want := `{"path":".env"}`; call.Function.Arguments != want {
		t.Errorf("arguments = %q, want %q", call.Function.Arguments, want)
	}
	if got := strings.Join(kinds(events), ","); got != "finish,tool,done" {
		t.Errorf("kinds = %v, want the calls between the finish and the terminator", events)
	}
}

// A turn that called nothing reports nothing, since an event carrying an empty
// set of calls is one every caller would have to check before it could act.
func TestStreamDeliversNoToolEventWhenNoToolWasCalled(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	if len(eventsOfKind(events, EventTool)) != 0 {
		t.Errorf("kinds = %v, want no tool event", kinds(events))
	}
	if got := strings.Join(kinds(events), ","); got != "delta,done" {
		t.Errorf("kinds = %v, want the delta and the terminator", events)
	}
}

// A call whose name never arrived is not a call, since there would be nothing to
// run, and the turn reports having called nothing.
func TestStreamDropsAToolCallWithNoName(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function",` +
			`"function":{"arguments":"{\"pa"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a\"}"}}]}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	if len(eventsOfKind(events, EventTool)) != 0 {
		t.Errorf("kinds = %v, want no tool event", kinds(events))
	}
	if got := strings.Join(kinds(events), ","); got != "done" {
		t.Errorf("kinds = %v, want the terminator alone", events)
	}
}

// Two calls in one turn are interleaved on the wire, so each has to be rebuilt
// from its own fragments rather than from the stream in the order it arrived.
func TestStreamDeliversTwoToolCallsInOrder(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function",` +
			`"function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function",` +
			`"function":{"name":"list_dir","arguments":"{\"pa"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a\"}"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"th\":\"b\"}"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	calls := eventsOfKind(events, EventTool)
	if len(calls) != 1 {
		t.Fatalf("kinds = %v, want one tool event carrying both calls", kinds(events))
	}
	want := []struct{ id, name, args string }{
		{"call_a", "read_file", `{"path":"a"}`},
		{"call_b", "list_dir", `{"path":"b"}`},
	}
	got := calls[0].ToolCalls
	if len(got) != len(want) {
		t.Fatalf("ToolCalls = %+v, want %d calls", got, len(want))
	}
	for i, w := range want {
		if got[i].Index != i {
			t.Errorf("call %d has index %d, want %d", i, got[i].Index, i)
		}
		if got[i].ID != w.id || got[i].Function.Name != w.name || got[i].Function.Arguments != w.args {
			t.Errorf("call %d = %+v, want %s/%s/%s", i, got[i], w.id, w.name, w.args)
		}
	}

	var finish string
	for _, e := range events {
		if e.Kind == EventEnd {
			finish = e.Finish
		}
	}
	if finish != "tool_calls" {
		t.Errorf("finish = %q, want %q", finish, "tool_calls")
	}
}

// The accounting arrives on the last chunk before the terminator, and the calls
// are delivered between it and the terminator, so that a caller has the figures
// in hand before it is asked to run anything.
func TestStreamDeliversUsageAlongsideAToolCall(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function",` +
			`"function":{"name":"read_file","arguments":"{}"}}]}}]}`,
		``,
		`data: {"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":13}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	if got := strings.Join(kinds(events), ","); got != "usage,tool,done" {
		t.Fatalf("kinds = %v, want the accounting, then the calls, then the terminator", events)
	}
	if events[0].Usage == nil || events[0].Usage.PromptTokens != 9 || events[0].Usage.CompletionTokens != 13 {
		t.Errorf("usage = %+v, want 9/13", events[0].Usage)
	}
	if len(events[1].ToolCalls) != 1 {
		t.Errorf("ToolCalls = %+v, want the call", events[1].ToolCalls)
	}
}

// A call cut short by a stream that ends before the terminator is not a call,
// since the model never finished asking for it. It is dropped rather than
// handed over as one, and the text that did arrive is kept.
func TestStreamDropsAToolCallFromACutStream(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"reading","tool_calls":[{"index":0,"id":"call_1",` +
			`"type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
		``,
	}, "\n")

	events := toolEvents(t, body)

	if len(eventsOfKind(events, EventTool)) != 0 {
		t.Errorf("kinds = %v, want no tool event", kinds(events))
	}
	if got := joined(events); got != "reading" {
		t.Errorf("content = %q, want the text kept", got)
	}
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Errorf("last = %+v, want the truncation reported", last)
	}
}
