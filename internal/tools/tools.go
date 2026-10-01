// Package tools runs the tools a model asks for.
//
// The transport carries a call and does not run it. The wire types live in
// internal/openrouter and the execution lives here, so the dependency between
// the two runs one way and a tool cannot reach the interface that drew it.
//
// The package carries one guarantee that shapes everything in it: a call always
// produces a result. An unknown name, arguments that are not a JSON object, a
// body that returned an error and a body that panicked are all reported as a
// result carrying that error, because a call producing no result at all leaves
// the turn waiting for something that never arrives, and a turn that waits is
// reported by a reader as a hang rather than as a fault.
package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Result is what one tool call produced.
//
// Text and Err are not alternatives. A body that failed partway returns what
// it managed to produce alongside the failure, since a partial answer is more
// useful to a model than an error on its own.
type Result struct {
	Call openrouter.ToolCall
	Text string
	Err  error
}

// toolFn is the body of one tool. It is handed the arguments as they arrived
// on the wire and returns the text that goes back to the model.
type toolFn func(json.RawMessage) (string, error)

// entry is one tool as the set holds it: what a model is told about it, and
// what runs it.
type entry struct {
	spec openrouter.Tool
	fn   toolFn
}

// Set is the tools offered to a session and the registry that runs them.
//
// The order the tools were added in is kept, so that what a model is offered is
// the same on every run. A map iterated directly would order them by hash, and
// a catalogue offered in a different order each time reads as a different
// catalogue.
type Set struct {
	mu     sync.RWMutex
	order  []string
	byName map[string]entry
}

// New returns a set holding no tools. A session offering none is ordinary: the
// request carries none and the model is still asked questions.
func New() *Set {
	return &Set{byName: make(map[string]entry)}
}

// Add registers a tool under the name in its spec.
//
// A tool with no name, or with no body, is not registered: a call the set
// cannot run is reported as an error when it arrives rather than being
// answered by something that is not there. A name already registered is
// replaced, since a second registration of the same name is one tool written
// twice rather than two tools.
func (s *Set) Add(t openrouter.Tool, fn func(json.RawMessage) (string, error)) {
	name := t.Function.Name
	if name == "" || fn == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, held := s.byName[name]; !held {
		s.order = append(s.order, name)
	}
	s.byName[name] = entry{spec: t, fn: fn}
}

// Specs returns what a request offers, in the order the tools were added.
//
// The slice is built on each call rather than kept, so that a caller cannot
// change what the next request offers by editing what it was handed.
func (s *Set) Specs() []openrouter.Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	specs := make([]openrouter.Tool, 0, len(s.order))
	for _, name := range s.order {
		specs = append(specs, s.byName[name].spec)
	}
	return specs
}

// Names returns the names of the tools offered, in the order Specs reports
// them. A caller matches a reply against a name, and a name in a different
// order from the one the request offered would be a name looked up wrongly.
func (s *Set) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, len(s.order))
	copy(names, s.order)
	return names
}

// Run executes the call a model asked for.
//
// The call is echoed into the result whatever the outcome, since the reply
// going back to the model names the call it answers and a result without one
// cannot be matched to a call.
func (s *Set) Run(c openrouter.ToolCall) Result {
	name := c.Function.Name
	if name == "" {
		return Result{Call: c, Err: errors.New("the call named no tool")}
	}
	fn, ok := s.lookup(name)
	if !ok {
		return Result{Call: c, Err: s.refusal(name)}
	}
	text, err := invoke(fn, c.Function.Arguments)
	return Result{Call: c, Text: text, Err: err}
}

// refusal reports a call naming a tool that was not offered, with what was.
//
// The offered names are in the message because the model chooses its next call
// from it, and a refusal naming nothing but the mistake leaves it guessing.
func (s *Set) refusal(name string) error {
	names := s.Names()
	if len(names) == 0 {
		return fmt.Errorf("no tool is named %q, and this session offers no tools", name)
	}
	return fmt.Errorf("no tool is named %q; the tools offered are %s",
		name, strings.Join(names, ", "))
}

// lookup returns the body registered under a name.
//
// The lock is released before the body runs, since a body calling back into the
// set would otherwise deadlock against it, and a body does not unregister
// itself.
func (s *Set) lookup(name string) (toolFn, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.byName[name]
	if !ok {
		return nil, false
	}
	return e.fn, true
}

// invoke runs a body and turns a panic into an error.
//
// The recovery is here rather than left to the caller so that a body which
// panics is reported the same way as one which returned an error. A tool is
// code this package did not write in every case, and a reader is owed a result
// either way.
func invoke(fn toolFn, args string) (text string, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		text = ""
		if v, ok := r.(error); ok {
			err = fmt.Errorf("the tool panicked: %w", v)
			return
		}
		err = fmt.Errorf("the tool panicked: %v", r)
	}()
	return fn(json.RawMessage(args))
}

// decode reads the arguments of a call.
//
// They arrive as a JSON document that nothing upstream has parsed, since the
// transport carries what a model sent rather than a value. A document that is
// not an object is refused here rather than by each body, so that a model which
// sent a string where an object belongs is told so in the same words every
// time.
func decode(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("the call carried no arguments")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("the arguments are not a JSON object: %w", err)
	}
	return nil
}

// missingArg reports an argument the call left out, by the name the schema
// gives it.
//
// The schema name is used rather than a Go field name, since the model is the
// one reading the message and the name it has to add is the one it was offered.
func missingArg(name string) error {
	return fmt.Errorf("the %q argument is required and was not given", name)
}
