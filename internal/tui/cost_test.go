package tui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// costBody is a reply that ends with a usage chunk. The cost is written in as
// given, or left out when it is empty, so that a response the endpoint did not
// price can be told from one it priced at nothing.
func costBody(cost string) string {
	usage := `"prompt_tokens":1000,"completion_tokens":500`
	if cost != "" {
		usage += `,"cost":` + cost
	}
	return `data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{` + usage + `}}` + "\n\n" +
		"data: [DONE]\n\n"
}

// costServer answers the catalogue with the models given and every completion
// with the next body in the list, repeating the last once they run out.
func costServer(t *testing.T, models string, bodies ...string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	next := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, models)
			return
		}
		mu.Lock()
		body := bodies[min(next, len(bodies)-1)]
		next++
		mu.Unlock()
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// paidModels prices the test model at a millionth of a dollar for a token sent
// and two millionths for a token received, so 1000 and 500 tokens come to two
// thousandths of a dollar.
const paidModels = `{"data":[{"id":"test/model","context_length":8192,` +
	`"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`

// freeModels prices the test model at nothing.
const freeModels = `{"data":[{"id":"test/model","context_length":8192,` +
	`"pricing":{"prompt":"0","completion":"0"}}]}`

// noModels does not carry the test model at all.
const noModels = `{"data":[]}`

func costTestSession(t *testing.T, srv *httptest.Server) *Session {
	t.Helper()
	s := delegateSession(t, srv.URL)
	return s
}

// topBarOf returns the top bar of the frame the session would draw, found by
// content rather than by position.
func topBarOf(t *testing.T, s *Session) string {
	t.Helper()
	s.mu.Lock()
	frame := s.frame
	s.mu.Unlock()
	for _, row := range Render(frame, 30, 200) {
		if strings.Contains(row, "Context:") {
			return row
		}
	}
	t.Fatal("no top bar in the frame")
	return ""
}

// Before any response has reported usage there is nothing to show, so the bar
// carries a dash rather than a zero that would claim the session was free.
func TestTheCostIsADashBeforeAnyResponse(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("0.01")))
	s.updateStatus()

	if got := s.frame.Status.Cost; got != "" {
		t.Errorf("Cost = %q, want it empty before any response", got)
	}
	if bar := topBarOf(t, s); !strings.Contains(bar, "Cost: -") {
		t.Errorf("top bar = %q, want a dash for the cost", bar)
	}
}

// The cost the endpoint reports for a response is shown as it reported it, in
// the top bar and nowhere else.
func TestAReportedCostIsShownInTheTopBar(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("0.0123")))

	s.send(s.ctx, s.conv, "hello")

	if got := s.frame.Status.Cost; got != "$0.0123" {
		t.Errorf("Cost = %q, want $0.0123", got)
	}
	if bar := topBarOf(t, s); !strings.Contains(bar, "Cost: $0.0123") {
		t.Errorf("top bar = %q, want the reported cost", bar)
	}
}

// A reported cost is used even when the catalogue prices the model, since the
// endpoint knows what it charged and the catalogue is only a rate.
func TestAReportedCostWinsOverTheCatalogue(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("0.5")))

	s.send(s.ctx, s.conv, "hello")

	if got := s.frame.Status.Cost; got != "$0.5000" {
		t.Errorf("Cost = %q, want the reported $0.5000 and no tilde", got)
	}
}

// The cost is the sum over the session, one response after the next.
func TestTheCostAccumulatesAcrossRequests(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("0.0123"), costBody("0.0100")))

	s.send(s.ctx, s.conv, "one")
	s.send(s.ctx, s.conv, "two")

	if got := s.frame.Status.Cost; got != "$0.0223" {
		t.Errorf("Cost = %q, want the two responses summed to $0.0223", got)
	}
}

