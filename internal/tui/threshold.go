package tui

import (
	"context"
	"fmt"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// contextLength remembers the window size of the selected model.
//
// The figure comes from the model list rather than from the completion
// response, so it is fetched once per model and cached. Fetching it per request
// would add a call to every message for a value that does not change.
type contextLength struct {
	byModel map[string]int
}

// newContextLength returns an empty cache.
func newContextLength() *contextLength {
	return &contextLength{byModel: map[string]int{}}
}

// fallbackWindow is used when the model list does not report a size.
//
// A window is needed for the threshold to mean anything, and guessing a small
// one would compact constantly while guessing a large one would compact too
// late. The figure is a common modern window rather than a small one, on the
// reasoning that compacting early is cheap and compacting too late is not.
const fallbackWindow = 128_000

// lookup returns the window for model, fetching it once if needed.
func (c *contextLength) lookup(ctx context.Context, s *Session, model string) int {
	// A session built without a cache has no window to report, which is the
	// case for a session assembled by a test rather than started.
	if c == nil {
		return fallbackWindow
	}
	if model == "" {
		return fallbackWindow
	}
	if n, ok := c.byModel[model]; ok && n > 0 {
		return n
	}

	if s.client == nil {
		return fallbackWindow
	}
	models, err := s.client.Models(ctx)
	if err != nil {
		return fallbackWindow
	}
	for _, m := range models {
		if m.ContextLength > 0 {
			c.byModel[m.ID] = m.ContextLength
		}
	}
	if n, ok := c.byModel[model]; ok {
		return n
	}
	// A model the list does not carry, such as one reached through an alias,
	// falls back rather than reporting an unbounded window, which would
	// disable compaction entirely.
	return fallbackWindow
}

// shouldCompact reports whether the conversation has grown past the point at
// which compaction is worth running.
//
// Two conditions must hold. The share of the window in use must be past the
// threshold, and there must be enough turns for a summary to save anything. A
// conversation holding two long messages is compacted into something no shorter
// than itself, which costs a request to achieve nothing.
func shouldCompact(window, turns, estimatedTokens int) bool {
	if window <= 0 || turns < minCompactionTurns {
		return false
	}
	return float64(estimatedTokens) > float64(window)*compactionThreshold
}

// minCompactionTurns is the number of recorded turns below which compaction is
// not attempted.
const minCompactionTurns = 4

// compactionNotice is the line shown while a compaction runs.
func compactionNotice(before, after int) string {
	return fmt.Sprintf("compacted: %s -> %s estimated tokens",
		compactNumber(before), compactNumber(after))
}

// compactNumber renders a token figure for a message.
func compactNumber(n int) string { return tokenCount(n) }

// recentTurns is the number of trailing turns held out of a summary request.
//
// The most recent exchange is kept out so that the model answering next still
// has the immediate context in front of it, rather than only a description of
// it. Two turns is the question and the reply it produced.
const recentTurns = 2

// forSummary returns the turns to summarise, excluding the instructions and
// the most recent exchange.
//
// The instructions are excluded because Compact keeps the original, and a
// summary of them would compete with it rather than add to it.
func (c *Conversation) forSummary() []openrouter.Message {
	var out []openrouter.Message
	for _, m := range c.messages {
		if m.Role == openrouter.RoleSystem {
			continue
		}
		out = append(out, m)
	}
	if len(out) > recentTurns {
		out = out[:len(out)-recentTurns]
	}
	return out
}

// EstimatedTokens guesses the size of the recorded conversation.
func (c *Conversation) EstimatedTokens() int {
	return estimateTokens(c.messages)
}

// EstimatedTokensWith guesses the size the conversation would be once the
// given messages were recorded.
//
// The check runs inside a turn, where the messages about to be sent are not yet
// part of the conversation. A tool result is the largest text an agentic turn
// carries, so an estimate that ignored what was in flight would be wrong
// exactly when the size of the result decides whether the next request fits.
func (c *Conversation) EstimatedTokensWith(extra []openrouter.Message) int {
	if len(extra) == 0 {
		return estimateTokens(c.messages)
	}
	all := make([]openrouter.Message, 0, len(c.messages)+len(extra))
	all = append(all, c.messages...)
	all = append(all, extra...)
	return estimateTokens(all)
}

// estimateTokens guesses the size of a set of turns.
func estimateTokens(msgs []openrouter.Message) int {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Role) + len(m.Content) + 4
		// The calls an assistant turn carries are counted with the turn they
		// belong to. They are small beside a result, but they are text the
		// next request replays, and a model that calls in a loop is the case
		// where the estimate decides anything.
		for _, call := range m.ToolCalls {
			chars += len(call.Function.Name) + len(call.Function.Arguments) + len(call.ID)
		}
	}
	// Four characters per token is an approximation, since counting exactly
	// needs the model tokenizer. It is deliberately generous, so that
	// compaction starts before a request would actually fail rather than
	// after.
	return chars / 4
}

// Turns reports the number of recorded turns.
func (c *Conversation) Turns() int { return len(c.messages) }
