package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// auditTurns returns a snapshot of the turns a conversation holds, so that two
// states can be compared exactly.
func auditTurns(c *Conversation) []openrouter.Message {
	return append([]openrouter.Message{}, c.messages...)
}

// auditCopy returns a snapshot of a slice of turns, for the branch readers
// below, which hold turns rather than a conversation.
func auditCopy(in []openrouter.Message) []openrouter.Message {
	return append([]openrouter.Message{}, in...)
}

// auditSameTurns reports whether two snapshots are identical.
func auditSameTurns(got, want []openrouter.Message) string {
	if len(got) != len(want) {
		return fmt.Sprintf("%d turns, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Sprintf("turn %d is %+v, want %+v", i, got[i], want[i])
		}
	}
	return ""
}

// A branch is a copy. Appending to the conversation it was taken from must not
// reach it, and the appends below grow that conversation well past the capacity
// it had when the branch was taken, which is where a shared backing array
// would show.
func TestBranchIsIndependentOfTheConversationItCameFrom(t *testing.T) {
	main := NewConversation()
	for i := 0; i < 8; i++ {
		main.Record(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}

	b := main.TakeBranch("main")
	want := auditCopy(b.conversation())

	for i := 0; i < 64; i++ {
		main.Record("later", "later reply")
	}

	if msg := auditSameTurns(auditCopy(b.conversation()), want); msg != "" {
		t.Errorf("the branch changed as the conversation it came from grew: %s", msg)
	}
}

// The same in the other direction: a conversation restored from a branch must
// not be able to grow into the branch it was restored from.
func TestRestoredConversationIsIndependentOfTheBranch(t *testing.T) {
	main := NewConversation()
	for i := 0; i < 8; i++ {
		main.Record(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}

	b := main.TakeBranch("main")
	want := auditCopy(b.conversation())

	restored := NewConversation()
	restored.Restore(b)
	for i := 0; i < 64; i++ {
		restored.Record("in the branch", "in the branch")
	}

	if msg := auditSameTurns(auditCopy(b.conversation()), want); msg != "" {
		t.Errorf("the branch changed as the conversation restored from it grew: %s", msg)
	}
}

// A compaction replaces the turns of the conversation it runs on. A branch
// taken before it must still hold what it held, and a branch taken after it
// must hold the summary rather than the turns that are gone.
func TestACompactionReachesOnlyTheConversationItRunsOn(t *testing.T) {
	main := NewConversation()
	for i := 0; i < 4; i++ {
		main.Record(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}
	before := main.TakeBranch("main")
	want := auditCopy(before.conversation())

	main.Compact("a summary of the work so far")

	if msg := auditSameTurns(auditCopy(before.conversation()), want); msg != "" {
		t.Errorf("a compaction reached a branch taken before it: %s", msg)
	}
	if !main.HasSummary() {
		t.Error("the conversation compacted no summary")
	}

	after := main.TakeBranch("main")
	if after.Turns() != 1 {
		t.Errorf("a branch taken after the compaction holds %d turns, want the summary alone",
			after.Turns())
	}
	if got := after.conversation()[0].Content; !strings.HasPrefix(got, compactionMarker) {
		t.Errorf("the turn carried is %q, want the marked summary", got)
	}
}

// conversation returns the turns a branch holds. It is a reader for the tests
// above, since Branch keeps them private.
func (b *Branch) conversation() []openrouter.Message {
	return b.turns
}

// A thread branched from a conversation carrying no instructions must keep the
// turns it branched from. Seeding over the copy would discard it, since a seed
// replaces the history and an empty seed does nothing at all, so this is the
// case where that would be visible.
func TestThreadFromAConversationWithNoInstructions(t *testing.T) {
	main := NewConversation()
	main.SetModel("test/model")
	for i := 0; i < 3; i++ {
		main.Record(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}

	thread := main.BeginEphemeral("btw")

	if got := thread.Conversation().Turns(); got != 6 {
		t.Errorf("the thread holds %d turns, want the six it branched from", got)
	}
	if seed := thread.Conversation().seedText(); seed != "" {
		t.Errorf("seedText = %q, want no instructions where none were seeded", seed)
	}
	for i, m := range auditTurns(thread.Conversation()) {
		if m.Role != openrouter.RoleUser && m.Role != openrouter.RoleAssistant {
			t.Errorf("turn %d has the role %q, want one of the copied turns", i, m.Role)
		}
	}
	if thread.Conversation().Recording() {
		t.Error("the thread records, want a thread to record nothing")
	}
}

// A thread branched from a compacted conversation must not take the summary as
// its instructions, since the summary governs a different thing. It still
// carries the summary among its turns, so the model has the context.
func TestThreadFromACompactedConversation(t *testing.T) {
	main := NewConversation()
	main.SetModel("test/model")
	main.Seed("the opening instructions")
	for i := 0; i < 3; i++ {
		main.Record(fmt.Sprintf("q%d", i), fmt.Sprintf("a%d", i))
	}
	main.Compact("a summary of the work so far")

	thread := main.BeginEphemeral("btw")
	conv := thread.Conversation()

	if seed := conv.seedText(); seed != "the opening instructions" {
		t.Errorf("seedText = %q, want the instructions rather than the summary", seed)
	}
	if conv.messages[0].Role != openrouter.RoleSystem ||
		conv.messages[0].Content != "the opening instructions" {
		t.Errorf("the thread opens with %+v, want the instructions in front", conv.messages[0])
	}
	found := false
	for _, m := range auditTurns(conv) {
		if strings.HasPrefix(m.Content, compactionMarker) {
			found = true
		}
	}
	if !found {
		t.Error("the thread does not carry the summary, so the model has no context")
	}
}

// Leaving a thread restores the conversation exactly as it was. The exchanges
// held by the thread are dropped, and nothing it recorded reaches the main
// conversation.
func TestLeavingAThreadRestoresTheConversationExactly(t *testing.T) {
	s, _ := auditSession(t, auditStream)
	s.conv.Record("q1", "a1")
	s.conv.Record("q2", "a2")
	want := auditTurns(s.conv)

	s.beginThread()
	if s.thread == nil {
		t.Fatal("beginThread did not start a thread")
	}
	s.send("a question asked in the thread")
	s.send("another one")

	if s.conv == s.mainConv {
		t.Fatal("the thread did not take over the conversation")
	}
	if got := s.thread.Count(); got != 2 {
		t.Errorf("the thread counted %d exchanges, want 2", got)
	}
	if s.conv.Turns() != 4 {
		t.Errorf("the thread holds %d turns, want the four it branched from", s.conv.Turns())
	}

	s.endThread()

	if s.conv != s.mainConv {
		t.Error("the main conversation was not put back")
	}
	if msg := auditSameTurns(auditTurns(s.conv), want); msg != "" {
		t.Errorf("the conversation came back changed: %s", msg)
	}
	if s.thread != nil {
		t.Error("the thread survived being left")
	}
}

// The main conversation keeps its own exchanges while a thread is open, and the
// thread is unaffected by them, since a branch is a copy.
func TestExchangesInAThreadDoNotReachTheMainConversation(t *testing.T) {
	s, _ := auditSession(t, auditStream)
	s.conv.Record("q1", "a1")

	s.beginThread()
	threadTurns := auditTurns(s.conv)

	s.endThread()
	for i := 0; i < 20; i++ {
		s.send(fmt.Sprintf("question %d", i))
	}

	if got := s.conv.Turns(); got != 42 {
		t.Errorf("the main conversation holds %d turns, want 42", got)
	}
	if msg := auditSameTurns(threadTurns, auditTurns(s.conv)[:len(threadTurns)]); msg != "" {
		t.Errorf("the turns the thread branched from were disturbed: %s", msg)
	}
}

// A delegate is taken from a copy, so the conversation carries on underneath it
// and the answer is written to the pane rather than to either conversation.
func TestDelegateDoesNotDisturbTheConversation(t *testing.T) {
	reached := make(chan struct{}, 1)
	release := make(chan struct{})
	s := blockingSession(t, auditStream, reached, release)
	s.conv.Record("q1", "a1")
	want := auditTurns(s.conv)

	s.startDelegate("what next")
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("the delegate request never reached the server")
	}

	for i := 0; i < 20; i++ {
		s.send(fmt.Sprintf("question %d", i))
	}
	close(release)

	deadline := time.Now().Add(10 * time.Second)
	for s.delegatePending() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := s.delegatePending(); got != 0 {
		t.Fatalf("%d delegates still running after the release", got)
	}

	// The exchanges the reader made are the only turns the conversation holds
	// beyond the one it started with.
	if got := s.conv.Turns(); got != 42 {
		t.Errorf("the conversation holds %d turns, want 42", got)
	}
	if msg := auditSameTurns(auditTurns(s.conv)[:len(want)], want); msg != "" {
		t.Errorf("the turns the delegate branched from were disturbed: %s", msg)
	}
	if s.conv.HasSummary() {
		t.Error("the delegate left a summary behind")
	}
}

// Every path that records an exchange goes through Record, which is where
// retention is decided. An in-cognito conversation, a thread and a delegate all
// reach it and all of them record nothing.
func TestRetentionIsDecidedOnTheConversation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		eph   func(*Conversation)
		turns int
	}{
		{"recording", nil, 4},
		{"in cognito", func(c *Conversation) { c.setEphemeral() }, 0},
	} {
		c := NewConversation()
		if tc.eph != nil {
			tc.eph(c)
		}
		c.Record("q1", "a1")
		c.Record("q2", "a2")
		if got := c.Turns(); got != tc.turns {
			t.Errorf("%s: turns = %d, want %d", tc.name, got, tc.turns)
		}
	}
}
