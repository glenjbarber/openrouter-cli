package tui

import (
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
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// TestADelegateIsStillSentNoTools checks the rule /spawn was added beside has
// not moved: a delegate records nothing, so a tool must not be offered.
//
// A model offered a tool it cannot have streams the markup as text instead,
// which a reader sees as a tool call written out in the reply rather than a
// reply at all. That is the failure the notice and the absent catalogue are
// both there to prevent.
func TestADelegateIsStillSentNoTools(t *testing.T) {
	var offered atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			offered.Store(true)
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, t.TempDir())
	s.startDelegate("what is in here")
	waitFor(t, func() bool { return s.delegatePending() == 0 && dpaneHasLines(s) },
		"the delegate never settled")

	if offered.Load() {
		t.Error("a delegate was offered tools, which would act for the reader unrecorded")
	}
}

// dpaneHasLines reports whether the delegate pane has taken anything, since a
// delegate runs on a goroutine and the count drops to zero before the lines are
// written.
func dpaneHasLines(s *Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dpane.lines) > 0
}

// TestASpawnIsGivenTheTools checks the other half: a worker is offered the
// catalogue, since a worker that cannot call a tool is a delegate under a new
// name and the distinction would be worth nothing.
func TestASpawnIsGivenTheTools(t *testing.T) {
	var offered atomic.Bool
	var mu sync.Mutex
	// requests counts every request the worker made, and answered holds the
	// bodies of those that carry a tool message. The two are kept apart since
	// the request that carries the answer to a call is the second one, so a
	// slice filled only while there is no answer can never hold it.
	var requests int
	var answered []map[string]any
	var seenAnswer bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tools"]; ok {
			offered.Store(true)
		}
		msgs, _ := body["messages"].([]any)
		carries := false
		for _, m := range msgs {
			mm, _ := m.(map[string]any)
			if mm["role"] == openrouter.RoleTool {
				carries = true
			}
		}
		mu.Lock()
		requests++
		if carries {
			answered = append(answered, body)
			seenAnswer = true
		}
		// The first request is the one made before any tool message has been
		// seen. Every later one is given the final stream, since a worker that
		// is handed the call again would go on calling until it ran out of
		// rounds and the test would be proving the limit instead.
		first := !seenAnswer
		mu.Unlock()
		if first {
			io.WriteString(w, toolCallStream("list_dir", map[string]any{"path": "."}))
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}

	s := toolSession(t, srv.URL, dir)
	s.startSpawn("what is in here", "")
	waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
		"the worker never settled")

	if !offered.Load() {
		t.Error("a worker was offered no tools, want the catalogue the session holds")
	}

	mu.Lock()
	defer mu.Unlock()
	if requests >= maxToolRounds {
		t.Errorf("the worker made %d requests, want it to settle in fewer than the limit of %d",
			requests, maxToolRounds)
	}
	if len(answered) == 0 {
		t.Fatalf("the worker made %d requests and none carried an answer to the call", requests)
	}
	var found bool
	for _, m := range answered[0]["messages"].([]any) {
		mm, _ := m.(map[string]any)
		if mm["role"] == openrouter.RoleTool {
			found = true
			if mm["tool_call_id"] != "call_1" {
				t.Errorf("the answer carries id %v, want call_1", mm["tool_call_id"])
			}
			if c, _ := mm["content"].(string); c == "" {
				t.Error("the answer carries no content, want what the call returned")
			}
		}
	}
	if !found {
		t.Errorf("the follow-up request carries no answer to the call: %v",
			answered[0]["messages"])
	}
}

// wpaneHasLines reports whether the worker pane has taken anything, since a
// worker runs on a goroutine and the count drops to zero before the lines are
// written.
func wpaneHasLines(s *Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.wpane.lines) > 0
}

// TestASpawnIsRefusedWhileCognito checks the one gate on the command.
//
// A worker acts on the host, and cognito promises nothing is recorded, so a
// worker started under it would be the one path where a tool does something
// with no record of it having been offered.
func TestASpawnIsRefusedWhileCognito(t *testing.T) {
	s := toolSession(t, "http://127.0.0.1:1/api/v1", t.TempDir())
	s.cognito = true
	s.command("/spawn do a thing")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "/spawn is refused") {
		t.Errorf("/spawn was not refused under cognito:\n%s", joined)
	}
	if s.workerPending() != 0 {
		t.Error("a worker was started under cognito")
	}
}

