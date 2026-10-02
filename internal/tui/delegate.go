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
	// onUsage is told what a response reported, so that its cost is added to
	// the session total. It is nil for a delegate built without a session.
	onUsage usageFunc

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
		Role:    openrouter.RoleSystem,
		Content: delegateNotice,
	})
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
		if e.Usage != nil && d.onUsage != nil {
			d.onUsage(model, e)
		}
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
	d.onUsage = s.noteSideSpend

	// The model and the client are taken here rather than inside the
	// goroutine. The conversation a delegate was branched from is replaced when
	// a thread starts or the in-cognito mode is turned on, and reading the
	// field from the goroutine would be a race against the input goroutine
	// doing that. The model in force when the question was asked is also the
	// one the answer should come from.
	model, client := s.conv.Model(), s.client

	// The delegate is tracked so that leaving does not leave it writing to a
	// frame nobody is drawing on. The counter is raised under the same lock as
	// the closing flag is read, since Close waits on it and a delegate counted
	// after that wait began would be one nobody is waiting for.
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		s.appendLines("the session is closing")
		return
	}
	if s.delegates == nil {
		s.delegates = map[*Delegate]bool{}
	}
	s.delegates[d] = true
	s.delegateWG.Add(1)
	s.mu.Unlock()

	// The label goes in the pane first, so the reader sees the question before
	// the answer rather than after.
	s.addDelegateLines("/delegate " + task)
	// The reader is told the same thing the model is, since a delegate that
	// answers short of what the question needed is otherwise a reader
	// wondering whether the model did not understand it.
	s.addDelegateLines("A delegate is given no tools, so it answers from the " +
		"conversation and from what it already knows. Ask the conversation " +
		"itself for anything it would have to go and look at.")
	s.draw()

	go func() {
		// The counter is lowered once the answer has been written to the frame
		// and drawn, so that Close returning means the goroutine is finished
		// rather than merely cancelled.
		defer s.delegateWG.Done()

		d.Run(s.ctx, client, model, func(text string) {
			// The partial answer replaces the last line rather than being
			// appended, so a growing answer does not fill the pane with
			// copies of itself.
			s.setDelegatePartial(text)
			s.draw()
		})

		answer, _, err := d.Answer()

		s.mu.Lock()
		delete(s.delegates, d)
		s.dpane.partial = ""
		// The finished answer joins the delegate pane as ordinary text. It is not
		// added to the conversation, since a delegate keeps nothing.
		switch {
		case err != nil:
			s.dpane.lines = append(s.dpane.lines,
				fmt.Sprintf("/delegate %s (error) %v", task, err))
		case answer == "":
			// An empty line would be indistinguishable from a question that
			// was never asked, so the absence is said in the same words the
			// request path uses.
			s.dpane.lines = append(s.dpane.lines, "(the model returned nothing)")
		default:
			s.dpane.addAnswer(answer)
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

// delegateNotice tells the model what a delegate is and is not given.
//
// A delegate is sent no tools, since it records nothing and a tool acts on the
// host. A model that is not told will ask for one anyway when the question calls
// for it, and a provider asked for a call it was not offered streams the
// markup as text: the reader sees a tool call written out in the reply rather
// than a reply at all.
//
// Saying so is what stops it. It is the same notice a thread gives, for the
// same reason, since both are conversations that promise nothing is recorded.
const delegateNotice = "You are answering one question alongside a conversation " +
	"that is still going on. You are not given any tools: you cannot read a " +
	"file, run a program or look at a repository. Answer from the conversation " +
	"and from what you already know. If the question needs something you cannot " +
	"reach, say what is missing rather than asking to be given a tool."
