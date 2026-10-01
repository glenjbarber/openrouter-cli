package openrouter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A response body that is not closed holds its connection until the pool gives
// up on it, so every path out of a request is checked for the close rather than
// the one path a reply happens to exercise.

// countingBody records whether it was closed and how much of it was read, which
// is what a leaked body on a streaming response looks like from here.
type countingBody struct {
	io.Reader
	mu      sync.Mutex
	reads   int
	closed  int
	closedC chan struct{}
}

func newCountingBody(text string) *countingBody {
	return &countingBody{Reader: strings.NewReader(text), closedC: make(chan struct{})}
}

func (b *countingBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	b.reads++
	b.mu.Unlock()
	return b.Reader.Read(p)
}

func (b *countingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed++
	select {
	case <-b.closedC:
	default:
		close(b.closedC)
	}
	return nil
}

func (b *countingBody) isClosed() bool {
	select {
	case <-b.closedC:
		return true
	default:
		return false
	}
}

func (b *countingBody) readCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

// transportFor points the client at a transport that answers with the given
// status and body, so a test can observe what the package does with a response
// rather than with a server.
func transportFor(c *Client, code int, body *countingBody) *countingBody {
	c.http.Transport = quotingTransport{code: code, body: body}
	return body
}

// Where a failure reaches the caller. A failure part way through a stream is
// delivered to the callback, since the text that arrived before it is still
// worth keeping, while a failure before the stream begins is returned.
const (
	failNowhere   = ""
	failReturned  = "returned"
	failDelivered = "delivered"
)

// A stream carrying one delta, without the terminating marker.
const oneDelta = `data: {"choices":[{"delta":{"content":"x"}}]}`

// Every path out of a request closes the body.
func TestBodyIsClosedOnEveryPath(t *testing.T) {
	tests := []struct {
		name string
		call string
		code int
		body string
		fail string
	}{
		{name: "a stream that completes", call: "chat", code: http.StatusOK, body: "data: [DONE]\n\n"},
		{name: "a stream that is cut short", call: "chat", code: http.StatusOK, body: oneDelta + "\n\n", fail: failDelivered},
		{name: "a stream that fails to decode", call: "chat", code: http.StatusOK, body: "data: {not json\n\n", fail: failDelivered},
		{name: "an empty body on a stream", call: "chat", code: http.StatusOK, body: "", fail: failDelivered},
		{name: "a stream that carries an error", call: "chat", code: http.StatusOK, body: `data: {"error":{"message":"no"}}` + "\n\n", fail: failDelivered},
		{name: "an error status on a stream", call: "chat", code: http.StatusUnauthorized, body: `{"error":{"message":"no"}}`, fail: failReturned},
		{name: "a listing", call: "models", code: http.StatusOK, body: `{"data":[]}`},
		{name: "a listing that is not a catalogue", call: "models", code: http.StatusOK, body: "<html>maintenance</html>", fail: failReturned},
		{name: "an error status on a listing", call: "models", code: http.StatusInternalServerError, body: "no", fail: failReturned},
		{name: "a key usage", call: "key", code: http.StatusOK, body: `{"data":{}}`},
		{name: "an error status on a key usage", call: "key", code: http.StatusTooManyRequests, body: "no", fail: failReturned},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := New("https://example.invalid", "k")
			body := transportFor(c, tc.code, newCountingBody(tc.body))

			var returned, delivered error
			var done bool
			onEvent := func(e StreamEvent) {
				if e.Err != nil {
					delivered = e.Err
				}
				if e.Done {
					done = true
				}
			}

			switch tc.call {
			case "chat":
				returned = c.Chat(context.Background(), ChatRequest{
					Model:    "test/model",
					Messages: []Message{{Role: RoleUser, Content: "hi"}},
				}, onEvent)
			case "models":
				_, returned = c.Models(context.Background())
			case "key":
				_, returned = c.KeyUsage(context.Background())
			default:
				t.Fatalf("the test names no call: %q", tc.call)
			}

			switch tc.fail {
			case failReturned:
				if returned == nil {
					t.Error("the call succeeded, want the failure returned")
				}
				if delivered != nil {
					t.Errorf("the callback carried %v, want the failure returned", delivered)
				}
			case failDelivered:
				if delivered == nil {
					t.Errorf("the callback carried no failure, want it delivered: returned %v", returned)
				}
				if returned != nil {
					t.Errorf("the call returned %v, want the failure delivered to the callback", returned)
				}
				if done {
					t.Error("a stream that failed was reported as complete")
				}
			default:
				if returned != nil || delivered != nil {
					t.Errorf("the call failed: returned %v, delivered %v, want it to succeed",
						returned, delivered)
				}
			}
			if !body.isClosed() {
				t.Error("the response body was not closed")
			}
			if body.readCount() == 0 {
				t.Error("the response body was closed without being read")
			}
		})
	}
}

