package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// A turn where the model calls a tool makes more than one request, and the
// second carries the result of the call. Without that the model is asked the
// same question again and calls the same file again, which is the loop a reader
// would report as the client being stuck.
func TestACalledToolIsRunAndItsResultSentBack(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		first := len(body["messages"].([]any)) == 1
		mu.Unlock()
		if first {
			io.WriteString(w, toolCallStream("list_dir", map[string]any{"path": "."}))
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	// The directory is given something in it, since a listing of an empty
	// directory is legitimately empty and would make the assertion about the
	// answer prove nothing.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	s := toolSession(t, srv.URL, dir)
	s.send(s.ctx, s.conv, "what is in here")

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("the turn made %d requests, want 2", len(bodies))
	}
	if _, offered := bodies[0]["tools"]; !offered {
		t.Error("the first request offered no tools, want the tools the session has")
	}
	var answered bool
	for _, m := range bodies[1]["messages"].([]any) {
		mm, _ := m.(map[string]any)
		if mm["role"] == openrouter.RoleTool {
			answered = true
			if mm["tool_call_id"] != "call_1" {
				t.Errorf("the answer carries id %v, want call_1", mm["tool_call_id"])
			}
			if c, _ := mm["content"].(string); c == "" {
				t.Error("the answer carries no content, want what the call returned")
			}
		}
	}
	if !answered {
		t.Errorf("the second request carries no answer to the call: %v",
			bodies[1]["messages"])
	}
}

// A turn where the model called nothing must behave exactly as it did before
// this work: one request, one recorded exchange, and no tool turns. A client
// that sent empty tool turns would carry messages the provider did not ask for.
func TestATurnThatCalledNothingIsUnchanged(t *testing.T) {
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		asked++
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.send(s.ctx, s.conv, "a question")

	if asked != 1 {
		t.Errorf("the turn made %d requests, want 1", asked)
	}
	msgs := s.conv.Messages()
	if len(msgs) != 2 {
		t.Fatalf("the conversation holds %d turns, want 2: %+v", len(msgs), msgs)
	}
	for i, m := range msgs {
		if m.Role == openrouter.RoleTool || len(m.ToolCalls) > 0 {
			t.Errorf("turn %d is %+v, want no tool turn in a turn that called nothing", i, m)
		}
	}
}

// A call that failed still produces an answer. A request carrying no answer to
// a call is refused by most providers, and a turn producing nothing at all is a
// turn that stalls, which a reader experiences as a hang.
func TestAFailedCallStillAnswersTheModel(t *testing.T) {
	var mu sync.Mutex
	var second map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		first := len(body["messages"].([]any)) == 1
		mu.Lock()
		if !first && second == nil {
			second = body
		}
		mu.Unlock()
		if first {
			io.WriteString(w, toolCallStream("read_file",
				map[string]any{"path": "nothing-here"}))
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.send(s.ctx, s.conv, "read a file that is not there")

	mu.Lock()
	defer mu.Unlock()
	if second == nil {
		t.Fatal("the turn never asked again, so the failure was never reported back")
	}
	var answered bool
	for _, m := range second["messages"].([]any) {
		mm, _ := m.(map[string]any)
		if mm["role"] == openrouter.RoleTool {
			answered = true
			if c, _ := mm["content"].(string); !strings.Contains(c, "nothing-here") {
				t.Errorf("the answer names %q, want the failure to name the file", c)
			}
		}
	}
	if !answered {
		t.Error("the follow-up request carries no answer for the failed call")
	}
}

// A model that keeps calling tools is stopped at the limit, and the stop is
// said rather than being a turn that ends for no visible reason.
func TestAModelThatKeepsCallingToolsIsStopped(t *testing.T) {
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		asked++
		io.WriteString(w, toolCallStream("list_dir", map[string]any{}))
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.send(s.ctx, s.conv, "keep going")

	if asked != maxToolRounds {
		t.Errorf("the turn made %d requests, want the limit of %d", asked, maxToolRounds)
	}
	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "reached its limit") {
		t.Errorf("the pane does not say the turn was stopped at the limit:\n%s", joined)
	}
	if len(s.conv.Messages()) != 0 {
		t.Errorf("the conversation holds %+v, want nothing recorded from a stopped turn",
			s.conv.Messages())
	}
}

// A turn the reader stopped records nothing, which is the guarantee the session
// already made and the one a loop is most likely to break: the tool results
// are real, and writing them into the conversation would have the model carry
// work it was never shown the answers to.
func TestATurnStoppedPartWayThroughARecordsNothing(t *testing.T) {
	// The counter is read from the test goroutine while the handler writes it
	// from its own, so it is atomic rather than a plain int. A plain counter
	// here is a race that the detector reports and a fast machine hides.
	var asked atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		asked.Add(1)
		io.WriteString(w, toolCallStream("list_dir", map[string]any{}))
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.startTurn("keep going", s.conv)
	waitFor(t, func() bool { return asked.Load() >= 1 }, "the turn never reached the endpoint")
	s.stopTurn("never mind")
	// The stop settles the turn before the reader is answered, so the
	// recording decision has been made by the time it returns.
	waitFor(t, func() bool { return !s.working() }, "the turn did not settle")

	if len(s.conv.Messages()) != 0 {
		t.Errorf("the conversation holds %+v, want nothing recorded from a stopped turn",
			s.conv.Messages())
	}
}

