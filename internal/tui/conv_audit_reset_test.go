package tui

import (
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Clearing a conversation must not carry a description of the work it held.
// A summary is a system turn as the instructions are, so the turn that survives
// a clearing has to be told apart from one that is not the instructions.
func TestResetDropsASummary(t *testing.T) {
	c := NewConversation()
	c.Record("q1", "a1")
	c.Record("q2", "a2")
	c.Compact("a summary of the work so far")
	if !c.HasSummary() {
		t.Fatal("the compaction left no summary to be dropped")
	}

	c.Reset()

	if c.HasSummary() {
		t.Error("the summary survived the clearing of the conversation")
	}
	if c.Turns() != 0 {
		t.Errorf("turns = %d, want the conversation empty", c.Turns())
	}
}

// The instructions survive a clearing whether or not a summary sits behind
// them, since losing them would change how the model behaves with nothing said.
func TestResetKeepsTheInstructionsAfterACompaction(t *testing.T) {
	c := NewConversation()
	c.Seed("the opening instructions")
	c.Record("q1", "a1")
	c.Record("q2", "a2")
	c.Compact("a summary of the work so far")

	c.Reset()

	if c.Turns() != 1 {
		t.Fatalf("turns = %d, want only the instructions to survive", c.Turns())
	}
	if got := c.messages[0].Content; got != "the opening instructions" {
		t.Errorf("surviving turn = %q, want the opening instructions", got)
	}
	if c.messages[0].Role != openrouter.RoleSystem {
		t.Errorf("surviving turn has the role %q, want the system role", c.messages[0].Role)
	}
}
