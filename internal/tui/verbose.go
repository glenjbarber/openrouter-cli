package tui

import (
	"fmt"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// streamReport is the detail of one streamed turn, gathered while the events
// arrive and rendered once the turn has finished.
//
// A line per event would bury the reply under as many rows as there were
// deltas, so the events are counted and the shape is summarised. What a reader
// debugging a model or an endpoint wants is how many deltas it took, whether a
// finish reason and any accounting arrived, and whether anything failed, not
// the text of each increment.
type streamReport struct {
	deltas  int
	chars   int
	finish  string
	usage   bool
	in, out int
	// failed records that an error ended the turn. The error text itself is
	// already shown in the pane, so only its presence is carried here.
	failed bool
	done   bool
}

// note records one stream event.
//
// It is called from the request goroutine for every event, so it holds no
// lock of its own: the caller owns the report for the duration of the turn,
// which is a single goroutine.
func (r *streamReport) note(e openrouter.StreamEvent) {
	switch e.Kind {
	case openrouter.EventDelta:
		r.deltas++
		r.chars += len(e.Content)
	case openrouter.EventUsage:
		r.usage = true
		if e.Usage != nil {
			r.in = e.Usage.PromptTokens
			r.out = e.Usage.CompletionTokens
		}
	case openrouter.EventEnd:
		r.finish = e.Finish
	case openrouter.EventError:
		if e.Err != nil {
			r.failed = true
		}
	case openrouter.EventDone:
		r.done = true
	}
}

// summary renders the report as a single line.
//
// The order follows the stream itself, so the line reads in the order the
// events arrived rather than in the order the fields happen to be declared.
func (r *streamReport) summary() string {
	parts := []string{plural(r.deltas, "delta"), plural(r.chars, "char")}
	if r.finish != "" {
		parts = append(parts, "finish "+r.finish)
	}
	if r.usage {
		parts = append(parts, fmt.Sprintf("usage %d in / %d out", r.in, r.out))
	} else {
		parts = append(parts, "no usage")
	}
	if r.failed {
		parts = append(parts, "failed")
	}
	if !r.done {
		// The terminating marker is what tells a complete turn from one that
		// was cut short, so its absence is reported rather than left to be
		// inferred from a reply that simply stops.
		parts = append(parts, "unterminated")
	}
	return strings.Join(parts, ", ")
}

// plural counts things in the form a reader expects, since a summary reading
// "1 deltas" reads as a mistake in the summary rather than as a count.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// verboseOn reports whether the stream detail is wanted for this turn.
//
// The mode is a field on the session rather than a parameter threaded through
// every caller, in the manner of the bell, so that a turn that arrives from
// any path reports the same way.
func (s *Session) verboseOn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verbose
}

// toggleVerbose turns the stream detail on or off.
//
// It is a display mode rather than a recording one. Nothing about the request,
// the reply, or the conversation changes because of it, so what it adds is
// only the shape of the stream as it arrives.
func (s *Session) toggleVerbose() {
	s.mu.Lock()
	s.verbose = !s.verbose
	state := "off"
	if s.verbose {
		state = "on"
	}
	s.mu.Unlock()
	s.appendLines("verbose " + state +
		": the pane reports the shape of each streamed turn")
}