// An ephemeral conversation is offered no tools. The gate is one condition, and
// this is the half of it that keeps /cognito and /btw honest: both mark the
// conversation as recording nothing, and a tool acting unrecorded would break
// the promise the mode makes.
func TestAnEphemeralConversationIsOfferedNoTools(t *testing.T) {
	asked := 0
	var offered bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		asked++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		_, offered = body["tools"]
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.conv.setEphemeral()
	s.send(s.ctx, s.conv, "read a file")

	if asked != 1 {
		t.Fatalf("the turn made %d requests, want 1", asked)
	}
	if offered {
		t.Error("a turn recording nothing was offered tools")
	}
}

// A thread is ephemeral as well, so it is covered by the same condition rather
// than by a check for the mode by name.
func TestAThreadIsOfferedNoTools(t *testing.T) {
	var offered bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		_, offered = body["tools"]
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.command("/btw")
	s.send(s.ctx, s.conv, "read a file")

	if offered {
		t.Error("a thread was offered tools, which would act for the reader unrecorded")
	}
}

// A call is reported by its size rather than by its content. A read of a large
// file would bury the conversation under the file, and the reader asked a
// question rather than for a file listing.
func TestACallIsReportedByItsSizeRatherThanItsContent(t *testing.T) {
	big := strings.Repeat("x", 200000)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big"), []byte(big), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if len(body["messages"].([]any)) == 1 {
			io.WriteString(w, toolCallStream("read_file", map[string]any{"path": "big"}))
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, dir)
	s.send(s.ctx, s.conv, "read the big file")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "[fs] read_file big") {
		t.Errorf("the pane does not report the call:\n%s", joined)
	}
	if strings.Contains(joined, big) {
		t.Error("the result was drawn into the pane, want it summarised by size")
	}
}

// A failed call is drawn as the reason on the same line, since a call that
// failed is not a call that is still running.
func TestAFailedCallIsReportedOnItsOwnLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if len(body["messages"].([]any)) == 1 {
			io.WriteString(w, toolCallStream("read_file", map[string]any{"path": "absent"}))
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.send(s.ctx, s.conv, "read a file that is absent")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "[fs] read_file absent ->") {
		t.Errorf("the pane does not report the failed call:\n%s", joined)
	}
	if !strings.Contains(joined, "absent") {
		t.Errorf("the pane does not name the file that failed:\n%s", joined)
	}
}

// /tools reports what the model is given and the root it is contained to, which
// is the only place a reader can find out what the client is willing to do.
func TestToolsReportsWhatTheModelIsGiven(t *testing.T) {
	dir := t.TempDir()
	s := toolSession(t, "http://127.0.0.1:1/api/v1", dir)
	s.command("/tools")

	joined := strings.Join(s.frame.Reply, "\n")
	for _, want := range []string{"read_file", "write_file", "list_dir", dir} {
		if !strings.Contains(joined, want) {
			t.Errorf("/tools does not mention %q:\n%s", want, joined)
		}
	}
}

// The update a reader composes replaces the question rather than being added
// as prose after it. After a round the request is the question and the answers
// to the calls, and restating those as text would throw away what the model has
// already been told.
func TestTheUpdateReplacesTheQuestion(t *testing.T) {
	msgs := []openrouter.Message{
		{Role: openrouter.RoleUser, Content: "the question"},
		{Role: openrouter.RoleAssistant, ToolCalls: []openrouter.ToolCall{{ID: "c1"}}},
		{Role: openrouter.RoleTool, ToolCallID: "c1", Content: "the answer"},
	}
	got := AmendLastUser(msgs, "the question\n\nthe update")
	if got[0].Content != "the question\n\nthe update" {
		t.Errorf("the question is %q, want the update appended to it", got[0].Content)
	}
	if got[2].Content != "the answer" {
		t.Errorf("the tool answer is %q, want it left alone", got[2].Content)
	}
	if len(got) != 3 {
		t.Errorf("the update added turns, want the question replaced in place: %+v", got)
	}
}

// toolCallStream is a stream in which the model calls one tool and says
// nothing.
//
// The chunk is marshalled rather than written out by hand. The arguments are a
// JSON document carried inside a JSON string, so the two have to be escaped
// against each other, and a hand-written fragment ends up testing the fragment
// rather than the transport. The reassembly of a call split across several
// deltas is proved in the transport package, where the framing belongs.
func toolCallStream(name string, args map[string]any) string {
	arguments, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	chunk := map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": 0,
					"id":    "call_1",
					"type":  "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(arguments),
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		panic(err)
	}
	return "data: " + string(encoded) + "\n\ndata: [DONE]\n\n"
}

// toolSession returns a session with tools, pointed at the given endpoint and
// contained to dir. The screen is a real one writing to a file, so nothing
// reaches the terminal, which is the idiom the rest of this package uses.
func toolSession(t *testing.T, baseURL, dir string) *Session {
	t.Helper()
	return newToolSession(t, baseURL, dir, tools.AlwaysAllow())
}

// newToolSession is toolSession with the approver the shell tool asks, since
// the tool being tested is one that asks. The default approves everything,
// which is right for the tests about the tool loop and wrong for the tests
// about what happens when a reader says no.
func newToolSession(t *testing.T, baseURL, dir string, approver tools.Approver) *Session {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}
	conv := NewConversation()
	conv.SetModel("test/model")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := &Session{
		conv:      conv,
		mainConv:  conv,
		screen:    &Screen{out: out, in: out, height: 24, width: 80},
		spinner:   NewSpinner(),
		windows:   newContextLength(),
		client:    openrouter.New(baseURL, "k"),
		approvals: newApprovalState(),
		tools:     toolsAt(dir, approver),
		ctx:       ctx,
		cancel:    cancel,
	}
	t.Cleanup(s.Close)
	return s
}
