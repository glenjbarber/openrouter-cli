package openrouter

import (
	"encoding/json"
	"testing"
)

// A session offering no tools sends the request it sent before tools existed.
// The field is omitted rather than sent empty, so a client with no tools is not
// a client asking the model to consider none, which is a different question.
func TestChatRequestWithoutToolsOmitsTheKey(t *testing.T) {
	body, err := json.Marshal(ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"model":"test/model","messages":[{"role":"user","content":"hi"}]}`
	if string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

// The request as it goes on the wire, since the accounting is added to the body
// alongside the request and must not drag a tools key in with it.
func TestChatRequestOnTheWireCarriesNoToolsKey(t *testing.T) {
	body, err := json.Marshal(withUsage(ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := sent["tools"]; ok {
		t.Errorf("body = %s, want no tools key", body)
	}
}

// A tool is sent under the names the endpoint reads, with the schema carried as
// it was written rather than decoded and written back out again.
func TestChatRequestSendsToolsAsTheWireExpects(t *testing.T) {
	body, err := json.Marshal(ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "read .env"}},
		Tools: []Tool{{
			Type: ToolTypeFunction,
			Function: ToolFunction{
				Name:        "read_file",
				Description: "read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"model":"test/model","messages":[{"role":"user","content":"read .env"}],` +
		`"tools":[{"type":"function","function":{"name":"read_file","description":"read a file",` +
		`"parameters":{"type":"object","properties":{"path":{"type":"string"}}}}}]}`
	if string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

// A call is replayed on the assistant turn that asked for it, and a result is
// addressed to the call it answers. Both must reach the wire, or the model
// cannot tell which result belongs to which call it set in motion.
func TestMessageCarriesToolCallsAndTheirResults(t *testing.T) {
	body, err := json.Marshal([]Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{{
				Index: 0,
				ID:    "call_1",
				Type:  ToolTypeFunction,
				Function: ToolCallFunction{
					Name:      "read_file",
					Arguments: `{"path":".env"}`,
				},
			}},
		},
		{
			Role:       RoleTool,
			Content:    "hello",
			ToolCallID: "call_1",
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `[{"role":"assistant","content":"","tool_calls":[{"index":0,"id":"call_1",` +
		`"type":"function","function":{"name":"read_file","arguments":"{\"path\":\".env\"}"}}]},` +
		`{"role":"tool","content":"hello","tool_call_id":"call_1"}]`
	if string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}

// A message carrying neither calls nor a result says nothing about tools, since
// a field sent empty is a field the endpoint has to read past on every turn of
// every conversation.
func TestMessageOmitsTheToolFieldsWhenAbsent(t *testing.T) {
	body, err := json.Marshal(Message{Role: RoleUser, Content: "hi"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"role":"user","content":"hi"}`
	if string(body) != want {
		t.Errorf("body = %s, want %s", body, want)
	}
}