// A transport that fails hands back no body, so there is nothing to close, and
// neither call must reach a nil dereference on the way out.
func TestNoBodyIsClosedWhenTheTransportFails(t *testing.T) {
	cause := errors.New("no route to host")

	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "a listing",
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "a stream",
			call: func(c *Client) error {
				return c.Chat(context.Background(), ChatRequest{
					Model:    "test/model",
					Messages: []Message{{Role: RoleUser, Content: "hi"}},
				}, func(StreamEvent) {})
			},
		},
		{
			name: "a key usage",
			call: func(c *Client) error {
				_, err := c.KeyUsage(context.Background())
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New("https://example.invalid", "k")
			c.http.Transport = quotingTransport{err: cause}

			err := tc.call(c)
			if err == nil {
				t.Fatal("the call accepted a transport failure, want an error")
			}
			if !errors.Is(err, cause) {
				t.Errorf("errors.Is(%v, the transport error) = false, want true", err)
			}
			if !strings.Contains(err.Error(), "https://example.invalid") {
				t.Errorf("err = %q, want the endpoint named", err)
			}
		})
	}
}

// The body of a successful stream is read once, by the parser, and is not read
// first by the status check. The delta arriving proves it, since a body consumed
// by the status check would have left the parser with nothing.
func TestStreamIsReadByTheParserAlone(t *testing.T) {
	c := New("https://example.invalid", "k")
	body := transportFor(c, http.StatusOK, newCountingBody(oneDelta+"\n\ndata: [DONE]\n\n"))

	var got strings.Builder
	if err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) { got.WriteString(e.Content) }); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.String() != "x" {
		t.Errorf("content = %q, want the stream read once by the parser", got.String())
	}
	if !body.isClosed() {
		t.Error("the response body was not closed")
	}
	if body.readCount() == 0 {
		t.Error("the response body was never read")
	}
}

// A body that is not a stream is bounded, so that a server answering with an
// endless one cannot take the session down with it. The bound is far above the
// largest legitimate catalogue.
func TestResponseBodyIsBounded(t *testing.T) {
	c := New("https://example.invalid", "k")
	transportFor(c, http.StatusOK, newCountingBody(strings.Repeat("x", maxResponse+1)))

	_, err := c.Models(context.Background())
	if err == nil {
		t.Fatal("Models read an endless body, want it refused")
	}
	if !strings.Contains(err.Error(), "exceeded") {
		t.Errorf("err = %q, want the bound reported", err)
	}
}

// A body too large to read does not hide the status behind it. The status is
// the part a reader acts on, so a rejected key is still named as a rejected key
// when the body that came with it was too large to be read. A status that is
// not an error leaves the bound as the whole of the diagnostic, since there is
// no status to report and reporting one would be a lie.
func TestStatusIsReportedWhenTheBodyCannotBeRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		want     error
		wantHTTP bool
		wantSize bool
	}{
		{name: "a rejected key", code: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "an exhausted allowance", code: http.StatusTooManyRequests, want: ErrRateLimited},
		{name: "a status with no named error", code: http.StatusBadGateway, wantHTTP: true},
		{name: "a success", code: http.StatusOK, wantSize: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New("https://example.invalid", "k")
			body := transportFor(c, tc.code, newCountingBody(strings.Repeat("x", maxResponse+1)))

			_, err := c.Models(context.Background())
			if err == nil {
				t.Fatal("Models read an endless body, want it refused")
			}
			if !strings.Contains(err.Error(), "reading the response") {
				t.Errorf("err = %q, want the body that could not be read named", err)
			}
			switch {
			case tc.want != nil:
				if !errors.Is(err, tc.want) {
					t.Errorf("errors.Is(%v, %v) = false, want the status named", err, tc.want)
				}
			case errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrRateLimited):
				t.Errorf("err = %v, want no named error for a status of %d", err, tc.code)
			}
			if got := strings.Contains(err.Error(), "HTTP"); got != tc.wantHTTP {
				t.Errorf("err = %q, want the status named = %v", err, tc.wantHTTP)
			}
			// The size is stated only where the status does not account for
			// the failure on its own, which is the case that has no status.
			if got := strings.Contains(err.Error(), "exceeded"); got != tc.wantSize {
				t.Errorf("err = %q, want the bound stated = %v", err, tc.wantSize)
			}
			if !body.isClosed() {
				t.Error("the response body was not closed")
			}
		})
	}
}

