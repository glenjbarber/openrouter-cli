package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Delegate runs a question in a separate conversation while the main one stays
// open.
//
// A delegate is not a thread and not a subagent in the sense of a worker that
// is spawned and forgotten. It is a second request made from a copy of the
// conversation, whose answer is shown in the pane and kept in neither
// conversation. The main conversation is untouched throughout, and the prompt
// stays live while the delegate works, so a user can keep typing and queue the
// next message rather than waiting.
//
// The distinction matters for retention. A delegate writes its text to the pane
// and to nothing else. There is no file, no history, and no record kept once it
// finishes, so a delegate does not weaken the guarantee that /cognito makes.
type Delegate struct {
	// task is the question the delegate is answering.
	task string
	// conv is the copied conversation the request is made from.
	conv *Conversation
	// answer accumulates the reply as it arrives.
	answer strings.Builder
	// done reports that the request has finished.
	done bool
	// err carries the failure that ended it, if any.
	err error

	mu sync.Mutex
}

// NewDelegate returns a delegate answering task from a copy of the
// conversation.
func NewDelegate(conv *Conversation, task string) *Delegate {
	d := &Delegate{task: task, conv: conv.TakeBranch("delegate").Restore2()}
	return d
}

// Restore2 returns a conversation holding the branch it took, for a delegate to
// send from.
//
// A delegate needs a conversation it may send from and must not record into,
// which is what an ephemeral conversation is. It is taken from the main
// conversation so that the model has the context the user was looking at.
func (b *Branch) Restore2() *Conversation {
	c := NewConversation()
	c.Restore(b)
	c.setEphemeral()
	return c
}

// Run asks the question and reports progress to the pane.
//
// The callback is called as the answer arrives, so that a slow delegate does
// not leave the interface apparently idle. The pane is written to rather than
// the conversation, since a delegate keeps nothing.
func (d *Delegate) Run(ctx context.Context, c *openrouter.Client, model string,
	onProgress func(text string)) {

	d.mu.Lock()
	d.conv.messages = append(d.conv.messages, openrouter.Message{
		Role:    openrouter.RoleUser,
		Content: d.task,
	})
	history := append([]openrouter.Message{}, d.conv.messages...)
	d.mu.Unlock()

	err := c.Chat(ctx, openrouter.ChatRequest{
		Model:    model,
		Messages: history,
	}, func(e openrouter.StreamEvent) {
		if e.Err != nil {
			d.mu.Lock()
			d.err = e.Err
			d.mu.Unlock()
			return
		}
		if e.Done {
			return
		}
		d.mu.Lock()
		d.answer.WriteString(e.Content)
		text := d.answer.String()
		d.mu.Unlock()
		if onProgress != nil {
			onProgress(text)
		}
	})

	d.mu.Lock()
	d.done = true
	if d.err == nil {
		d.err = err
	}
	d.mu.Unlock()
}

// Answer returns the reply and whether the delegate has finished.
func (d *Delegate) Answer() (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return strings.TrimRight(d.answer.String(), "\n"), d.done, d.err
}

// Task returns the question being answered, for the pane to label it with.
func (d *Delegate) Task() string { return d.task }

// startDelegate runs a question in the background and returns without waiting.
func (s *Session) startDelegate(task string) {
	if s.client == nil {
		s.appendLines("no API key is configured")
		return
	}
	if s.conv.Model() == "" {
		s.appendLines("no model is selected: /model NAME")
		return
	}

	d := NewDelegate(s.conv, task)

	// The delegate is tracked so that leaving does not leave it writing to a
	// frame nobody is drawing on.
	s.mu.Lock()
	if s.delegates == nil {
		s.delegates = map[*Delegate]bool{}
	}
	s.delegates[d] = true
	s.mu.Unlock()

	// The label goes in the pane first, so the reader sees the question before
	// the answer rather than after.
	s.appendLines("/delegate " + task)
	s.draw()

	go func() {
		d.Run(s.ctx, s.client, s.conv.Model(), func(text string) {
			s.mu.Lock()
			// The partial answer replaces the last line rather than being
			// appended, so a growing answer does not fill the pane with
			// copies of itself.
			s.frame.Delegate = text
			s.mu.Unlock()
			s.draw()
		})

		answer, _, err := d.Answer()

		s.mu.Lock()
		delete(s.delegates, d)
		s.frame.Delegate = ""
		// The finished answer joins the pane as ordinary text. It is not
		// added to the conversation, since a delegate keeps nothing.
		if err != nil {
			s.frame.Reply = append(s.frame.Reply,
				fmt.Sprintf("/delegate %s (error) %v", task, err))
		} else {
			s.frame.Reply = append(s.frame.Reply, answer)
		}
		s.mu.Unlock()
		s.draw()
	}()
}

// delegatePending reports how many delegates are still running.
func (s *Session) delegatePending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.delegates)
}