// TestASpawnProviderIsRefusedWhenUnconfigured checks that --provider NAME is
// refused by name when nothing is configured under that name, rather than
// falling back to the session's OpenRouter client and model silently.
func TestASpawnProviderIsRefusedWhenUnconfigured(t *testing.T) {
	s := toolSession(t, "http://127.0.0.1:1/api/v1", t.TempDir())
	s.command("/spawn --provider groq do a thing")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "groq") || !strings.Contains(joined, "not configured") {
		t.Errorf("/spawn --provider groq was not refused by name:\n%s", joined)
	}
	if s.workerPending() != 0 {
		t.Error("a worker was started with no provider configured")
	}
}

// TestASpawnProviderNeedsAName checks that --provider with nothing after it
// is refused rather than treated as the question.
func TestASpawnProviderNeedsAName(t *testing.T) {
	s := toolSession(t, "http://127.0.0.1:1/api/v1", t.TempDir())
	s.command("/spawn --provider")

	joined := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(joined, "--provider needs a name") {
		t.Errorf("/spawn --provider with no name was not refused:\n%s", joined)
	}
	if s.workerPending() != 0 {
		t.Error("a worker was started with --provider and no name")
	}
}

// TestASpawnProviderUsesTheNamedBackend checks that --provider NAME actually
// reaches that provider's client and model rather than the session's own, by
// giving the two servers distinguishable replies and asking which one
// answered.
func TestASpawnProviderUsesTheNamedBackend(t *testing.T) {
	mainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		t.Error("the main backend was called by a --provider worker")
	}))
	defer mainSrv.Close()

	var gotModel string
	provSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"groq/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		io.WriteString(w, auditStream)
	}))
	defer provSrv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	s := toolSession(t, mainSrv.URL, t.TempDir())
	s.providers = map[string]providerBinding{
		"groq": {client: openrouter.New(provSrv.URL, "k"), model: "groq/model"},
	}
	s.command("/spawn --provider groq what is in here")

	waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
		"the provider worker never settled")

	if gotModel != "groq/model" {
		t.Errorf("the provider backend was asked for model %q, want %q", gotModel, "groq/model")
	}
}

// TestASpawnNamesTheLogItLeaves checks the record, since a worker that acts on
// the host and keeps nothing is the one thing /cognito refuses. The pane is
// what a reader watches while it happens; the file is what they read after.
func TestASpawnNamesTheLogItLeaves(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	s := toolSession(t, srv.URL, t.TempDir())
	s.startSpawn("what is in here", "")
	waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
		"the worker never settled")

	path, err := workerDir()
	if err != nil {
		t.Fatalf("resolving the worker directory: %v", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(entries) != 1 {
		t.Fatalf("%s holds %d files, want the one the worker left", path, len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".md") {
		t.Errorf("the log is named %q, want a name ending .md", entries[0].Name())
	}

	body, err := os.ReadFile(filepath.Join(path, entries[0].Name()))
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "what is in here") {
		t.Errorf("the log does not name the question:\n%s", text)
	}
	if !strings.Contains(text, "answer") {
		t.Errorf("the log does not carry the answer:\n%s", text)
	}

	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("stat the log: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the log has mode %04o, want 0600", mode)
	}
}

// TestASpawnStopsAtTheRoundLimit checks the cap, since a worker with tools can
// ask for the same file for ever and the cap is what stops a loop that does
// not converge. It is the same figure a turn uses, for the same reason.
func TestASpawnStopsAtTheRoundLimit(t *testing.T) {
	var asked atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		asked.Add(1)
		io.WriteString(w, toolCallStream("list_dir", map[string]any{"path": "."}))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	s := toolSession(t, srv.URL, t.TempDir())
	s.startSpawn("keep going", "")
	waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
		"the worker never settled")

	if got := int(asked.Load()); got != maxToolRounds {
		t.Errorf("the worker made %d requests, want the limit of %d", got, maxToolRounds)
	}
	s.mu.Lock()
	joined := strings.Join(s.wpane.lines, "\n")
	s.mu.Unlock()
	if !strings.Contains(joined, "reached its limit") {
		t.Errorf("the pane does not say the worker stopped at the limit:\n%s", joined)
	}
}

