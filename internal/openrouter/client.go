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
// altered. The client has no timeout of its own on the request body, since a
// reasoning model can legitimately take minutes to produce its first token.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http: &http.Client{
			// The header timeout bounds a stalled connection. It is longer
			// than a typical generation so that a slow first token is not
			// mistaken for a dead one.
			Timeout: 10 * time.Minute,
		},
	}
}

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
			return nil, fmt.Errorf("encoding the request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorize(req); err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the response: %w", err)
	}
	if err := statusError(resp.StatusCode, data); err != nil {
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
func statusError(code int, body []byte) error {
	switch code {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", ErrUnauthorized, detail(body))
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", ErrRateLimited, detail(body))
	default:
		return fmt.Errorf("the server returned HTTP %d: %s", code, detail(body))
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