// A catalogue within the bound is read whole, so the bound costs a listing
// nothing.
func TestResponseBodyWithinTheBoundIsReadWhole(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[`)
		for i := 0; i < 2000; i++ {
			if i > 0 {
				io.WriteString(w, ",")
			}
			fmt.Fprintf(w, `{"id":"vendor/model-%d","name":"Model %d","description":%q,"prompt":"0","completion":"0"}`,
				i, i, strings.Repeat("d", 200))
		}
		io.WriteString(w, `]}`)
	})

	models, err := c.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 2000 {
		t.Errorf("len = %d, want the whole catalogue", len(models))
	}
	if !models[1999].Free() {
		t.Errorf("model = %+v, want the catalogue decoded and not merely counted", models[1999])
	}
}

// The error body of a stream is bounded as well, since it is read into memory
// before the message is taken from it.
func TestStreamErrorBodyIsBounded(t *testing.T) {
	c := New("https://example.invalid", "k")
	body := transportFor(c, http.StatusBadGateway, newCountingBody(strings.Repeat("y", maxDetail+1)))

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil {
		t.Fatal("Chat accepted an endless error body, want an error")
	}
	if !strings.Contains(err.Error(), "no detail was given") {
		t.Errorf("err = %q, want the bound to leave nothing to quote", err)
	}
	if len(err.Error()) > 1024 {
		t.Errorf("err is %d bytes, want the detail bounded", len(err.Error()))
	}
	if !body.isClosed() {
		t.Error("the response body was not closed")
	}
}

// A request with no credential is caught before it is sent, on every path, since
// the backend answers an absent key and a rejected key the same way.
func TestNoCredentialIsCaughtBeforeTheRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a request was sent with no credential")
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "")
	if c.HasKey() {
		t.Fatal("HasKey = true for a client built with no credential")
	}
	if err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {}); !errors.Is(err, ErrNoKey) {
		t.Errorf("Chat = %v, want ErrNoKey", err)
	}
	if _, err := c.Models(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Errorf("Models = %v, want ErrNoKey", err)
	}
	if _, err := c.KeyUsage(context.Background()); !errors.Is(err, ErrNoKey) {
		t.Errorf("KeyUsage = %v, want ErrNoKey", err)
	}
}

// A client that has a credential reports having one, without exposing it.
func TestHasKeyDoesNotExposeTheCredential(t *testing.T) {
	c := New("https://example.invalid", "test-key")
	if !c.HasKey() {
		t.Error("HasKey = false for a client with a credential")
	}
	if c.BaseURL() != "https://example.invalid" {
		t.Errorf("BaseURL = %q, want it unchanged by the credential", c.BaseURL())
	}
}

// A request is cancelled when the caller cancels it, rather than running to the
// deadline with nobody waiting for it.
func TestRequestIsCancelledWithTheCaller(t *testing.T) {
	release := make(chan struct{})
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	defer close(release)

	done := make(chan error, 1)
	go func() {
		_, err := c.Models(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Models = %v, want the cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Models did not return after the caller cancelled")
	}
}

// A stream is cancelled with the caller too, and a cancellation part way through
// is reported rather than passing for a complete reply. The text that arrived
// before it is kept, since that is what a caller watching a reply draws.
func TestStreamIsCancelledWithTheCaller(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, oneDelta+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	type outcome struct {
		err      error
		streamed error
		done     bool
		content  string
	}
	done := make(chan outcome, 1)
	go func() {
		var got strings.Builder
		var o outcome
		o.err = c.Chat(ctx, ChatRequest{
			Model:    "test/model",
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		}, func(e StreamEvent) {
			if e.Err != nil {
				o.streamed = e.Err
			}
			if e.Done {
				o.done = true
			}
			got.WriteString(e.Content)
		})
		o.content = got.String()
		done <- o
	}()

	select {
	case o := <-done:
		if o.content != "x" {
			t.Errorf("content = %q, want the text before the cancellation kept", o.content)
		}
		if o.done {
			t.Error("a cancelled stream was reported as complete")
		}
		if o.streamed == nil && o.err == nil {
			t.Error("a cancelled stream was reported as neither a failure nor a return")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Chat did not return after the caller cancelled")
	}
}

// A client with no deadline of its own could hang for ever on a connection that
// never answers, so the deadline is asserted rather than assumed. Which
// deadline is right is a decision for the maintainer; that there is one is not.
func TestClientCarriesADeadline(t *testing.T) {
	c := New("https://example.invalid", "k")
	if c.http.Timeout <= 0 {
		t.Error("the client has no deadline, want one bounding a stalled connection")
	}
	if c.http.Timeout > time.Hour {
		t.Errorf("the deadline is %s, want it long enough for a slow first token", c.http.Timeout)
	}
}

// The base URL is used as given apart from a trailing slash, since an override
// carrying a full path must not have anything appended to it.
func TestBaseURLIsUsedVerbatim(t *testing.T) {
	for given, want := range map[string]string{
		"https://example.invalid":         "https://example.invalid",
		"https://example.invalid/":        "https://example.invalid",
		"https://example.invalid/api/v1":  "https://example.invalid/api/v1",
		"https://example.invalid/api/v1/": "https://example.invalid/api/v1",
		"http://localhost:3000":           "http://localhost:3000",
		"http://localhost:3000/api/v1///": "http://localhost:3000/api/v1",
	} {
		if got := New(given, "k").BaseURL(); got != want {
			t.Errorf("BaseURL(%q) = %q, want %q", given, got, want)
		}
	}
}
