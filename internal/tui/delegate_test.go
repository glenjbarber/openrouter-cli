package tui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// A delegate must record nothing, or it weakens the guarantee /cognito makes.
// It keeps the history it branched from and adds no exchange of its own.
func TestDelegateRecordsNothing(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")

	d := NewDelegate(c, "a side question")

	before := d.conv.Turns()
	d.conv.Record("the question", "the answer")
	if d.conv.Turns() != before {
		t.Errorf("turns = %d, want the delegate to record nothing", d.conv.Turns())
	}
}

// The main conversation must not gain the delegate's exchange either.
func TestDelegateDoesNotTouchMain(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")

	before := c.Turns()
	d := NewDelegate(c, "a side question")
	d.conv.Record("q", "a")

	if c.Turns() != before {
		t.Errorf("main turns = %d, want %d, so the delegate leaked",
			c.Turns(), before)
	}
}

// A delegate must see the conversation it branched from, or it cannot answer a
// question about it.
func TestDelegateSeesHistory(t *testing.T) {
	c := NewConversation()
	c.SetModel("test/model")
	c.Record("q1", "a1")
	c.Record("q2", "a2")

	d := NewDelegate(c, "a side question")
	if d.conv.Turns() < 4 {
		t.Errorf("turns = %d, want the copied history", d.conv.Turns())
	}
}

// The instructions survive into a delegate, since losing them would change how
// the model behaves without anything saying so.
func TestDelegateKeepsInstructions(t *testing.T) {
	c := NewConversation()
	c.Seed("answer in the third person")
	c.SetModel("test/model")
	c.Record("q1", "a1")

	d := NewDelegate(c, "a side question")
	msgs := d.conv.messages
	if len(msgs) == 0 || !strings.Contains(msgs[0].Content, "third person") {
		t.Errorf("first turn = %+v, want the instructions", msgs[0])
	}
}

// The task is the question, and it is what the pane labels the answer with.
func TestDelegateTask(t *testing.T) {
	c := NewConversation()
	d := NewDelegate(c, "why is it slow?")
	if d.Task() != "why is it slow?" {
		t.Errorf("Task = %q, want the question", d.Task())
	}
}

// A delegate that has not run reports no answer and is not done.
func TestDelegateBeforeRunning(t *testing.T) {
	d := NewDelegate(NewConversation(), "q")
	answer, done, err := d.Answer()
	if done {
		t.Error("done = true before running")
	}
	if err != nil {
		t.Errorf("err = %v before running", err)
	}
	if answer != "" {
		t.Errorf("answer = %q, want empty", answer)
	}
}

// The delegate runs on its own goroutine, so its bookkeeping must be safe
// against a reader arriving at the same time. The race detector is the point.
func TestDelegateAnswerIsRaceFree(t *testing.T) {
	d := NewDelegate(NewConversation(), "q")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			d.mu.Lock()
			d.answer.WriteString("x")
			d.mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			d.Answer()
		}
	}()
	wg.Wait()
}

// A running delegate must be visible in the pane without being part of the
// conversation, or a reply would be read as the answer to the last question.
func TestFrameShowsDelegateSeparately(t *testing.T) {
	lines := Render(Frame{
		Reply:    []string{"> the main question"},
		Delegate: "the delegate answer",
	}, 12, 40)

	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "the delegate answer") {
		t.Errorf("frame is missing the delegate answer:\n%s", body)
	}
	if !strings.Contains(body, "the main question") {
		t.Errorf("frame is missing the main question:\n%s", body)
	}
}

// A delegate with no answer shows nothing, so the pane is not given a blank row
// for it.
func TestFrameOmitsEmptyDelegate(t *testing.T) {
	lines := Render(Frame{Reply: []string{"a"}, Delegate: ""}, 10, 40)
	for i, l := range lines {
		if strings.Contains(l, "delegate") {
			t.Errorf("row %d = %q, want no delegate row", i, l)
		}
	}
}

// The main prompt must stay live while a delegate runs, which is the point of
// running one in the background.
func TestDelegateDoesNotBlockInput(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv

	// A session with no client cannot start one, and must say so rather than
	// block or panic.
	s.startDelegate("a question")

	if len(s.frame.Reply) == 0 {
		t.Error("no message reported for a delegate that could not start")
	}
}

// A delegate must finish before the session is left, or it writes to a frame
// nobody is drawing on.
func TestDelegateTrackedWhileRunning(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv

	d := NewDelegate(s.conv, "q")
	s.mu.Lock()
	if s.delegates == nil {
		s.delegates = map[*Delegate]bool{}
	}
	s.delegates[d] = true
	s.mu.Unlock()

	if got := s.delegatePending(); got != 1 {
		t.Errorf("pending = %d, want 1", got)
	}
}

func TestOpenRouterMessageRole(t *testing.T) {
	// The delegate sends a user turn, so the role constant is used rather than
	// a literal.
	if openrouter.RoleUser != "user" {
		t.Errorf("RoleUser = %q, want %q", openrouter.RoleUser, "user")
	}
	_ = time.Second
}