// Every round of a turn that calls tools is a request that was paid for, so the
// rounds are summed rather than the last one kept.
func TestToolRoundsAreSummed(t *testing.T) {
	round := func(stream, cost string) string {
		return strings.Replace(stream, "data: [DONE]",
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"cost":`+cost+`}}`+
				"\n\ndata: [DONE]", 1)
	}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, paidModels)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		if !strings.Contains(string(raw), `"role":"tool"`) {
			io.WriteString(w, round(toolCallStream("list_dir", map[string]any{"path": "."}), "0.01"))
			return
		}
		io.WriteString(w, round(textStream("done"), "0.02"))
	}))
	t.Cleanup(srv.Close)

	s := toolSession(t, srv.URL, t.TempDir())
	s.send(s.ctx, s.conv, "look around")

	if got := s.frame.Status.Cost; got != "$0.0300" {
		t.Errorf("Cost = %q, want both rounds of the turn summed to $0.0300", got)
	}
}

// With no cost reported, the figure is the tokens multiplied by the catalogue
// price, and it is marked as an estimate with a leading tilde.
func TestAnEstimateIsMarked(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("")))

	s.send(s.ctx, s.conv, "hello")

	if got := s.frame.Status.Cost; got != "~$0.0020" {
		t.Errorf("Cost = %q, want 1000 x 0.000001 + 500 x 0.000002 as ~$0.0020", got)
	}
	if bar := topBarOf(t, s); !strings.Contains(bar, "Cost: ~$0.0020") {
		t.Errorf("top bar = %q, want the estimate marked", bar)
	}
}

// With neither a reported cost nor a price, there is nothing to show, and the
// bar says so with a dash rather than inventing a figure.
func TestNoCostAndNoPriceIsADash(t *testing.T) {
	s := costTestSession(t, costServer(t, noModels, costBody("")))

	s.send(s.ctx, s.conv, "hello")

	if got := s.frame.Status.Cost; got != "" {
		t.Errorf("Cost = %q, want nothing when there is no source for it", got)
	}
	if bar := topBarOf(t, s); !strings.Contains(bar, "Cost: -") {
		t.Errorf("top bar = %q, want a dash", bar)
	}
}

// A model priced at nothing adds zero, and the figure is exact rather than an
// estimate, since a free model is a known cost and not a guess.
func TestAFreeModelAddsZero(t *testing.T) {
	for name, body := range map[string]string{
		"reported":  costBody("0"),
		"catalogue": costBody(""),
	} {
		models := paidModels
		if name == "catalogue" {
			models = freeModels
		}
		s := costTestSession(t, costServer(t, models, body))

		s.send(s.ctx, s.conv, "hello")

		if got := s.frame.Status.Cost; got != "$0.0000" {
			t.Errorf("%s: Cost = %q, want an exact zero", name, got)
		}
	}
}

// A free response added to a paid one leaves the total as it was.
func TestAFreeResponseLeavesTheTotalAsItWas(t *testing.T) {
	s := costTestSession(t, costServer(t, paidModels, costBody("0.0123"), costBody("0")))

	s.send(s.ctx, s.conv, "one")
	s.send(s.ctx, s.conv, "two")

	if got := s.frame.Status.Cost; got != "$0.0123" {
		t.Errorf("Cost = %q, want $0.0123", got)
	}
}

// A response with no cost and no price is missing from the total, so the total
// is marked rather than left looking complete.
func TestAnUnpricedResponseMarksTheTotal(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io.WriteString(w, noModels)
			return
		}
		mu.Lock()
		asked++
		n := asked
		mu.Unlock()
		if n == 1 {
			io.WriteString(w, costBody("0.0100"))
			return
		}
		io.WriteString(w, costBody(""))
	}))
	t.Cleanup(srv.Close)
	s := costTestSession(t, srv)

	s.send(s.ctx, s.conv, "one")
	if got := s.frame.Status.Cost; got != "$0.0100" {
		t.Fatalf("Cost = %q after the first response, want $0.0100", got)
	}
	s.send(s.ctx, s.conv, "two")
	if got := s.frame.Status.Cost; got != "~$0.0100" {
		t.Errorf("Cost = %q, want the total marked as missing a response", got)
	}
}

// The cost of a compaction, a delegate and a worker reaches the same total as
// the conversation's own requests, since they are requests the session paid for.
func TestSideRequestsAreCounted(t *testing.T) {
	t.Run("compaction", func(t *testing.T) {
		s := costTestSession(t, costServer(t, paidModels, costBody("0.0040")))
		msgs := []openrouter.Message{
			{Role: openrouter.RoleUser, Content: "a question"},
			{Role: openrouter.RoleAssistant, Content: "an answer"},
		}
		if _, err := SummariseCounted(context.Background(), s.client, "test/model", msgs, s.noteSideSpend); err != nil {
			t.Fatalf("SummariseCounted: %v", err)
		}
		if got := s.frame.Status.Cost; got != "$0.0040" {
			t.Errorf("Cost = %q, want the compaction counted", got)
		}
	})
	t.Run("delegate", func(t *testing.T) {
		s := costTestSession(t, costServer(t, paidModels, costBody("0.0070")))
		s.startDelegate("a question")
		waitFor(t, func() bool { return s.delegatePending() == 0 && dpaneHasLines(s) },
			"the delegate never settled")
		s.mu.Lock()
		got := s.frame.Status.Cost
		s.mu.Unlock()
		if got != "$0.0070" {
			t.Errorf("Cost = %q, want the delegate counted", got)
		}
	})
	t.Run("worker", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		s := toolSession(t, costServer(t, paidModels, costBody("0.0090")).URL, t.TempDir())
		s.startSpawn("what is in here")
		waitFor(t, func() bool { return s.workerPending() == 0 && wpaneHasLines(s) },
			"the worker never settled")
		s.mu.Lock()
		got := s.frame.Status.Cost
		s.mu.Unlock()
		if got != "$0.0090" {
			t.Errorf("Cost = %q, want the worker counted", got)
		}
	})
}

// Cost belongs to the session, so a new session begins at nothing, and clearing
// the conversation does not give back what was spent.
func TestTheCostResetsWithANewSessionOnly(t *testing.T) {
	srv := costServer(t, paidModels, costBody("0.0123"))
	s := costTestSession(t, srv)
	s.send(s.ctx, s.conv, "hello")

	s.cmdNew(nil)
	s.updateStatus()
	if got := s.frame.Status.Cost; got != "$0.0123" {
		t.Errorf("Cost = %q after /new, want what was spent kept", got)
	}

	fresh := costTestSession(t, srv)
	fresh.updateStatus()
	if got := fresh.frame.Status.Cost; got != "" {
		t.Errorf("a new session starts with Cost = %q, want nothing", got)
	}
}

// A response that carries no usage adds nothing, such as one that was cut off.
func TestAResponseWithNoUsageAddsNothing(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"Hello"}}]}` + "\n\ndata: [DONE]\n\n"
	s := costTestSession(t, costServer(t, paidModels, body))

	s.send(s.ctx, s.conv, "hello")

	if got := s.frame.Status.Cost; got != "" {
		t.Errorf("Cost = %q, want nothing for a response that reported no usage", got)
	}
}

