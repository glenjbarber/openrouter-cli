package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// Conversation holds the turns exchanged in a session.
type Conversation struct {
	messages []openrouter.Message
	// model is the model every request is sent to. It is empty until a model
	// is chosen, which is reported rather than defaulted, since a guess would
	// spend the allowance on a model the user did not ask for.
	model string
	// usage accumulates the token counts reported across the session.
	usage openrouter.Usage
	// tokensIn and tokensOut accumulate the per-exchange counts, which the
	// key endpoint does not report.
	tokensIn  int
	tokensOut int
	// ephemeral marks a conversation that records nothing. An in-cognito
	// session and an ephemeral thread both set it, so that retention is
	// decided in one place rather than at each call site.
	ephemeral bool
}

// AddTokens accumulates the token counts reported for one exchange.
func (c *Conversation) AddTokens(in, out int) {
	if in > 0 {
		c.tokensIn += in
	}
	if out > 0 {
		c.tokensOut += out
	}
}

// TokensIn returns the accumulated prompt tokens.
func (c *Conversation) TokensIn() int { return c.tokensIn }

// TokensOut returns the accumulated completion tokens.
func (c *Conversation) TokensOut() int { return c.tokensOut }

// NewConversation returns an empty conversation.
func NewConversation() *Conversation {
	return &Conversation{}
}

// Model returns the selected model, empty when none has been chosen.
func (c *Conversation) Model() string { return c.model }

// SetModel chooses the model used for subsequent requests.
func (c *Conversation) SetModel(model string) {
	c.model = strings.TrimSpace(model)
}

// Usage returns the accumulated usage reported by the backend.
func (c *Conversation) Usage() openrouter.Usage { return c.usage }

// Seed adds the opening instructions from a bootstrap document.
//
// The text is added as a system turn, so it applies to every request in the
// session rather than being sent once and forgotten.
func (c *Conversation) Seed(instructions string) {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return
	}
	// A second seed replaces the first rather than stacking, so that changing
	// the document does not leave the earlier one in force.
	c.messages = nil
	c.messages = append(c.messages, openrouter.Message{
		Role:    openrouter.RoleSystem,
		Content: instructions,
	})
}

// Messages returns a copy of the turns recorded so far.
//
// The copy is what makes a save possible: the turns are written by the request
// goroutine, and a caller that held the slice would see it change under them.
func (c *Conversation) Messages() []openrouter.Message {
	return append([]openrouter.Message(nil), c.messages...)
}

// Load replaces the conversation with a saved one.
//
// The model is adopted only where none has been chosen, so that a model the
// reader picked still wins over the one a saved session was carried by. The
// counters come back as they were, since they described this conversation and
// a resumed one carrying figures from a session the reader never had would
// report spend that was not theirs.
//
// The ephemeral flag is not taken from the file. It is what the reader asked
// for in this session, and a file that could turn recording off on load would
// be a file that could defeat the mode from somewhere the reader never looked.
func (c *Conversation) Load(s *saved.Session) {
	if s == nil {
		return
	}
	c.messages = append([]openrouter.Message(nil), s.Messages...)
	c.usage = s.Usage
	c.tokensIn = s.TokensIn
	c.tokensOut = s.TokensOut
	if c.model == "" {
		c.SetModel(s.Model)
	}
}

// Pending returns the turns to send for a new user message.
//
// The user turn is included but not recorded. Recording it before the reply
// arrives would leave a user turn with no assistant turn after it if the
// request failed, which the next request would then replay as though it were
// a complete exchange.
func (c *Conversation) Pending(user string) []openrouter.Message {
	out := make([]openrouter.Message, 0, len(c.messages)+1)
	out = append(out, c.messages...)
	out = append(out, openrouter.Message{Role: openrouter.RoleUser, Content: user})
	return out
}

// Record stores the exchange once a reply has been received.
// An ephemeral conversation keeps nothing. The request itself still carried
// the history, so the model has the context; only the retention is skipped.
func (c *Conversation) Record(user, reply string) {
	if c.ephemeral {
		return
	}
	c.messages = append(c.messages,
		openrouter.Message{Role: openrouter.RoleUser, Content: user},
		openrouter.Message{Role: openrouter.RoleAssistant, Content: reply},
	)
}

// Reset clears the turns but keeps the opening instructions.
//
// The distinction matters: clearing the conversation is a normal action, while
// losing the bootstrap document would silently change how the model behaves.
//
// A summary is a system turn as the instructions are, but it is recorded work.
// Clearing a conversation is a request to stop carrying the work, and a
// description of the work carried in its place would defeat it.
func (c *Conversation) Reset() {
	if len(c.messages) > 0 {
		first := c.messages[0]
		if first.Role == openrouter.RoleSystem && !isCompaction(first.Content) {
			c.messages = c.messages[:1]
			return
		}
	}
	c.messages = nil
}

// tokenCount renders a token figure for the status bar.
func tokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// hostname reports the local host for the status bar, or a dash when it cannot
// be determined.
func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return placeholder
	}
	return h
}
