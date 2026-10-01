package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestServer returns a client pointed at a test server.
func newTestServer(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, "test-key"), srv
}

func TestChatStreamsDeltas(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want a bearer credential", got)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept = %q, want an event stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, body)
	})

	var got strings.Builder
	var done bool
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Done {
			done = true
			return
		}
		got.WriteString(e.Content)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "Hello world" {
		t.Errorf("content = %q, want %q", got.String(), "Hello world")
	}
	if !done {
		t.Error("no Done event was delivered")
	}
}

// A comment line is a keep-alive and carries no payload.
func TestChatIgnoresComments(t *testing.T) {
	body := ": keep-alive\n" +
		`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n" +
		"data: [DONE]\n\n"

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var got strings.Builder
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) { got.WriteString(e.Content) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "ok" {
		t.Errorf("content = %q, want %q", got.String(), "ok")
	}
}

// A stream that ends without the terminating marker must not be presented as
// a complete reply.
func TestChatReportsTruncatedStream(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var got strings.Builder
	var streamErr error
	var sawDone bool
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Err != nil {
			streamErr = e.Err
		}
		if e.Done {
			sawDone = true
		}
		got.WriteString(e.Content)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if sawDone {
		t.Error("a truncated stream was reported as complete")
	}
	if streamErr == nil {
		t.Error("streamErr = nil, want the truncation reported")
	}
	if got.String() != "partial" {
		t.Errorf("content = %q, want the partial text kept", got.String())
	}
}

func TestChatReportsUnauthorized(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"No auth credentials found"}}`)
	})

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil {
		t.Fatal("Chat accepted a 401, want an error")
	}
	if !strings.Contains(err.Error(), "No auth credentials found") {
		t.Errorf("err = %q, want the backend detail", err)
	}
}

func TestChatReportsRateLimit(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"limit reached"}}`)
	})

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil || !strings.Contains(err.Error(), "usage limit") {
		t.Errorf("err = %v, want a rate limit error", err)
	}
}

// An empty key is caught locally rather than sent, since the backend answers
// it with a 401 that does not distinguish the two mistakes.
func TestChatRejectsEmptyKey(t *testing.T) {
	c := New("https://example.invalid", "")
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err != ErrNoKey {
		t.Errorf("err = %v, want ErrNoKey", err)
	}
}

func TestChatRequiresModel(t *testing.T) {
	c := New("https://example.invalid", "k")
	err := c.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil {
		t.Error("Chat accepted an empty model, want an error")
	}
}

func TestChatRequiresMessages(t *testing.T) {
	c := New("https://example.invalid", "k")
	err := c.Chat(context.Background(), ChatRequest{Model: "m"}, func(StreamEvent) {})
	if err == nil {
		t.Error("Chat accepted no messages, want an error")
	}
}

func TestModels(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		io.WriteString(w, `{"data":[{"id":"a/b","name":"A","context_length":128000}]}`)
	})

	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].ID != "a/b" || models[0].ContextLength != 128000 {
		t.Errorf("model = %+v, want the decoded fields", models[0])
	}
}

func TestKeyUsage(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key" {
			t.Errorf("path = %q, want /key", r.URL.Path)
		}
		io.WriteString(w, `{"data":{"usage":0.5,"limit":1,
			"free_model_daily_requests":{"used":3,"limit":1000}}}`)
	})

	usage, err := c.KeyUsage(context.Background())
	if err != nil {
		t.Fatalf("KeyUsage: %v", err)
	}
	if usage.Usage != 0.5 || usage.Limit != 1 {
		t.Errorf("usage = %v/%v, want 0.5/1", usage.Usage, usage.Limit)
	}
	if usage.FreeModelRequests == nil || usage.FreeModelRequests.Used != 3 {
		t.Errorf("free requests = %+v, want 3 used", usage.FreeModelRequests)
	}
}

// The key endpoint omits the free allowance for a key without free access, and
// that must not be reported as an error.
func TestKeyUsageWithoutFreeAllowance(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":{"usage":1,"limit":1}}`)
	})

	usage, err := c.KeyUsage(context.Background())
	if err != nil {
		t.Fatalf("KeyUsage: %v", err)
	}
	if usage.FreeModelRequests != nil {
		t.Errorf("free requests = %+v, want nil", usage.FreeModelRequests)
	}
}

// An unknown field in a response must not break decoding, so a newer endpoint
// stays readable.
func TestDecodesUnknownFields(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"usage": 1, "limit": 2, "future": true},
		})
	})

	if _, err := c.KeyUsage(context.Background()); err != nil {
		t.Fatalf("KeyUsage: %v", err)
	}
}
