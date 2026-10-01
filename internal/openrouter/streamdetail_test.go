package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// kindsOf streams a fixed body and returns the kind of every event delivered,
// in order. It is what the detail tests inspect, since the kind is what a
// caller watching a stream uses to tell one event from another.
func kindsOf(t *testing.T, body string) []StreamEvent {
	t.Helper()

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var events []StreamEvent
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) { events = append(events, e) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return events
}

// Every event must be labelled, since a caller cannot otherwise tell a delta
// from the accounting that arrives on the same chunk.
func TestStreamEventsCarryKind(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":22}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	var kinds []string
	for _, e := range kindsOf(t, body) {
		kinds = append(kinds, e.Kind)
	}
	want := []string{EventDelta, EventUsage, EventEnd, EventDelta, EventDone}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("kinds = %v, want %v", kinds, want)
	}
}

// The kind is a label on the event rather than a replacement for it, so the
// payload the interface already relied on must be untouched.
func TestStreamKindDoesNotChangePayload(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"hi"}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}` + "\n\n" +
		"data: [DONE]\n\n"

	events := kindsOf(t, body)

	var content string
	var usage *StreamEvent
	var done bool
	for _, e := range events {
		content += e.Content
		if e.Usage != nil {
			usage = &e
		}
		if e.Done {
			done = true
		}
	}
	if content != "hi" {
		t.Errorf("content = %q, want %q", content, "hi")
	}
	if usage == nil || usage.Usage.PromptTokens != 7 || usage.Usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v, want 7/3", usage)
	}
	if !done {
		t.Error("no Done event was delivered")
	}
}

// The reason the model gave for ending the turn is the one thing about a
// stream a reader cannot see from the text, so it must be carried.
func TestStreamCarriesFinishReason(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	var finish string
	for _, e := range kindsOf(t, body) {
		if e.Kind == EventEnd {
			finish = e.Finish
		}
	}
	if finish != "stop" {
		t.Errorf("finish = %q, want %q", finish, "stop")
	}
}

// A choice that ends the turn carries no delta, so the finish event must be
// delivered even though there is no content to deliver it with.
func TestStreamReportsFinishWithoutDelta(t *testing.T) {
	body := `data: {"choices":[{"delta":{},"finish_reason":"length"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	var kinds []string
	for _, e := range kindsOf(t, body) {
		kinds = append(kinds, e.Kind)
	}
	if len(kinds) != 2 || kinds[0] != EventEnd || kinds[1] != EventDone {
		t.Errorf("kinds = %v, want a finish then a done", kinds)
	}
}

// A stream that ends badly is labelled as an error, so a reader in verbose
// mode is not shown a summary that looks complete.
func TestStreamLabelsFailures(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"

	var last StreamEvent
	for _, e := range kindsOf(t, body) {
		last = e
	}
	if last.Kind != EventError {
		t.Errorf("kind = %q, want %q", last.Kind, EventError)
	}
	if last.Err == nil {
		t.Error("the truncation was not reported")
	}
}

// An error the stream itself carries is labelled too, since it arrives in a
// different shape from the truncation above.
func TestStreamLabelsReportedError(t *testing.T) {
	body := `data: {"error":{"message":"upstream refused"}}` + "\n\n" +
		"data: [DONE]\n\n"

	var last StreamEvent
	for _, e := range kindsOf(t, body) {
		last = e
	}
	if last.Kind != EventError {
		t.Errorf("kind = %q, want %q", last.Kind, EventError)
	}
	if last.Err == nil {
		t.Error("the reported error was not carried")
	}
}
