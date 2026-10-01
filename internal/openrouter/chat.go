package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
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
// A reply is always streamed, so the request carries no switch for it. A field
// the caller could set and the client then overwrote would promise a scripted
// path that does not exist, and a reader of the struct would be misled by it.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
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

// UnmarshalJSON decodes the accounting without refusing a figure the platform
// cannot hold.
//
// The fields are ints because that is what the interface uses, but a plain
// decode of a figure above the platform maximum is an error, and an error in
// this chunk ends the stream. A count too large to hold is therefore clamped
// rather than allowed to cut a reply short. A figure written as a string, or
// written in exponent form, is read rather than refused, since the endpoint is
// not under this repository's control.
func (u *streamUsage) UnmarshalJSON(b []byte) error {
	var wide struct {
		PromptTokens     json.RawMessage `json:"prompt_tokens"`
		CompletionTokens json.RawMessage `json:"completion_tokens"`
	}
	if err := json.Unmarshal(b, &wide); err != nil {
		return err
	}
	u.PromptTokens = tokenCount(wide.PromptTokens)
	u.CompletionTokens = tokenCount(wide.CompletionTokens)
	return nil
}

// tokenCount reads a count from a JSON value in any of the shapes the endpoint
// has used. An absent or null value is zero, which is what an absent count
// means, and the pointer on the field is what tells an absent count from a
// reported zero.
func tokenCount(raw json.RawMessage) int {
	text := scalarText(raw)
	if text == "" {
		return 0
	}
	if v, err := strconv.ParseInt(text, 10, 64); err == nil {
		return clampInt(v)
	}
	if v, err := strconv.ParseFloat(text, 64); err == nil {
		// The bound is a power of two below the platform maximum, so the
		// comparison is exact in floating point on every platform and the
		// conversion below cannot round past the maximum.
		limit := math.Ldexp(1, strconv.IntSize-2)
		switch {
		case v >= limit:
			return maxInt
		case v <= -limit:
			return minInt
		default:
			return int(v)
		}
	}
	return 0
}

// scalarText returns a JSON string or number as the text it arrived as.
func scalarText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(string(raw))
}

// The bounds an int is clamped to. They are computed rather than written down,
// since the platform maximum differs between a 32-bit and a 64-bit build.
var (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// clampInt holds a wider figure in an int without changing its meaning.
func clampInt(v int64) int {
	switch {
	case v > int64(maxInt):
		return maxInt
	case v < int64(minInt):
		return minInt
	default:
		return int(v)
	}
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
	// Kind names what the event carried, so that a caller watching a stream
	// can tell a text delta from metadata without inferring it from the
	// fields it happens to set. It is one of the EventKind constants. The
	// fields above are unchanged and remain the payload; Kind is the label
	// describing them.
	Kind string
	// Finish is the reason the model gave for ending the turn, reported on a
	// final choice alongside an empty delta. It is empty on every other
	// event.
	Finish string
}

// The kinds a stream event can carry.
//
// The names are short because they are shown in a verbose listing, and a
// reader scanning a stream wants the shape rather than a sentence.
const (
	EventDelta = "delta"
	EventUsage = "usage"
	EventDone  = "done"
	EventError = "error"
	EventEnd   = "finish"
)

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
	body, err := json.Marshal(withUsage(req))
	if err != nil {
		return c.wrapf(err, "encoding the request")
	}

	httpReq, err := c.newStreamRequest(ctx, "/chat/completions", body)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return c.wrapf(err, "contacting %s", c.baseURL)
	}
	defer resp.Body.Close()

	// The body is the stream on success, so the status is examined without
	// reading it. Reading it here would consume the reply before the stream
	// parser saw it, and the caller would receive nothing.
	if resp.StatusCode != http.StatusOK {
		// The body is not a stream on this path, so it is read to find out
		// what the server said. It is bounded, since an error page from an
		// endpoint this repository does not control need not be a page. A
		// read that fails is not reported: the status is the part that
		// decides the outcome, and a partial detail beats none.
		data, _ := readLimited(resp.Body, maxDetail)
		return c.statusError(resp.StatusCode, data)
	}
	return c.readStream(resp.Body, onEvent)
}

// maxDetail bounds the body of an error response, where the body is a message
// rather than a stream. A figure well above any message the backend writes is
// here so that an endless body cannot be read into memory.
const maxDetail = 1 << 20

// newStreamRequest builds a streaming request.
//
// The accept header asks for an event stream, since the response is not JSON
// but a sequence of server-sent events.
func (c *Client) newStreamRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, c.wrapf(err, "building the request")
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

// maxStreamLine bounds a single line of the event stream.
//
// A line longer than this ends the reply rather than being cut without saying
// so, since a delta missing its tail is text that is wrong with nothing to show
// that it is. The figure is well above any delta a model writes, and the end is
// reported the way every other failure on this path is reported, which is
// through the callback with the text that did arrive kept.
const maxStreamLine = 8 << 20

// readStream decodes a server-sent event stream.
//
// The encoding is a sequence of lines, each prefixed with a field name. A line
// beginning with "data:" carries the payload, a line beginning with a colon is a
// comment used as a keep-alive, and a blank line ends an event. The literal
// [DONE] marks the end of the stream.
func (c *Client) readStream(body io.Reader, onEvent func(StreamEvent)) error {
	// The terminating marker is what distinguishes a complete reply from a
	// stream that was cut short, so it is required rather than assumed.
	sawDone := false
	scanner := bufio.NewScanner(body)
	// A single event may exceed the default limit when a model returns a long
	// completion in one delta, so the buffer is raised rather than letting
	// the scan fail partway through a reply.
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLine)

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
			onEvent(StreamEvent{Done: true, Kind: EventDone})
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			// A payload that is not JSON is reported rather than skipped, so
			// a truncated reply is not presented as a complete one.
			onEvent(StreamEvent{Err: c.wrapf(err, "decoding a stream event"), Kind: EventError})
			return nil
		}
		if chunk.Error != nil {
			onEvent(StreamEvent{
				Err:  c.wrapf(nil, "the stream reported an error: %s", chunk.Error.Message),
				Kind: EventError,
			})
			return nil
		}
		if chunk.Usage != nil {
			onEvent(StreamEvent{Usage: chunk.Usage, Kind: EventUsage})
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				onEvent(StreamEvent{Kind: EventEnd, Finish: choice.FinishReason})
			}
			if choice.Delta.Content != "" {
				onEvent(StreamEvent{Content: choice.Delta.Content, Kind: EventDelta})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// The failure is reported through the callback rather than returned,
		// because that is where every other failure on this path is reported
		// and because a caller acting on a return value drops the text that
		// arrived before the failure. A single line longer than the buffer is
		// the case this covers, and it ends the reply just as a cut
		// connection does.
		onEvent(StreamEvent{
			Err:  c.wrapf(err, "reading the stream"),
			Kind: EventError,
		})
		return nil
	}
	if !sawDone {
		onEvent(StreamEvent{
			Err:  fmt.Errorf("the stream ended before it was complete"),
			Kind: EventError,
		})
	}
	return nil
}
