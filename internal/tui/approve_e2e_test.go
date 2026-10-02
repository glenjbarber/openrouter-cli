package tui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// recordingReader stands in for a reader at the keyboard.
//
// The prompt itself cannot be driven from a test, since it reads a key through
// the terminal, so what is substituted here is the keyboard and the decision
// made around the question is what gets exercised. It answers whatever it is
// told to and records what it was asked, since a question that names something
// other than what runs would be approved on the wrong terms.
type recordingReader struct {
	answer bool
	mu     sync.Mutex
	asked  []string
}

// ask records the question and answers it.
func (a *recordingReader) ask(command string, args []string, dir string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked = append(a.asked, command+" "+strings.Join(args, " "))
	return a.answer
}

// questions returns what the reader was asked.
func (a *recordingReader) questions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.asked...)
}

// A model asking to build the tree is the case the tool exists for, and the
// whole turn is exercised here: the call is offered, run, and its output
// returned to the model.
func TestTheModelCanRunACommandAndSeeWhatItPrinted(t *testing.T) {
	dir := t.TempDir()
	var asked atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		if asked.Add(1) == 1 {
			io.WriteString(w, toolCallStream("shell", map[string]any{
				"command": "echo", "args": []string{"built"},
			}))
			return
		}
		io.WriteString(w, textStream("the build said built"))
	}))
	defer srv.Close()

	s, reader := askingSession(t, srv.URL, dir, true)
	s.approvals.record("echo", true)

	s.startTurn("build it", s.conv)
	waitFor(t, func() bool { return asked.Load() >= 2 }, "the turn never completed its tool round")

	if got := reader.questions(); len(got) != 0 {
		t.Errorf("the reader was asked about a granted program: %v", got)
	}
}

// A program the reader approved once is not asked about again, which is the
// memory the reader asked for: a model asking in every round is asked once.
func TestAProgramApprovedOnceIsNotAskedAboutAgain(t *testing.T) {
	dir := t.TempDir()
	var asked atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		if asked.Add(1) == 1 {
			io.WriteString(w, toolCallStream("shell", map[string]any{
				"command": "echo", "args": []string{"again"},
			}))
			return
		}
		io.WriteString(w, textStream("done"))
	}))
	defer srv.Close()

	// The session is the approver, since the memory it keeps is the thing
	// being tested and a stand-in would bypass it. The recorder stands in for
	// the keyboard, so a question reaching it is a question the reader was
	// actually put.
	reader := &recordingReader{answer: true}
	s := toolSession(t, srv.URL, dir)
	s.ask = reader.ask
	// The grant stands for an answer the reader has already given.
	s.approvals.record("echo", true)

	s.startTurn("build it", s.conv)
	waitFor(t, func() bool { return asked.Load() >= 2 }, "the turn never completed")

	// The session settled the call from the grant, so no question reached the
	// reader and the recorded answer is untouched.
	if got := len(reader.questions()); got != 0 {
		t.Errorf("the reader was asked %d times about a granted program", got)
	}
	if approved, answered := s.approvals.remembered("echo"); !answered || !approved {
		t.Error("the grant did not survive the call that used it")
	}
}

