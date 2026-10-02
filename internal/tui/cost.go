package tui

import (
	"context"
	"fmt"
	"sync"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// This file holds what a session has spent.
//
// The figure is the cost of every request the session has made, summed as each
// response reports its usage: a turn, each tool round of a turn, a compaction,
// a delegate and a worker. It belongs to the session rather than to a
// conversation, since money spent is not given back by clearing the
// conversation, switching to a thread or loading a saved one. It is held in
// memory only, so a new session begins at nothing and nothing is written to a
// file or to the database.

// costLedger is the running cost of a session, in US dollars.
//
// It is guarded by a lock of its own rather than by the session lock. A
// response is accounted for on the goroutine of the request that made it, and a
// delegate and a worker run beside the conversation, so the ledger is written
// from several goroutines while the paint path reads it.
type costLedger struct {
	mu sync.Mutex
	// total is the sum of every figure added.
	total float64
	// known reports that at least one response has added a figure, whether
	// reported, estimated or zero. With none the ledger has nothing to show, and
	// the bar shows a dash rather than a zero that would claim the session was
	// free.
	known bool
	// inexact reports that the total is not wholly what the endpoint charged:
	// some part of it was estimated from the catalogue price, or some response
	// was not priced at all and so is missing from it.
	inexact bool
}

// reported adds a cost the endpoint reported for a response. A reported zero is
// a free response and is added as such.
func (l *costLedger) reported(usd float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total += usd
	l.known = true
}

// estimated adds a cost worked out from the catalogue price. It marks the total
// as inexact, since the endpoint did not say what was charged.
func (l *costLedger) estimated(usd float64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total += usd
	l.known = true
	l.inexact = true
}

// unpriced notes a response that carried no cost and had no price to estimate
// from. It adds nothing, but the total is then missing a response, so it is
// marked inexact rather than left looking complete.
func (l *costLedger) unpriced() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.inexact = true
}

// text is the figure as the top bar shows it, or empty when the ledger has
// nothing to show. A leading tilde marks a total that is not wholly reported.
func (l *costLedger) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.known {
		return ""
	}
	out := formatCost(l.total)
	if l.inexact {
		out = "~" + out
	}
	return out
}

// formatCost renders an amount of US dollars.
//
// A response costs a fraction of a cent far more often than a cent, so below a
// dollar the figure carries four decimal places, enough that a single cheap
// request is visible and that the figure moves as the session goes on. From a
// dollar up two places are enough, since the cents are what a reader weighs.
func formatCost(usd float64) string {
	if usd < 1 {
		return fmt.Sprintf("$%.4f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// usageFunc is told about the usage a response reported, with the model that
// answered. A request that is not the conversation's own, such as a compaction,
// a delegate or a worker, is given one so that its spend reaches the same
// ledger without the type knowing about the session.
type usageFunc func(model string, e openrouter.StreamEvent)

// priceBook remembers what each model charges per token, for the response that
// reports no cost of its own.
//
// The catalogue is fetched once, the first time a price is wanted, rather than
// at startup. The endpoint reports a cost with every response when asked, so
// the book is the fallback and most sessions never open it. Its lock is its
// own, since the price is wanted from the goroutine of whichever request
// finished.
type priceBook struct {
	mu      sync.Mutex
	fetched bool
	byModel map[string][2]float64
}

// lookup returns the prompt and completion price of a model, in US dollars per
// token, and whether they are known. A model the catalogue does not carry, a
// price it does not quote and a catalogue that cannot be fetched are all
// unknown. A failed fetch is not remembered, so a later response tries again.
func (p *priceBook) lookup(ctx context.Context, c *openrouter.Client, model string) (prompt, completion float64, ok bool) {
	if p == nil || c == nil || model == "" {
		return 0, 0, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.fetched {
		models, err := c.Models(ctx)
		if err != nil {
			return 0, 0, false
		}
		p.byModel = make(map[string][2]float64, len(models))
		for _, m := range models {
			if in, out, known := m.Prices(); known {
				p.byModel[m.ID] = [2]float64{in, out}
			}
		}
		p.fetched = true
	}
	price, found := p.byModel[model]
	return price[0], price[1], found
}

// noteSpend adds the cost of one response to the session total, from the usage
// event that closed it.
//
// The cost the endpoint reported is used when there is one. Without it the cost
// is the tokens multiplied by the catalogue price, and the total is marked as
// an estimate. A model priced at nothing adds zero exactly, since that is a
// known cost rather than a guess. With neither a reported cost nor a price the
// response adds nothing and the total is marked as incomplete.
//
// A response that reported no usage at all, as a failed or stopped request does
// not, adds nothing, since there is nothing to say what it cost.
func (s *Session) noteSpend(ctx context.Context, model string, e openrouter.StreamEvent) {
	u := e.Usage
	if u == nil {
		return
	}
	if u.Cost != nil {
		s.cost.reported(*u.Cost)
	} else if in, out, ok := s.prices.lookup(ctx, s.client, model); !ok {
		s.cost.unpriced()
	} else if in == 0 && out == 0 {
		s.cost.reported(0)
	} else {
		s.cost.estimated(float64(u.PromptTokens)*in + float64(u.CompletionTokens)*out)
	}
	s.refreshCost()
}

// noteSideSpend is the usageFunc for a request made beside the conversation.
func (s *Session) noteSideSpend(model string, e openrouter.StreamEvent) {
	s.noteSpend(s.ctx, model, e)
	s.draw()
}

// refreshCost copies the ledger into the status. The text is worked out before
// the session lock is taken, since the paint path copies the whole frame under
// it.
func (s *Session) refreshCost() {
	text := s.cost.text()
	s.mu.Lock()
	s.frame.Status.Cost = text
	s.mu.Unlock()
}
