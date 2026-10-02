package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// Spawn is a delegate that is given tools.
//
// It is the same shape as a Delegate in every respect but one: the request
// carries a tool catalogue, and a call is run and answered rather than refused.
// The distinction is worth its own type rather than a field on Delegate because
// the two answers to a turn differ in ways a flag would hide. A delegate
// records nothing and is told so, and a model offered a tool it cannot have
// will stream the markup as text instead. A worker is told what it may do, and
// what it did is written down twice: once to the pane, so a reader can see it
// arrive, and once to a file, so it can be read after the fact.
//
// The worker does not record into the conversation. It records into its log,
// which is the guarantee a delegate gives made concrete rather than relaxed:
// nothing a worker did is carried into the next request as though the model had
// been told about it, and nothing it did is lost.
type Spawn struct {
	// task is the question the worker is answering.
	task string
	// conv is the copied conversation the request is made from.
	conv *Conversation
	// specs are the tools the worker is offered.
	specs []openrouter.Tool
	// tools is what a call is run by.
	tools *toolSet
	// log is the record the worker leaves behind.
	log *workerLog
	// answer accumulates the reply as it arrives.
	answer strings.Builder
	// done reports that the request has finished.
	done bool
	// err carries the failure that ended it, if any.
	err error

	mu sync.Mutex
}

// NewSpawn returns a worker answering task from a copy of the conversation,
// offered the given tools.
func NewSpawn(conv *Conversation, task string, set *toolSet, specs []openrouter.Tool) *Spawn {
	return &Spawn{
		task:  task,
		conv:  conv.TakeBranch("spawn").Restore2(),
		specs: specs,
		tools: set,
	}
}

// spawnProgress is what a worker reports as it works.
//
// It is passed rather than reached for because the worker is a conversation of
// its own and the session is what draws. A worker in a test carries none of
// these and simply runs.
type spawnProgress struct {
	// onCall is called once per tool call, as the result comes back.
	onCall func(tools.Result)
	// onAnswer is called as the reply arrives, with the whole of it so far.
	onAnswer func(text string)
}