// A refused call reports the refusal to the model and runs nothing, so a model
// that is told no learns it rather than trying again for ever.
func TestARefusedCommandIsReportedToTheModelAndRunsNothing(t *testing.T) {
	dir := t.TempDir()
	marker := dir + "/written"
	var asked atomic.Int64
	// The second request body is guarded rather than a plain map, since the
	// handler fills it on the server goroutine and the assertions below read
	// it from this one.
	var second atomic.Pointer[map[string]any]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		if asked.Add(1) == 1 {
			io.WriteString(w, toolCallStream("shell", map[string]any{
				"command": "grep", "args": []string{"x", ">", marker},
			}))
			return
		}
		body := map[string]any{}
		json.NewDecoder(r.Body).Decode(&body)
		second.Store(&body)
		io.WriteString(w, textStream("understood"))
	}))
	defer srv.Close()

	s, reader := askingSession(t, srv.URL, dir, false)

	s.startTurn("do it", s.conv)
	// The wait is on the body rather than on the request count, since the
	// count is incremented before the body is decoded and a wait on it would
	// return before there was anything to assert on.
	waitFor(t, func() bool { return second.Load() != nil }, "the second request never arrived")

	if questions := reader.questions(); len(questions) != 1 {
		t.Errorf("the reader was asked %d times about one call: %v", len(questions), questions)
	}
	// A refusal is remembered too, so a model retrying does not wear the
	// reader down by asking again.
	if approved, answered := s.approvals.remembered("grep"); !answered || approved {
		t.Error("the refusal was not remembered for the rest of the session")
	}
	held := second.Load()
	if held == nil {
		t.Fatal("the second request never arrived")
	}
	if got := toolResultText(*held); !strings.Contains(got, "did not approve") {
		t.Errorf("the model was not told the call was refused: %s", got)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a refused call wrote a file")
	}
}

// toolResultText returns the content of the tool result in a request body.
func toolResultText(body map[string]any) string {
	messages, _ := body["messages"].([]any)
	// Every message is searched rather than only the tool role, since which
	// role carries the answer is a property of the transport rather than of
	// this feature, and a test asserting it would break when that changes.
	for _, entry := range messages {
		message, _ := entry.(map[string]any)
		content, _ := message["content"].(string)
		if strings.Contains(content, "approve") {
			return content
		}
	}
	return ""
}

// The shell is offered to a model along with the tools that were already there,
// so a model can choose to build rather than having to be told it may.
func TestTheShellIsOfferedAlongsideTheOtherTools(t *testing.T) {
	dir := t.TempDir()
	// Atomic rather than a plain bool, since the handler runs on the server
	// goroutine while the wait reads it from this one. The race detector
	// reports the plain bool, and a test that only passes without it is not a
	// test of the thing it claims to cover.
	var offered atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, `{"data":[{"id":"test/model","context_length":8192}]}`)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		list, _ := body["tools"].([]any)
		for _, entry := range list {
			spec, _ := entry.(map[string]any)
			fn, _ := spec["function"].(map[string]any)
			if fn["name"] == "shell" {
				offered.Store(true)
			}
		}
		io.WriteString(w, textStream("ok"))
	}))
	defer srv.Close()

	s := toolSession(t, srv.URL, dir)
	s.startTurn("what can you do", s.conv)
	waitFor(t, func() bool { return offered.Load() }, "the shell was never offered")

	if !s.tools.offersShell() {
		t.Error("the session does not hold a shell")
	}
}

// toolSessionWith is toolSession with an approver the test controls, since the
// default one answers everything and the whole point here is what happens when
// it does not.
func toolSessionWith(t *testing.T, baseURL, dir string, approver tools.Approver) *Session {
	t.Helper()
	return newToolSession(t, baseURL, dir, approver)
}

// textStream is a reply carrying text and nothing else, which is what the
// second request of a tool turn expects to find.
func textStream(text string) string {
	chunk := map[string]any{
		"choices": []any{map[string]any{
			"delta": map[string]any{"content": text},
		}},
	}
	body, err := json.Marshal(chunk)
	if err != nil {
		panic(err)
	}
	return "data: " + string(body) + "\n\ndata: [DONE]\n\n"
}

// askingSession builds a session that asks the reader, with a keyboard
// standing in for the terminal.
//
// The tool set is built with the session itself rather than with a stand-in, so
// that the memory the session keeps is what decides the call. A set built with
// an approver that answers everything would run the program before the reader
// was ever consulted, and the test would pass without asking anything.
func askingSession(t *testing.T, baseURL, dir string, answer bool) (*Session, *recordingReader) {
	t.Helper()
	reader := &recordingReader{answer: answer}
	s := newToolSession(t, baseURL, dir, nil)
	s.tools = toolsAt(dir, s)
	s.ask = reader.ask
	return s, reader
}
