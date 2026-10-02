package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// compactionThreshold is the share of the window at which compaction runs.
//
// The figure is deliberately well below the whole window. Compaction costs a
// request of its own, and leaving only a small remainder would compact again
// almost immediately after the next exchange, so the work has to start while
// there is still room to do it.
const compactionThreshold = 0.75

// compactionReserve is the share of the window kept as headroom after
// compacting, expressed as a count of tokens rather than a share, since a
// share of a very large window is more than any reply needs.
const compactionReserve = 8_000

// compactionModel is the model used to write the summary.
//
// The session model is used unless one is named here. A summary is a small,
// mechanical piece of work, and a cheaper model produces an adequate one, but
// choosing silently would spend the allowance on a model the user did not ask
// for, so the session model is used by default.
const compactionModel = ""

// CompactionError reports a compaction that could not be completed.
//
// The conversation is left untouched when one occurs, since a half-summarised
// history is worse than a long one.
type CompactionError struct {
	Reason string
}

// Error implements the error interface.
func (e *CompactionError) Error() string { return "compaction failed: " + e.Reason }

// Summarise asks the model to reduce a set of messages to a single summary
// turn.
//
// The instruction asks for facts rather than prose, since a summary that
// paraphrases the intent is useless for continuing the work and one that
// records decisions, constraints, and open questions is not.
func Summarise(ctx context.Context, c *openrouter.Client, model string, msgs []openrouter.Message) (string, error) {
	return SummariseCounted(ctx, c, model, msgs, nil)
}

// SummariseCounted is Summarise with a function that is told what the request
// reported, so that the spend of a compaction reaches the session total. The
// function is nil for a caller that has none, and is given the model that
// answered, which is the default when none was named.
func SummariseCounted(ctx context.Context, c *openrouter.Client, model string, msgs []openrouter.Message, onUsage usageFunc) (string, error) {
	if len(msgs) == 0 {
		return "", &CompactionError{Reason: "there is nothing to summarise"}
	}

	prompt := []openrouter.Message{{
		Role: openrouter.RoleSystem,
		Content: "Summarise the following conversation so that it can replace " +
			"the original without losing anything needed to continue.\n\n" +
			"Keep, as plain statements:\n" +
			"- decisions made and the reasoning behind them\n" +
			"- constraints, requirements, and preferences the user stated\n" +
			"- file names, identifiers, and commands that matter\n" +
			"- anything left unresolved, and what was being attempted\n\n" +
			"Drop greetings, restatements, and anything already superseded.\n" +
			"Do not add anything that was not said. Write plain prose, no preamble.",
	}}

	transcript := &strings.Builder{}
	for _, m := range msgs {
		// The role is written out so the summary can tell an instruction from
		// a question, which a bare transcript would lose.
		fmt.Fprintf(transcript, "%s: %s\n\n", m.Role, m.Content)
	}
	prompt = append(prompt, openrouter.Message{
		Role:    openrouter.RoleUser,
		Content: transcript.String(),
	})

	if model == "" {
		model = compactionModel
	}
	if model == "" {
		return "", &CompactionError{Reason: "no model is available to summarise with"}
	}

	var out strings.Builder
	var failed error
	err := c.Chat(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: prompt,
	}, func(e openrouter.StreamEvent) {
		if e.Usage != nil && onUsage != nil {
			onUsage(model, e)
		}
		if e.Err != nil {
			failed = e.Err
			return
		}
		out.WriteString(e.Content)
	})
	if err != nil {
		return "", err
	}
	if failed != nil {
		return "", failed
	}

	summary := strings.TrimSpace(out.String())
	if summary == "" {
		return "", &CompactionError{Reason: "the model returned no summary"}
	}
	return summary, nil
}

// Compact replaces the recorded turns with a summary of them.
//
// The opening instructions are kept, since losing them would silently change
// how the model behaves. A system turn written by this function is marked so
// that a second compaction replaces it rather than summarising a summary,
// which would compound the loss on every pass.
func (c *Conversation) Compact(summary string) {
	kept := make([]openrouter.Message, 0, 2)
	for _, m := range c.messages {
		if m.Role == openrouter.RoleSystem && !strings.HasPrefix(m.Content, compactionMarker) {
			kept = append(kept, m)
		}
	}
	kept = append(kept, openrouter.Message{
		Role:    openrouter.RoleSystem,
		Content: compactionMarker + "\n\n" + summary,
	})
	c.messages = kept
}

// compactionMarker prefixes a summary turn so that it is recognisable.
const compactionMarker = "[compacted]"

// HasSummary reports whether the conversation already carries a summary.
func (c *Conversation) HasSummary() bool {
	for _, m := range c.messages {
		if strings.HasPrefix(m.Content, compactionMarker) {
			return true
		}
	}
	return false
}

// compact summarises the conversation and replaces it with the summary.
//
// A manual request runs regardless of size, since asking for it is the point.
// The conversation is left untouched when the summary fails, so a failure
// cannot cost the user their history.
func (s *Session) compact(manual bool) {
	if msg := s.credentialProblem(); msg != "" {
		s.addReply(msg)
		return
	}
	if s.conv.Model() == "" {
		s.addReply("no model is selected: /model NAME")
		return
	}
	if !manual && !s.conv.HasSummary() && s.conv.Turns() < minCompactionTurns {
		return
	}

	toSummarise := s.conv.forSummary()
	if len(toSummarise) == 0 {
		if manual {
			s.addReply("there is nothing to compact")
		}
		return
	}

	before := s.conv.EstimatedTokens()
	s.addReply("compacting the conversation...")
	// The twiddle turns for the compaction as it does for a request, since it
	// costs a request of its own and the frame would otherwise sit still for
	// the length of it.
	s.beginWork()
	defer s.endWork()

	summary, err := SummariseCounted(s.ctx, s.client, s.conv.Model(), toSummarise, s.noteSideSpend)
	if err != nil {
		s.addReplyKind(kindFailure, "(error) "+err.Error())
		return
	}

	s.conv.Compact(summary)
	s.addReply(compactionNotice(before, s.conv.EstimatedTokens()))
}

// maybeCompact runs a compaction when the conversation has grown too large.
//
// It is called before a request is sent, since a request that exceeds the
// window is refused by the backend and costs the turn. Compacting beforehand
// means the request is sent smaller rather than not sent at all.
func (s *Session) maybeCompact() {
	s.maybeCompactFor(nil)
}

// maybeCompactFor runs a compaction when the conversation and the messages
// about to be sent have grown too large.
//
// It is called before every request of a turn rather than once before the
// first, since a turn that called tools makes several requests and the result
// of one is usually bigger than the question that asked for it. An estimate
// taken once at the start of the turn would not see any of it.
func (s *Session) maybeCompactFor(inflight []openrouter.Message) {
	if s.client == nil || s.conv.Model() == "" {
		return
	}
	window := s.windows.lookup(s.ctx, s, s.conv.Model())
	// The turns in flight count towards the threshold as well as towards the
	// estimate, since a turn holding several results is short of the turn
	// count on its own and would otherwise be left alone however large it grew.
	turns := s.conv.Turns() + len(inflight)
	if !shouldCompact(window, turns, s.conv.EstimatedTokensWith(inflight)) {
		return
	}
	s.compact(false)
}
