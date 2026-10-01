package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the OpenRouter.AI API.
//
// It is built on the standard library only, so the client carries no dependency
// for its API access.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// ErrNoKey reports that no API key is configured.
//
// It is returned rather than being sent as an empty credential, since the
// backend answers an empty key with a 401 that does not say which of the two
// mistakes was made.
var ErrNoKey = errors.New("no API key is configured")

// ErrUnauthorized reports that the key was rejected.
var ErrUnauthorized = errors.New("the API key was rejected")

// ErrRateLimited reports that the key has exhausted its allowance.
var ErrRateLimited = errors.New("the usage limit has been reached")

// New returns a client for the given base URL and key.
//
// The base URL is used verbatim, so an override carrying a full path is not
// altered. A deadline is set on the whole request, body included, so that a
// connection which never answers cannot hang the session for ever. It is a
// whole-request deadline rather than a header timeout, so the cost of it is
// that a reply still streaming after it is cut. The figure is long enough that
// a reasoning model taking minutes over its first token is not mistaken for a
// dead one.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http: &http.Client{
			// Timeout covers the request through the last byte of the body,
			// not the wait for the headers alone. A header-only bound would
			// be set on the transport instead, and which of the two is wanted
			// is a decision for the maintainer rather than one taken here.
			Timeout: 10 * time.Minute,
		},
	}
}

// redacted is what a diagnostic carries in place of the credential.
const redacted = "[redacted]"

// maxResponse bounds a body that is not a stream.
//
// The largest legitimate one is the model catalogue, which is a few hundred
// entries. The limit sits well above that, and it is here so that a server
// answering with an endless body cannot exhaust memory: an unbounded read from
// an endpoint this repository does not control is a way to lose the session
// rather than a way to read a reply.
const maxResponse = 32 << 20

// redactedError carries a message with the credential removed while leaving the
// underlying error reachable, so errors.Is and errors.As still work.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redact removes the credential from a string bound for a diagnostic.
func (c *Client) redact(s string) string {
	if c.apiKey == "" {
		return s
	}
	return strings.ReplaceAll(s, c.apiKey, redacted)
}

// wrapf builds a diagnostic with the credential removed from the text.
//
// The credential travels in a header, so it is in no request body this client
// builds and in no URL it requests. It can still come back from the far side:
// a server that quotes the request in the body it returns, or a proxy that
// quotes it in the message it reports. Every string built here is shown to a
// reader and can end in a terminal history, so each one is filtered rather
// than trusted to be free of it.
func (c *Client) wrapf(err error, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if err != nil {
		msg += ": " + err.Error()
	}
	return &redactedError{msg: c.redact(msg), err: err}
}

// maxQuote bounds how much of a body is quoted in a diagnostic.
//
// A diagnostic is shown in the pane and stays in the scrollback, so quoting a
// whole body would put up to maxResponse of a server's text into the terminal.
// The figure is far above any message the backend writes, and what falls past it
// is not read for a decision.
const maxQuote = 512

// quote returns what a body has to say, ready to be shown.
//
// The whole body is filtered before any of it is kept, so what is quoted has
// already had the credential removed from it rather than being filtered after a
// part of it was dropped. The cut falls on a character boundary, since a
// diagnostic is read as text and half a character would show as a replacement
// mark.
func (c *Client) quote(body []byte) string {
	text := c.redact(detail(body))
	if len(text) <= maxQuote {
		return text
	}
	return strings.ToValidUTF8(text[:maxQuote], "") + "..."
}

// withDetail returns err carrying the detail the server reported.
func (c *Client) withDetail(err error, body []byte) error {
	return &redactedError{msg: err.Error() + ": " + c.quote(body), err: err}
}

// readLimited reads at most limit bytes of a body and reports rather than
// truncates. A truncation would leave invalid JSON behind and be reported as a
// decoding failure, which does not say that the body was the problem.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("the response exceeded %d bytes", limit)
	}
	return data, nil
}

// HasKey reports whether a credential is present, without revealing it.
func (c *Client) HasKey() bool { return c.apiKey != "" }

// BaseURL returns the endpoint the client was built against.
//
// The credential is deliberately not exposed, since a diagnostic that printed
// it would put the key in a terminal history or a log.
func (c *Client) BaseURL() string { return c.baseURL }

// do performs a request and returns the response body on success.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, c.wrapf(err, "encoding the request")
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, c.wrapf(err, "building the request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorize(req); err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.wrapf(err, "contacting %s", c.baseURL)
	}
	defer resp.Body.Close()

	data, err := readLimited(resp.Body, maxResponse)
	if err != nil {
		return nil, c.wrapf(err, "reading the response")
	}
	if err := c.statusError(resp.StatusCode, data); err != nil {
		return nil, err
	}
	return data, nil
}

// authorize sets the credential headers.
func (c *Client) authorize(req *http.Request) error {
	if c.apiKey == "" {
		return ErrNoKey
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	// Both are optional and used by OpenRouter for attribution. They are set
	// so that the client is identifiable rather than anonymous.
	req.Header.Set("HTTP-Referer", "https://github.com/glenjbarber/openrouter-cli")
	req.Header.Set("X-OpenRouter-Title", "openrouter-cli")
	return nil
}

// statusError converts a non-200 response into an error.
//
// The status is mapped to a named error where one exists, since the actionable
// outcomes are an unusable key and an exhausted allowance, and the body text is
// kept for anything not covered.
func (c *Client) statusError(code int, body []byte) error {
	switch code {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return c.withDetail(ErrUnauthorized, body)
	case http.StatusTooManyRequests:
		return c.withDetail(ErrRateLimited, body)
	default:
		return c.wrapf(nil, "the server returned HTTP %d: %s", code, c.quote(body))
	}
}

// detail extracts the message the backend reported, if any.
func detail(body []byte) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		return envelope.Error.Message
	}
	if trimmed := strings.TrimSpace(string(body)); trimmed != "" {
		return trimmed
	}
	return "no detail was given"
}