// The figure carries four decimal places under a dollar and two from a dollar
// up, so that one cheap request is visible and a large total stays short.
func TestTheCostFormat(t *testing.T) {
	for _, tc := range []struct {
		usd  float64
		want string
	}{
		{0, "$0.0000"},
		{0.00004, "$0.0000"},
		{0.0001, "$0.0001"},
		{0.0123, "$0.0123"},
		{0.5, "$0.5000"},
		{1, "$1.00"},
		{12.5, "$12.50"},
		{1234.5, "$1234.50"},
	} {
		if got := formatCost(tc.usd); got != tc.want {
			t.Errorf("formatCost(%v) = %q, want %q", tc.usd, got, tc.want)
		}
	}
}

// The ledger on its own: nothing is a dash, an estimate or a missing response
// marks the total, and an exact total carries no mark.
func TestTheLedgerMarksATotalThatIsNotExact(t *testing.T) {
	var l costLedger
	if got := l.text(); got != "" {
		t.Errorf("an empty ledger shows %q, want nothing", got)
	}
	l.unpriced()
	if got := l.text(); got != "" {
		t.Errorf("a ledger with only an unpriced response shows %q, want nothing", got)
	}
	l.reported(0.25)
	if got := l.text(); got != "~$0.2500" {
		t.Errorf("text = %q, want the missing response marked", got)
	}

	var exact costLedger
	exact.reported(0.25)
	exact.reported(0.5)
	if got := exact.text(); got != "$0.7500" {
		t.Errorf("text = %q, want an exact total with no mark", got)
	}
	exact.estimated(0.25)
	if got := exact.text(); got != "~$1.00" {
		t.Errorf("text = %q, want an estimate to mark the total", got)
	}
}

// The ledger is written from the goroutine of whichever request finished, so it
// holds up under concurrent use. The race detector is what this is for.
func TestTheLedgerIsSafeToShare(t *testing.T) {
	var l costLedger
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				l.reported(0.0001)
				_ = l.text()
			}
		}()
	}
	wg.Wait()
	if got := l.text(); got != fmt.Sprintf("$%.4f", 0.0800) {
		t.Errorf("text = %q, want 800 additions of 0.0001", got)
	}
}

// The catalogue is fetched once for the prices, and not at all while every
// response carries a cost of its own.
func TestThePriceBookIsOnlyOpenedWhenWanted(t *testing.T) {
	var mu sync.Mutex
	fetched := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			mu.Lock()
			fetched++
			mu.Unlock()
			io.WriteString(w, paidModels)
			return
		}
		io.WriteString(w, costBody("0.01"))
	}))
	t.Cleanup(srv.Close)
	s := costTestSession(t, srv)
	s.windows = nil // The context window is looked up elsewhere and is not what is counted.

	s.send(s.ctx, s.conv, "hello")

	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return fetched
	}
	if n := count(); n != 0 {
		t.Errorf("the catalogue was fetched %d times for a response that carried its cost", n)
	}

	for i := 0; i < 2; i++ {
		if _, _, ok := s.prices.lookup(context.Background(), s.client, "test/model"); !ok {
			t.Fatalf("no price for the test model on lookup %d", i+1)
		}
	}
	if n := count(); n != 1 {
		t.Errorf("the catalogue was fetched %d times for two lookups, want once", n)
	}
}