// Run asks the question and runs whatever tools come back.
//
// A worker is a loop of rounds for the same reason a turn is: a call produces
// a result, the result goes back to the model, and the model may call again.
// The cap is the same one, since it is the same loop and the same risk of a
// model asking for the same file for ever.
//
// Unlike a turn, a worker does not compact and does not take the session turn
// slot, since it is not the turn in flight and the reader is free to keep
// typing. What it does take is the same approval path, so a worker asking to
// run a program puts the same question on the input block that a turn does.
func (w *Spawn) Run(ctx context.Context, c *openrouter.Client, model string,
	p spawnProgress) {

	w.mu.Lock()
	w.conv.messages = append(w.conv.messages, openrouter.Message{
		Role:    openrouter.RoleSystem,
		Content: spawnNotice,
	})
	w.conv.messages = append(w.conv.messages, openrouter.Message{
		Role:    openrouter.RoleUser,
		Content: w.task,
	})
	pending := append([]openrouter.Message{}, w.conv.messages...)
	w.mu.Unlock()

	for round := 0; round < maxToolRounds; round++ {
		var reply strings.Builder
		var calls []openrouter.ToolCall
		stopped := false
		failed := false

		err := c.Chat(ctx, openrouter.ChatRequest{
			Model:    model,
			Messages: pending,
			Tools:    w.specs,
		}, func(e openrouter.StreamEvent) {
			if e.Err != nil {
				w.mu.Lock()
				if ctx.Err() != nil {
					stopped = true
				} else {
					failed = true
					w.err = e.Err
				}
				w.mu.Unlock()
				return
			}
			if e.Done {
				return
			}
			if len(e.ToolCalls) > 0 {
				// The transport gathers the fragments of a call and delivers
				// the whole set once, so there is nothing to reassemble here.
				calls = e.ToolCalls
				return
			}
			reply.WriteString(e.Content)
			if p.onAnswer != nil {
				p.onAnswer(reply.String())
			}
		})

		// Every exit from a round ends the worker, and the text that had
		// arrived is kept: it is usually worth reading, and a worker stopped
		// short is a reader answering a question rather than a fault.
		switch {
		case stopped || ctx.Err() != nil:
			if reply.Len() > 0 {
				w.appendAnswer(strings.TrimRight(reply.String(), "\n"))
			}
			w.appendAnswer("(stopped)")
			w.finish()
			return
		case err != nil:
			w.fail(err)
			w.finish()
			return
		case failed:
			w.finish()
			return
		}

		pending = append(pending, openrouter.Message{
			Role:      openrouter.RoleAssistant,
			Content:   reply.String(),
			ToolCalls: calls,
		})

		if len(calls) == 0 {
			text := reply.String()
			if text == "" {
				w.fail(errors.New("the model returned nothing"))
				w.finish()
				return
			}
			w.appendAnswer(strings.TrimRight(text, "\n"))
			w.finish()
			return
		}

		// The text a worker says around a call is kept, since a worker that
		// narrates before it reads is saying something the reader wants.
		if reply.Len() > 0 {
			w.appendAnswer(strings.TrimRight(reply.String(), "\n"))
		}

		for _, call := range calls {
			result := w.run(call)
			if p.onCall != nil {
				p.onCall(result)
			}
			w.mu.Lock()
			w.log.call(result)
			w.mu.Unlock()
			// A call that failed still produces an answer. A request carrying
			// no answer to a call is refused by most providers, and a worker
			// producing nothing at all is one that stalls.
			pending = append(pending, openrouter.Message{
				Role:       openrouter.RoleTool,
				ToolCallID: call.ID,
				Content:    toolContent(result),
			})
		}

		if round == maxToolRounds-1 {
			w.appendAnswer(fmt.Sprintf("(stopped: the worker reached its limit "+
				"of %d requests)", maxToolRounds))
			w.finish()
			return
		}
	}

	w.finish()
}

// run answers one call, refusing rather than stalling where there is no set.
func (w *Spawn) run(call openrouter.ToolCall) tools.Result {
	if w.tools == nil {
		return tools.Result{Call: call, Err: errors.New("this worker was given no tools")}
	}
	return w.tools.run(call)
}

// appendAnswer adds a line to what the worker is showing.
func (w *Spawn) appendAnswer(text string) {
	w.mu.Lock()
	w.answer.WriteString(text + "\n")
	w.mu.Unlock()
}

// fail records the failure that ended the worker, keeping the first.
func (w *Spawn) fail(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
}

// finish marks the worker done and closes its log.
//
// The answer is written to the log only where the worker ended cleanly. A log
// holding an answer under an error is a log read as though the work had
// succeeded, which is the one misreading the record exists to prevent.
func (w *Spawn) finish() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.done = true
	if w.err == nil {
		w.log.answer(w.answer.String())
	}
	w.log.close()
}

// Answer returns the reply and whether the worker has finished.
func (w *Spawn) Answer() (string, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimRight(w.answer.String(), "\n"), w.done, w.err
}

// Task returns the question being answered, for the pane to label it with.
func (w *Spawn) Task() string { return w.task }

// LogPath returns where the worker log is, for the pane to name.
func (w *Spawn) LogPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.log.path
}

// LogProblem returns why the log could not be written in full, or nil.
func (w *Spawn) LogProblem() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.log.err
}

// spawnNotice tells the worker what it is and what it may reach.
//
// The wording is the delegate notice with the other half of it: a worker is
// given tools rather than refused them, so what it needs to be told is what
// those tools do on the host, and that a call may be put to the reader.
const spawnNotice = "You are answering one question in a background worker, " +
	"alongside a conversation that is still going on. You are given tools: you " +
	"may read, write and list files under the working directory, run git " +
	"commands there, and commit, push, create worktrees and merge as you are " +
	"asked. A program run is put to the reader first, so a call may be " +
	"refused; a refusal is the reader's answer and is not a fault to work " +
	"around. Report what you did and what came of it."

