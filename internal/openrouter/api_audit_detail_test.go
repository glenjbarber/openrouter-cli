package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// A diagnostic is shown in the pane and stays in the scrollback, so the amount
// of a remote body quoted into one is bounded. What is past the bound is not
// read for a decision, and a body that carries the credential near its end is
// filtered before it is cut, since a cut applied first would leave the part
// holding the key outside the text that is filtered and reported.

func TestDiagnosticQuotesOnlyABoundedAmountOfTheBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{
			name: "a status with no named error",
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "a rejected key",
			call: func(c *Client) error {
				_, err := c.Models(context.Background())
				return err
			},
		},
		{
			name: "an exhausted allowance",
			call: func(c *Client) error {
				_, err := c.KeyUsage(context.Background())
				return err
			},
		},
		{
			name: "an error status on a stream",
			call: func(c *Client) error {
				return c.Chat(context.Background(), ChatRequest{
					Model:    "test/model",
					Messages: []Message{{Role: RoleUser, Content: "hi"}},
				}, func(StreamEvent) {})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"error":{"message":"` + secretKey + " " + strings.Repeat("w", 8*maxQuote) + `"}}`
			var code int
			switch tc.name {
			case "a rejected key":
				code = http.StatusUnauthorized
			case "an exhausted allowance":
				code = http.StatusTooManyRequests
			default:
				code = http.StatusBadGateway
			}
			c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				io.WriteString(w, body)
			})

			err := tc.call(c)
			if err == nil {
				t.Fatal("the call accepted the status, want an error")
			}
			mustNotCarry(t, "the diagnostic", err.Error())
			if len(err.Error()) > 1024 {
				t.Errorf("the diagnostic is %d bytes, want the quoted body bounded", len(err.Error()))
			}
			if !strings.Contains(err.Error(), "...") {
				t.Errorf("err = %q, want the cut stated rather than silent", err)
			}
			// The detail is kept, since it is read for a decision. Only the
			// tail of a very long one is dropped.
			if !strings.Contains(err.Error(), redacted) {
				t.Errorf("err = %q, want the credential marked as removed", err)
			}
		})
	}
}

// A detail short enough to be shown whole is not cut, since the bound is there
// to keep a terminal readable rather than to be tidy.
func TestShortDetailIsQuotedWhole(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, `{"error":{"message":"upstream is closed for maintenance"}}`)
	})

	_, err := c.Models(context.Background())
	if err == nil {
		t.Fatal("Models accepted a 502, want an error")
	}
	if !strings.Contains(err.Error(), "upstream is closed for maintenance") {
		t.Errorf("err = %q, want the whole message", err)
	}
	if strings.Contains(err.Error(), "...") {
		t.Errorf("err = %q, want no cut on a short detail", err)
	}
}

// A message longer than the bound in characters rather than in bytes is cut on
// a character boundary, since a diagnostic is read as text and half a character
// would show as a replacement mark.
func TestQuotedDetailIsCutOnACharacterBoundary(t *testing.T) {
	c := New("https://example.invalid", "k")
	// One character of one byte in front of characters of two, so that the cut
	// lands in the middle of a character rather than on a boundary by accident.
	text := "x" + strings.Repeat("\u00e9", maxQuote)
	err := c.quote([]byte(text))
	if !strings.HasPrefix(err, "x\u00e9") {
		t.Fatalf("err = %q, want the quote to begin with the text", err)
	}
	if strings.ContainsRune(err, '\ufffd') {
		t.Errorf("err = %q, want no replacement mark from a cut inside a character", err)
	}
	if len(err) > maxQuote+3 {
		t.Errorf("err is %d bytes, want the quote bounded", len(err))
	}
}

// A credential never reaches the diagnostic, whether it falls inside the text
// that is quoted or past the point where the quote is cut. A credential inside
// the kept text is marked as removed, since a reader seeing the rest of a
// rejection wants to know that something was taken out of it.
func TestCredentialIsRemovedFromTheQuotedDetail(t *testing.T) {
	c := New("https://example.invalid", secretKey)

	inside := c.quote([]byte("rejected: " + secretKey + " " + strings.Repeat("v", 4*maxQuote)))
	if strings.Contains(inside, secretKey) {
		t.Errorf("quote = %q, want the credential removed", inside)
	}
	if !strings.Contains(inside, redacted) {
		t.Errorf("quote = %q, want the credential marked as removed", inside)
	}
	if len(inside) > maxQuote+3 {
		t.Errorf("quote is %d bytes, want it bounded", len(inside))
	}

	past := c.quote([]byte(strings.Repeat("v", 4*maxQuote) + " rejected: " + secretKey))
	if strings.Contains(past, secretKey) {
		t.Errorf("quote = %q, want the credential absent", past)
	}
}

// The named errors still unwrap through the filter, so a caller matching on one
// of them keeps working after the detail is quoted rather than carried.
func TestNamedErrorsSurviveTheQuotedDetail(t *testing.T) {
	c := auditServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, strings.Repeat("z", 8*maxQuote))
	})

	_, err := c.Models(context.Background())
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("errors.Is(%v, ErrUnauthorized) = false, want true", err)
	}
}
