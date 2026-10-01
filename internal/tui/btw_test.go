package tui

import (
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// A branch must be a copy. Sharing the slice would let an exchange in the
// thread appear in the main conversation, which defeats branching.
func TestTakeBranchCopies(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")

	b := c.TakeBranch("main")

	c.Record("q2", "a2")
	if b.Turns() != 2 {
		t.Errorf("Turns = %d, want 2, so the branch followed the main conversation", b.Turns())
	}
}

// Restoring a branch must put back exactly what was copied, discarding
// anything recorded in the branch since. Restoring into the same conversation
// and then recording again must not accumulate duplicated turns.
func TestRestoreDiscardsLaterTurns(t *testing.T) {
	c := NewConversation()
	c.Record("q1", "a1")
	b := c.TakeBranch("main")

	// Work done in the branch is not part of the snapshot.
	c.Restore(b)
	c.Record("branch work", "branch answer")

	// Returning to the snapshot drops that work.
	c.Restore(b)
	msgs := c.Pending("next")
	// Two recorded turns plus the pending one.
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3", len(msgs))
	}
	for _, m := range msgs {
		if m.Content == "branch work" {
			t.Error("the branch work survived the restore")
		}
	}
}

// A thread begins with the history it branched from, so the model has the
// context the user was looking at.
func TestThreadSeesBranchHistory(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")
	c.Record("q2", "a2")

	e := c.BeginEphemeral("btw")
	if e.Conversation().Turns() < 4 {
		t.Errorf("turns = %d, want the copied history", e.Conversation().Turns())
	}
	if e.Conversation().Model() != "test/model" {
		t.Errorf("model = %q, want it carried over", e.Conversation().Model())
	}
}

// The opening instructions must survive into the thread, since losing them
// changes how the model behaves without anything saying so.
func TestThreadKeepsInstructions(t *testing.T) {
	c := NewConversation()
	c.Seed("answer in the third person")
	c.SetModel("test/model")
	c.Record("q1", "a1")

	e := c.BeginEphemeral("btw")
	msgs := e.Conversation().Pending("next")
	if len(msgs) == 0 || msgs[0].Role != openrouter.RoleSystem {
		t.Fatalf("msgs = %+v, want the instructions first", msgs)
	}
	if got := msgs[0].Content; got != "answer in the third person" {
		t.Errorf("instructions = %q, want them preserved", got)
	}
}

// An exchange in the thread must not reach the main conversation.
func TestThreadDoesNotLeakBack(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")

	e := c.BeginEphemeral("btw")
	e.Conversation().Record("thread question", "thread answer")

	if got := len(c.Pending("next")); got != 3 {
		t.Errorf("main conversation has %d turns, want 3 with no thread exchange", got)
	}
}

// A discarded thread is dropped rather than written anywhere.
func TestDiscardDropsThread(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	e := c.BeginEphemeral("btw")
	e.Conversation().Record("q", "a")

	e.Discard()
	if e.Conversation() != nil {
		t.Error("Conversation survived Discard, want it dropped")
	}
	if e.Count() != 0 {
		t.Errorf("Count = %d after Discard, want 0", e.Count())
	}
}

func TestCountTracksExchanges(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	e := c.BeginEphemeral("btw")

	if e.Count() != 0 {
		t.Errorf("Count = %d, want 0 on a fresh thread", e.Count())
	}
	e.Note()
	if e.Count() != 1 {
		t.Errorf("Count = %d, want 1", e.Count())
	}
}

func TestPluralExchanges(t *testing.T) {
	if got := pluralExchanges(1); got != "1 exchange" {
		t.Errorf("got %q, want the singular", got)
	}
	if got := pluralExchanges(3); got != "3 exchanges" {
		t.Errorf("got %q, want the plural", got)
	}
}

// A compacted history must not have its summary treated as instructions when a
// thread is branched, since the two are both system turns.
func TestThreadSeedIgnoresCompaction(t *testing.T) {
	c := NewConversation()
	c.Record("q1", "a1")
	c.Compact("a summary")

	e := c.BeginEphemeral("btw")
	if got := e.Conversation().seedText(); got != "" {
		t.Errorf("seedText = %q, want the summary not taken as instructions", got)
	}
}