// cmdSpawn starts a worker that is given the tools.
//
// It is refused while cognito is on, since that mode promises nothing is
// recorded and a worker acts on the host. A thread is not refused: a thread is
// a conversation rather than a mode, and a worker started from one still keeps
// its own log where a reader can find it.
func (s *Session) cmdSpawn(args []string) bool {
	if s.cognito {
		s.addReply("(/spawn is refused while cognito is on, since a worker acts on " +
			"the host and cognito promises nothing is recorded; turn it off with " +
			"/cognito first)")
		return false
	}
	s.startSpawn(strings.Join(args, " "))
	return false
}

// startSpawn runs a worker in the background and returns without waiting.
//
// It is startDelegate with the tool set attached, and the same rules about
// counting apply: a worker is registered under the session lock and counted on
// the same wait group, so leaving does not leave one writing to a frame nobody
// is drawing on.
func (s *Session) startSpawn(task string) {
	if s.client == nil {
		s.appendLines("no API key is configured")
		return
	}
	if s.conv.Model() == "" {
		s.appendLines("no model is selected: /model NAME")
		return
	}
	if strings.TrimSpace(task) == "" {
		s.addReply("/spawn needs a question to answer")
		return
	}

	specs := s.tools.specs()
	if len(specs) == 0 {
		s.addReply("a worker is given no tools: " + s.tools.absence())
		return
	}

	// The log is opened before the goroutine starts, so a directory that
	// cannot be made is reported before the reader is told a worker has begun.
	// A worker that cannot leave a record is not a worker worth starting.
	started := time.Now()
	log, err := newWorkerLog(started)
	if err != nil {
		s.addReply("(a worker log could not be opened: " + err.Error() + ")")
		return
	}
	model := s.conv.Model()
	log.header(task, model)

	w := NewSpawn(s.conv, task, s.tools, specs)
	w.log = log

	// The client is taken here rather than inside the goroutine, for the same
	// reason a delegate takes it there.
	client := s.client

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		log.close()
		s.appendLines("the session is closing")
		return
	}
	if s.workers == nil {
		s.workers = map[*Spawn]bool{}
	}
	s.workers[w] = true
	s.workerWG.Add(1)
	s.mu.Unlock()

	s.addWorkerLines("/spawn " + task)
	s.addWorkerLines("A worker is given the tools, and every program it runs is " +
		"put to you first. What it does is logged to " + log.path + ".")
	s.draw()

	go func() {
		// The counter is lowered once the answer has been written to the frame
		// and drawn, so that Close returning means the goroutine is finished
		// rather than merely cancelled.
		defer s.workerWG.Done()

		w.Run(s.ctx, client, model, spawnProgress{
			onCall: func(r tools.Result) {
				s.drawWorkerCall(r)
				s.draw()
			},
			onAnswer: func(text string) {
				s.setWorkerPartial(text)
				s.draw()
			},
		})

		answer, _, err := w.Answer()

		s.mu.Lock()
		delete(s.workers, w)
		s.wpane.partial = ""
		switch {
		case err != nil:
			s.wpane.lines = append(s.wpane.lines,
				fmt.Sprintf("/spawn %s (error) %v", task, err))
		case answer == "":
			s.wpane.lines = append(s.wpane.lines, "(the model returned nothing)")
		default:
			s.wpane.lines = append(s.wpane.lines, answer)
		}
		if path := w.LogPath(); path != "" {
			s.wpane.lines = append(s.wpane.lines, "logged to "+path)
		}
		s.mu.Unlock()
		s.draw()
	}()
}

// workerPending reports how many workers are still running.
func (s *Session) workerPending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.workers)
}
