package tui

import (
	"fmt"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// setEphemeral marks a conversation as recording nothing.
//
// A thread needs the context to answer, so the turns it was branched from are
// present, but nothing sent in it is retained. Keeping them would leave the
// work reachable and would make the token counters report a total that is then
// discarded.
func (c *Conversation) setEphemeral() { c.ephemeral = true }

// Recording reports whether the conversation keeps what is sent to it.
func (c *Conversation) Recording() bool { return !c.ephemeral }

// Branch is a snapshot of a conversation at a point in time.
//
// A branch is taken by copying the turns, so that the branch and the main
// conversation cannot affect one another afterwards. Sharing the slice instead
// would let an exchange in one appear in the other, which defeats the point of
// branching at all.
type Branch struct {
	// name labels the branch in the interface.
	name string
	// turns is the copied history.
	turns []openrouter.Message
	// model is the model in force when the branch was taken, since a branch
	// may be compared against another model without that being a change to
	// the main conversation.
	model string
	// hasSummary records whether the copied history carried a summary, so
	// that returning to it restores the compaction state rather than
	// compacting a history that was already compacted.
	hasSummary bool
}

// TakeBranch copies the current conversation.
func (c *Conversation) TakeBranch(name string) *Branch {
	b := &Branch{
		name:       name,
		model:      c.model,
		hasSummary: c.HasSummary(),
	}
	// The copy is the point: the two must not share storage.
	b.turns = make([]openrouter.Message, len(c.messages))
	copy(b.turns, c.messages)
	return b
}

// Name returns the branch label.
func (b *Branch) Name() string { return b.name }

// Turns returns the number of recorded turns.
func (b *Branch) Turns() int { return len(b.turns) }

// Restore replaces the conversation with the branch.
//
// The main conversation is not restored afterwards by this call: a branch is
// entered and left deliberately, and returning to the previous state is the
// caller business, so that leaving a branch cannot lose the conversation that
// was interrupted.
func (c *Conversation) Restore(b *Branch) {
	c.messages = make([]openrouter.Message, len(b.turns))
	copy(c.messages, b.turns)
	if b.model != "" {
		c.model = b.model
	}
}

// Ephemeral returns a conversation that began from a branch.
//
// The turns are recorded so that the model has the context, but the branch
// marks itself discarded when it is dropped, and nothing writes it anywhere.
// An ephemeral thread is not persisted and is not carried into a later
// session.
type Ephemeral struct {
	conv *Conversation
	// origin is the conversation to return to on leaving.
	origin *Branch
	// count is how many exchanges the thread has held, for the interface to
	// report. The turns themselves are not kept, so the count is the only
	// trace a thread leaves behind it.
	count int
}

// BeginEphemeral starts a thread branched from the current conversation.
func (c *Conversation) BeginEphemeral(name string) *Ephemeral {
	e := &Ephemeral{
		conv:   NewConversation(),
		origin: c.TakeBranch("main"),
	}
	// The model is carried over, since a thread about the same work is
	// useless with a different model by accident.
	e.conv.model = c.model
	// The instructions are placed in front of the copied turns rather than
	// seeded over them, since a seed replaces the history and an empty seed
	// returns without doing anything at all. Placing them directly keeps the
	// copy whether or not the conversation carried any instructions.
	if seed := c.seedText(); seed != "" {
		e.conv.messages = append([]openrouter.Message{{
			Role:    openrouter.RoleSystem,
			Content: seed,
		}}, e.conv.messages...)
	}
	e.conv.messages = append(e.conv.messages, c.messages...)
	e.conv.setEphemeral()
	return e
}

// seedText returns the opening instructions held in the conversation.
func (c *Conversation) seedText() string {
	for _, m := range c.messages {
		if m.Role == openrouter.RoleSystem && !isCompaction(m.Content) {
			return m.Content
		}
	}
	return ""
}

// isCompaction reports whether a system turn is a summary rather than
// instructions.
func isCompaction(content string) bool {
	return strings.HasPrefix(content, compactionMarker)
}

// Conversation returns the underlying conversation of the thread.
func (e *Ephemeral) Conversation() *Conversation { return e.conv }

// Count returns the number of exchanges held by the thread.
func (e *Ephemeral) Count() int { return e.count }

// Note records that an exchange was held.
func (e *Ephemeral) Note() { e.count++ }

// Discard drops the thread.
//
// It exists so that leaving a thread is explicit at the call site. Nothing is
// written anywhere, so discarding is really only marking the point after which
// the thread is no longer reachable.
func (e *Ephemeral) Discard() {
	e.conv = nil
	e.origin = nil
}

// Origin returns the branch to return to on leaving.
func (e *Ephemeral) Origin() *Branch { return e.origin }

// ReturnToMain restores the conversation the thread branched from.
func (e *Ephemeral) ReturnToMain() {
	if e.origin != nil {
		e.conv.Restore(e.origin)
	}
}

// beginThread starts an ephemeral thread branched from the current
// conversation.
//
// A thread is not persisted. It is discarded when it is left, and nothing
// writes it anywhere, so a thread left open by a crash is lost rather than
// recovered, which is the point of it being ephemeral.
func (s *Session) beginThread() {
	if s.thread != nil {
		s.frame.Reply = append(s.frame.Reply,
			"already in a thread: /main returns before starting another")
		return
	}
	if s.conv.Model() == "" {
		s.frame.Reply = append(s.frame.Reply, "no model is selected: /model NAME")
		return
	}

	s.thread = s.conv.BeginEphemeral("btw")
	s.conv = s.thread.Conversation()
	s.updateStatus()

	s.appendLines("thread started: branched from this conversation")
	s.appendLines("It is not saved. /main returns to the conversation it came from.")
}

// endThread leaves the thread and restores the conversation.
//
// The main conversation is put back rather than a copy of it, so anything
// recorded in the thread is dropped rather than carried over.
func (s *Session) endThread() {
	if s.thread == nil {
		s.frame.Reply = append(s.frame.Reply, "not in a thread")
		return
	}

	held := s.thread.Count()
	s.thread.Discard()
	s.thread = nil
	s.conv = s.mainConv
	s.updateStatus()

	s.appendLines("thread discarded, holding " +
		pluralExchanges(held) + ". The conversation is as it was.")
}

// pluralExchanges renders a count of exchanges.
func pluralExchanges(n int) string {
	if n == 1 {
		return "1 exchange"
	}
	return fmt.Sprintf("%d exchanges", n)
}

// clearEphemeral restores recording.
//
// It is used when in-cognito mode is switched off. A thread is unrecorded for
// its own sake and is never restored by this, since an ephemeral conversation
// is ephemeral however it was entered.
func (c *Conversation) clearEphemeral() { c.ephemeral = false }
