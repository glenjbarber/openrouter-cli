package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The credential travels in a header and must not reach a diagnostic. It can
// come back from the far side: a server that quotes the request in the body it
// returns, or a proxy that quotes it in the message it reports. Every string
// this package builds is shown to a reader and can end in a terminal history,
// so each one is filtered rather than trusted to be free of it.

const secretKey = "sk-or-v1-AUDIT-CANARY-0123456789"

// auditServer returns a test server and a client for it whose credential is
// the canary, so a diagnostic that reaches the test is caught by comparing it
// against the one value that must never appear.
func auditServer(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, secretKey)
}

// mustNotCarry fails when a diagnostic carries the credential.
func mustNotCarry(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, secretKey) {
		t.Errorf("%s carries the credential: %q", what, s)
	}
}

// A server that answers an error by quoting the request, which is what a
// gateway in front of the endpoint does when it rejects a request.
func TestCredentialIsNotCarriedByStatusError(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"rejected Authorization: Bearer `+
			secretKey+` for /chat/completions"}}`)
	})

	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(StreamEvent) {})
	if err == nil {
		t.Fatal("Chat accepted a 401, want an error")
	}
	mustNotCarry(t, "the 401 diagnostic", err.Error())
	// The redaction must not swallow the backend detail, or the diagnostic
	// would say nothing at all.
	if !strings.Contains(err.Error(), redacted) {
		t.Errorf("err = %q, want the credential marked as removed", err)
	}
	if !strings.Contains(err.Error(), "/chat/completions") {
		t.Errorf("err = %q, want the rest of the detail kept", err)
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(%v, ErrUnauthorized) = false, want true", err)
	}
}

// An error body that is not the envelope shape carries its text through, and
// that text is filtered too.
func TestCredentialIsNotCarriedByPlainErrorBody(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream said no: token "+secretKey+" expired")
	})

	_, err := c.Models(context.Background())
	if err == nil {
		t.Fatal("Models accepted a 502, want an error")
	}
	mustNotCarry(t, "the 502 diagnostic", err.Error())
	if !strings.Contains(err.Error(), "token ") {
		t.Errorf("err = %q, want the rest of the detail kept", err)
	}
}

// The key endpoint is a diagnostic path of its own, so it is checked as well.
func TestCredentialIsNotCarriedByKeyUsage(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"message":"`+secretKey+`"}}`)
	})

	_, err := c.KeyUsage(context.Background())
	if err == nil {
		t.Fatal("KeyUsage accepted a 500, want an error")
	}
	mustNotCarry(t, "the key diagnostic", err.Error())
}

// An error the stream itself carries arrives from the same place and is shown
// in the pane, so it is filtered as well.
func TestCredentialIsNotCarriedByStreamError(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `data: {"error":{"message":"bad key `+secretKey+`"}}`+"\n\n")
	})

	var reported error
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Err != nil {
			reported = e.Err
		}
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if reported == nil {
		t.Fatal("the reported stream error was not carried")
	}
	mustNotCarry(t, "the stream diagnostic", reported.Error())
	if !strings.Contains(reported.Error(), "bad key") {
		t.Errorf("err = %q, want the rest of the detail kept", reported)
	}
}

// A transport reports its own text, and a proxy behind the endpoint quotes the
// request it refused. The transport layer is the one source of a diagnostic
// this package does not write, so it is the one most likely to carry a header.
type quotingTransport struct {
	err  error
	body io.ReadCloser
	code int
}

func (t quotingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	if t.err != nil {
		return nil, t.err
	}
	code := t.code
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{
		StatusCode: code,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       t.body,
	}, nil
}

func TestCredentialIsNotCarriedByTransportError(t *testing.T) {
	c := New("https://example.invalid", secretKey)
	c.http.Transport = quotingTransport{err: errors.New(
		"proxy refused: headers Authorization: Bearer " + secretKey + " (code 403)")}

	_, err := c.Models(context.Background())
	if err == nil {
		t.Fatal("Models accepted a transport failure, want an error")
	}
	mustNotCarry(t, "the transport diagnostic", err.Error())
	if !strings.Contains(err.Error(), "proxy refused") {
		t.Errorf("err = %q, want the rest of the detail kept", err)
	}
	if !strings.Contains(err.Error(), "https://example.invalid") {
		t.Errorf("err = %q, want the endpoint named", err)
	}
}

// A transport error still unwraps to what the transport reported, so a caller
// matching on a context or a system error keeps working through the filter.
func TestCredentialFilterKeepsTheWrappedError(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	c := New("https://example.invalid", secretKey)
	c.http.Transport = quotingTransport{err: cause}

	_, err := c.Models(context.Background())
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(%v, the transport error) = false, want true", err)
	}
}

// A cancellation must survive the filter, since a caller abandoning a request
// is recognised by matching the context error.
func TestCredentialFilterKeepsContextErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the request was sent after the context was cancelled")
	})

	_, err := c.Models(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("errors.Is(%v, context.Canceled) = false, want true", err)
	}
}

// With no credential there is nothing to remove, and the diagnostic is passed
// through untouched rather than being mangled by an empty replacement.
func TestRedactIsANoOpWithoutAKey(t *testing.T) {
	c := New("https://example.invalid", "")
	if got := c.redact("nothing to hide"); got != "nothing to hide" {
		t.Errorf("redact = %q, want it unchanged", got)
	}
}

// A key that is a substring of ordinary text is still removed, since a filter
// that matched whole words would leave a partial figure behind.
func TestRedactRemovesAKeyEmbeddedInText(t *testing.T) {
	c := New("https://example.invalid", secretKey)
	got := c.redact("prefix " + secretKey + " suffix")
	if strings.Contains(got, secretKey) {
		t.Errorf("redact = %q, want the credential removed", got)
	}
	if !strings.Contains(got, "prefix ") || !strings.Contains(got, " suffix") {
		t.Errorf("redact = %q, want the surrounding text kept", got)
	}
}

// A body that cannot be decoded is a diagnostic like any other, so it goes
// through the same filter as the rest. The text of a decoding failure carries
// no part of the body, so what is asserted here is that the filter is on the
// path at all: an error built outside it is unfiltered by construction, and a
// test reading only the text could not tell the two apart.
func TestDecodeFailuresPassThroughTheCredentialFilter(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		call func(*Client) error
	}{
		{
			name: "a listing that is not JSON",
			body: `<html>maintenance</html>`,
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "a listing whose data is not a list",
			body: `{"data":"none"}`,
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "a listing holding something that is not a model",
			body: `{"data":[1]}`,
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "an allowance whose figure is not a figure",
			body: `{"data":{"usage":"a lot"}}`,
			call: func(c *Client) error {
				_, err := c.KeyUsage(context.Background())
				return err
			},
		},
		{
			name: "an allowance that is not JSON",
			body: `[]`,
			call: func(c *Client) error {
				_, err := c.KeyUsage(context.Background())
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, tc.body)
			})

			err := tc.call(c)
			if err == nil {
				t.Fatalf("the call decoded %s, want a failure", tc.body)
			}
			mustNotCarry(t, "the decoding diagnostic", err.Error())
			var filtered *redactedError
			if !errors.As(err, &filtered) {
				t.Errorf("err = %v of type %T, want it built through the filter",
					err, err)
			}
		})
	}
}
