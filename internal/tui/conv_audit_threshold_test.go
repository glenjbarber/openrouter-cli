package tui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// captureSession returns a session whose server records every request it
// receives, so that what was actually sent can be inspected rather than
// inferred from the state of the conversation afterwards.
func captureSession(t *testing.T, models, body string,
	onRequest func(path, payload string)) *Session {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("reading the request body: %v", err)
			}
			onRequest(r.URL.Path, string(payload))
			if r.URL.Path == "/models" {
				io.WriteString(w, models)
				return
			}
			io.WriteString(w, body)
		}))
	t.Cleanup(srv.Close)

	out, err := os.CreateTemp(t.TempDir(), "frames")
	if err != nil {
		t.Fatalf("creating the capture file: %v", err)
	}

	conv := NewConversation()
	conv.SetModel("test/model")
	return &Session{
		conv:     conv,
		mainConv: conv,
		screen:   &Screen{out: out, height: 24, width: 80},
		spinner:  NewSpinner(),
		windows:  newContextLength(),
		client:   openrouter.New(srv.URL, "k"),
		ctx:      context.Background(),
		cancel:   func() {},
	}
}

// A compaction asked for runs whatever the size of the conversation, since
// asking for it is the point.
func TestManualCompactionRunsOnAShortConversation(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	s := captureSession(t,
		`{"data":[{"id":"test/model","context_length":8192}]}`,
		auditStream, func(path, payload string) {
			mu.Lock()
			requests = append(requests, path)
			mu.Unlock()
		})
	s.conv.Record("q1", "a1")

	s.compact(true)

	if !s.conv.HasSummary() {
		t.Error("a compaction asked for on one exchange did not run")
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range requests {
		if path == "/chat/completions" {
			return
		}
	}
	t.Errorf("requests = %v, want the summary to have been asked for", requests)
}

// A conversation too short to gain from compacting is left alone, however large
// its estimate is. Summarising it would cost a request to produce something no
// shorter than itself.
func TestAutomaticCompactionLeavesAShortConversationAlone(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	s := blockingSession(t, auditStream, reached, release)

	// One exchange, large enough to be past the threshold of the window the
	// model list reports.
	s.conv.Record("q1", strings.Repeat("x", 40_000))
	if got, want := s.conv.EstimatedTokens(),
		int(float64(8192)*compactionThreshold); got <= want {
		t.Fatalf("the estimate is %d against a window of 8192, want it past %d",
			got, want)
	}

	s.maybeCompact()

	select {
	case <-reached:
		t.Fatal("a compaction was asked for on a conversation too short to gain")
	case <-time.After(250 * time.Millisecond):
	}
	if s.conv.HasSummary() {
		t.Error("the conversation was compacted anyway")
	}
}

// The compaction runs before the request that would exceed the window, since a
// request past the window is refused and the turn is lost with it. The request
// that goes out afterwards carries the summary rather than the turns it
// replaced.
func TestCompactionRunsBeforeTheRequestThatWouldExceedTheWindow(t *testing.T) {
	var mu sync.Mutex
	var completions []string
	s := captureSession(t,
		`{"data":[{"id":"test/model","context_length":8192}]}`,
		auditStream, func(path, payload string) {
			if path == "/chat/completions" {
				mu.Lock()
				completions = append(completions, payload)
				mu.Unlock()
			}
		})
	s.conv.Record("the first question", strings.Repeat("x", 30_000))
	s.conv.Record("the second question", "the second answer")

	s.send("the question the reader asked")

	mu.Lock()
	defer mu.Unlock()
	if len(completions) != 2 {
		t.Fatalf("completions sent = %d, want the summary and then the question:\n%v",
			len(completions), completions)
	}
	if !strings.Contains(completions[0], "Summarise the following conversation") {
		t.Errorf("the first request was not the summary: %s", completions[0])
	}
	if !strings.Contains(completions[1], "the question the reader asked") {
		t.Errorf("the second request does not carry the question: %s", completions[1])
	}
	for _, gone := range []string{"the first question", "the second question"} {
		if strings.Contains(completions[1], gone) {
			t.Errorf("the request after the compaction still carries %q", gone)
		}
	}
	if !strings.Contains(completions[1], "Hello") {
		t.Errorf("the request after the compaction does not carry the summary: %s",
			completions[1])
	}
}

// The instructions and the most recent exchange are held out of the summary
// request. The instructions are kept by the compaction, so a summary of them
// would compete with the original, and the most recent exchange is the context
// the model answering next is expected to have.
func TestTheSummaryRequestHoldsOutTheInstructionsAndTheRecentExchange(t *testing.T) {
	var mu sync.Mutex
	var completions []string
	s := captureSession(t,
		`{"data":[{"id":"test/model","context_length":8192}]}`,
		auditStream, func(path, payload string) {
			if path == "/chat/completions" {
				mu.Lock()
				completions = append(completions, payload)
				mu.Unlock()
			}
		})
	s.conv.Seed("the opening instructions")
	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")
	s.conv.Record("q3", "a3")
	s.conv.Record("q4", "a4")

	s.compact(true)

	mu.Lock()
	defer mu.Unlock()
	if len(completions) == 0 {
		t.Fatal("no summary was asked for")
	}
	summary := completions[0]
	// The most recent exchange is the pair that is held out, which is the last
	// question and the answer it produced.
	for _, held := range []string{"the opening instructions", "q4", "a4"} {
		if strings.Contains(summary, held) {
			t.Errorf("the summary request carries %q, want it held out", held)
		}
	}
	for _, wanted := range []string{"q1", "a1", "q2", "a2", "q3", "a3"} {
		if !strings.Contains(summary, wanted) {
			t.Errorf("the summary request does not carry %q, want the older turns in it", wanted)
		}
	}
}

// The size of a conversation is estimated at four characters to a token, which
// is a figure rather than a count since counting exactly needs the tokenizer.
func TestEstimatedTokensCountsFourCharactersToAToken(t *testing.T) {
	c := NewConversation()
	c.Seed("be terse")
	c.Record("hello", "world")

	// A system turn of "be terse", a user turn of "hello" and an assistant
	// turn of "world", four characters counted for each of the three turns on
	// top of the role and the text.
	const chars = 6 + 8 + 4 + 4 + 5 + 4 + 9 + 5 + 4
	const want = chars / 4
	if got := c.EstimatedTokens(); got != want {
		t.Errorf("EstimatedTokens = %d, want %d from %d characters at four to a token",
			got, want, chars)
	}
}

// A model the list does not carry falls back to a common window rather than to
// nothing, which would disable the threshold and with it the compaction. The
// list is fetched once and remembered.
func TestLookupFallsBackForAModelTheListDoesNotCarry(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := captureSession(t,
		`{"data":[{"id":"other/model","context_length":64000}]}`,
		"", func(path, payload string) {
			if path == "/models" {
				mu.Lock()
				calls++
				mu.Unlock()
			}
		})

	c := newContextLength()
	if got := c.lookup(s.ctx, s, "test/model"); got != fallbackWindow {
		t.Errorf("lookup = %d, want the fallback %d for a model the list omits",
			got, fallbackWindow)
	}
	if got := c.lookup(s.ctx, s, "other/model"); got != 64_000 {
		t.Errorf("lookup = %d, want the window the list reports", got)
	}
	if got := c.lookup(s.ctx, s, "other/model"); got != 64_000 {
		t.Errorf("lookup = %d on the second call, want the cached window", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("the model list was fetched %d times, want once", calls)
	}
}
