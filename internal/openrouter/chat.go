package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Message is one turn of a conversation.
//
// The role is sent as given, so an assistant message is replayed as an
// assistant message rather than as a user one.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// The roles the API accepts.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// ChatRequest is a completion request.
//
// Stream is held so that a caller may turn streaming off, which is what the
// scripted path uses.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// streamUsage is the token accounting reported on a streamed reply.
//
// It arrives on the final chunk rather than on every delta, so the field is a
// pointer: an absent value and a reported zero are different, and a plain
// integer could not tell them apart.
type streamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// StreamEvent is one increment of a streamed reply.
type StreamEvent struct {
	// Content is the text delta. It is empty on an event carrying only
	// metadata.
	Content string
	// Usage is the token accounting, reported once on the final chunk. It is
	// nil on every other event.
	Usage *streamUsage
	// Done reports that the stream has finished.
	Done bool
	// Err carries a transport or decoding failure that ended the stream.
	// A stream that ends badly is reported through the callback rather than
	// as a return value, since the text received before the failure is
	// still worth keeping.
	Err error
}

// Chat streams a completion.
//
// The callback is called for each increment, in order, and must not be called
// after it returns an error. An error passed to the callback ends the stream,
// which is how a client stops a reply the user has abandoned.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onEvent func(StreamEvent)) error {
	if req.Model == "" {
		return fmt.Errorf("no model is selected")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("there is nothing to send")
	}
	req.Stream = true

	body, err := json.Marshal(withUsage(req))
	if err != nil {
		return fmt.Errorf("encoding the request: %w", err)
	}

	httpReq, err := c.newStreamRequest(ctx, "/chat/completions", body)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("contacting %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	// The body is the stream on success, so the status is examined without
	// reading it. Reading it here would consume the reply before the stream
	// parser saw it, and the caller would receive nothing.
	if resp.StatusCode != http.StatusOK {
		return statusError(resp.StatusCode, readAll(resp.Body))
	}
	return readStream(resp.Body, onEvent)
}

// newStreamRequest builds a streaming request.
//
// The accept header asks for an event stream, since the response is not JSON
// but a sequence of server-sent events.
func (c *Client) newStreamRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if err := c.authorize(req); err != nil {
		return nil, err
	}
	return req, nil
}

// streamChunk is one decoded event of a completion stream.
//
// The delta is a list because the field is an array in the API even though the
// first element carries the text in practice.
type streamChunk struct {
	Usage   *streamUsage `json:"usage"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// readStream decodes a server-sent event stream.
//
// The encoding is a sequence of lines, each prefixed with a field name. A line
// beginning with "data:" carries the payload, a line beginning with a colon is a
// comment used as a keep-alive, and a blank line ends an event. The literal
// [DONE] marks the end of the stream.
func readStream(body io.Reader, onEvent func(StreamEvent)) error {
	// The terminating marker is what distinguishes a complete reply from a
	// stream that was cut short, so it is required rather than assumed.
	sawDone := false
	scanner := bufio.NewScanner(body)
	// A single event may exceed the default limit when a model returns a long
	// completion in one delta, so the buffer is raised rather than letting
	// the scan fail partway through a reply.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)

		if payload == "[DONE]" {
			sawDone = true
			onEvent(StreamEvent{Done: true})
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A payload that is not JSON is reported rather than skipped, so
			// a truncated reply is not presented as a complete one.
			onEvent(StreamEvent{Err: fmt.Errorf("decoding a stream event: %w", err)})
			return nil
		}
		if chunk.Error != nil {
			onEvent(StreamEvent{Err: fmt.Errorf("the stream reported an error: %s", chunk.Error.Message)})
			return nil
		}
		if chunk.Usage != nil {
			onEvent(StreamEvent{Usage: chunk.Usage})
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				onEvent(StreamEvent{Content: choice.Delta.Content})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading the stream: %w", err)
	}
	if !sawDone {
		onEvent(StreamEvent{
			Err: fmt.Errorf("the stream ended before it was complete"),
		})
	}
	return nil
}

// readAll reads a body, used for an error response where the body is the
// detail rather than a stream.
func readAll(body io.Reader) []byte {
	data, _ := io.ReadAll(io.LimitReader(body, 1<<20))
	return data
}