// TestASpawnRecordsNothingInTheConversation checks that a worker does not leave
// its exchange behind. The record under API settles that a turn is committed
// once and only where it ended cleanly, and a worker that recorded into the
// conversation would have the model carry tool results it was never shown.
func TestASpawnRecordsNothingInTheConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if carriesToolResult(body) {
			io.WriteString(w, auditStream)
			return
		}
		io.WriteString(w, toolCallStream("list_dir", map[string]any{"path": "."}))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	s := toolSession(t, srv.URL, t.TempDir())
	s.startSpawn("read the directory", "")
	waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
		"the worker never settled")

	if msgs := s.conv.Messages(); len(msgs) != 0 {
		t.Errorf("the conversation holds %+v, want nothing from a worker", msgs)
	}
}

// TestAWorkerPaneIsItsOwnPane checks that /pane spawn reaches the worker output
// and that the next and previous keys step through all three panes, since a
// reader moving between them with a key should not wrap past the worker.
func TestAWorkerPaneIsItsOwnPane(t *testing.T) {
	s := toolSession(t, "http://127.0.0.1:1/api/v1", t.TempDir())

	s.command("/pane spawn")
	if got := s.panes.Current(); got != workerPaneIndex {
		t.Errorf("/pane spawn shows pane %d, want %d", got, workerPaneIndex)
	}
	if got := s.panes.Name(workerPaneIndex); got != workerPaneName {
		t.Errorf("the worker pane is named %q, want %q", got, workerPaneName)
	}

	s.command("/pane delegate")
	if got := s.panes.Current(); got != delegatePaneIndex {
		t.Errorf("/pane delegate shows pane %d, want %d", got, delegatePaneIndex)
	}
	s.command("/pane main")
	if !s.panes.OnMain() {
		t.Errorf("/pane main shows pane %d, want the conversation", s.panes.Current())
	}

	// Three panes, so a step forward from the last wraps to the first.
	s.stepPane(1)
	if got := s.panes.Current(); got != delegatePaneIndex {
		t.Errorf("one step forward shows pane %d, want %d", got, delegatePaneIndex)
	}
	s.stepPane(1)
	if got := s.panes.Current(); got != workerPaneIndex {
		t.Errorf("two steps forward shows pane %d, want %d", got, workerPaneIndex)
	}
	s.stepPane(1)
	if !s.panes.OnMain() {
		t.Errorf("three steps forward shows pane %d, want the conversation", s.panes.Current())
	}
}

// TestTheWorkerPaneShowsACallInItsOwnWords checks that a call the worker makes
// is reported the way a turn reports one: by name, with the argument that says
// which file, and by the size of the result rather than the result.
func TestTheWorkerPaneShowsACallInItsOwnWords(t *testing.T) {
	s := toolSession(t, "http://127.0.0.1:1/api/v1", t.TempDir())
	s.drawWorkerCall(tools.Result{
		Call: openrouter.ToolCall{
			ID:       "call_1",
			Function: openrouter.ToolCallFunction{Name: "list_dir", Arguments: `{"path":"sub"}`},
		},
		Text: strings.Repeat("x", 2000),
	})

	s.mu.Lock()
	joined := strings.Join(s.wpane.lines, "\n")
	s.mu.Unlock()
	if !strings.Contains(joined, "[fs] list_dir sub -> 2.0 kB") {
		t.Errorf("the worker pane does not report the call:\n%s", joined)
	}
	if strings.Contains(joined, strings.Repeat("x", 40)) {
		t.Error("the result was drawn into the pane, want it summarised by size")
	}
}

// TestCloseWaitsForAWorker checks the teardown, since a worker holds its own
// request and leaving while one is running would leave a goroutine writing to
// a terminal that has been handed back.
func TestCloseWaitsForAWorker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		io.WriteString(w, auditStream)
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)

	s := toolSession(t, srv.URL, t.TempDir())
	s.startSpawn("what is in here", "")
	waitFor(t, func() bool { return s.workerPending() == 1 }, "the worker never started")

	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return, so a worker was not waited for")
	}
	if s.workerPending() != 0 {
		t.Error("a worker was still counted after Close returned")
	}
}
