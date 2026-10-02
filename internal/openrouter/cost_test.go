package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// The cost of a response is read from the usage object, as a number or as a
// string holding one, and an absent or unusable figure is nil rather than zero,
// since a reported zero is a free response and a missing figure is not.
func TestTheCostIsReadFromTheUsage(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name string
		body string
		want *float64
	}{
		{"a number", `{"prompt_tokens":1,"completion_tokens":2,"cost":0.00123}`, f(0.00123)},
		{"a string", `{"prompt_tokens":1,"completion_tokens":2,"cost":"0.5"}`, f(0.5)},
		{"exponent form", `{"cost":1.5e-5}`, f(0.000015)},
		{"a reported zero", `{"prompt_tokens":1,"cost":0}`, f(0)},
		{"absent", `{"prompt_tokens":1,"completion_tokens":2}`, nil},
		{"null", `{"prompt_tokens":1,"cost":null}`, nil},
		{"not a figure", `{"cost":"free"}`, nil},
		{"an object", `{"cost":{}}`, nil},
		{"negative", `{"cost":-0.01}`, nil},
		{"too large to hold", `{"cost":1e999}`, nil},
	} {
		var u streamUsage
		if err := json.Unmarshal([]byte(tc.body), &u); err != nil {
			t.Errorf("%s: decoding: %v", tc.name, err)
			continue
		}
		switch {
		case tc.want == nil && u.Cost != nil:
			t.Errorf("%s: Cost = %v, want none", tc.name, *u.Cost)
		case tc.want != nil && u.Cost == nil:
			t.Errorf("%s: Cost is absent, want %v", tc.name, *tc.want)
		case tc.want != nil && *u.Cost != *tc.want:
			t.Errorf("%s: Cost = %v, want %v", tc.name, *u.Cost, *tc.want)
		}
	}
}

// The usage that is already read is read as it was, with a cost beside it or not.
func TestTheCostDoesNotDisturbTheCounts(t *testing.T) {
	var u streamUsage
	if err := json.Unmarshal([]byte(`{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33,"cost":0.25}`), &u); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if u.PromptTokens != 11 || u.CompletionTokens != 22 {
		t.Errorf("usage = %+v, want 11 and 22", u)
	}
}

// The cost reaches the caller on the usage event of a stream, which is the one
// event the client asks the endpoint to send it on.
func TestChatDeliversTheCost(t *testing.T) {
	body := `data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n" +
		`data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":22,"cost":0.0042}}` + "\n\n" +
		"data: [DONE]\n\n"

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var got *streamUsage
	err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Usage != nil {
			got = e.Usage
		}
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got == nil || got.Cost == nil {
		t.Fatalf("usage = %+v, want a cost", got)
	}
	if *got.Cost != 0.0042 {
		t.Errorf("Cost = %v, want 0.0042", *got.Cost)
	}
}

// A response with usage and no cost reports no cost, so a caller can tell it
// from one priced at nothing.
func TestChatReportsNoCostWhenTheEndpointSendsNone(t *testing.T) {
	body := `data: {"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":22}}` + "\n\n" +
		"data: [DONE]\n\n"

	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	})

	var got *streamUsage
	if err := c.Chat(context.Background(), ChatRequest{
		Model:    "test/model",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(e StreamEvent) {
		if e.Usage != nil {
			got = e.Usage
		}
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got == nil {
		t.Fatal("no usage was delivered")
	}
	if got.Cost != nil {
		t.Errorf("Cost = %v, want none", *got.Cost)
	}
}

// The prices of a model are read from the pricing object the catalogue
// documents, and from the model itself where there is none, and a pair that is
// not wholly known is not known at all.
func TestAModelPricesAreReadEitherWay(t *testing.T) {
	for _, tc := range []struct {
		name         string
		body         string
		prompt, comp float64
		ok           bool
	}{
		{"the pricing object", `{"id":"a","pricing":{"prompt":"0.000001","completion":"0.000002"}}`, 0.000001, 0.000002, true},
		{"the model itself", `{"id":"a","prompt":"0.000003","completion":"0.000004"}`, 0.000003, 0.000004, true},
		{"numbers", `{"id":"a","pricing":{"prompt":0.000001,"completion":0.000002}}`, 0.000001, 0.000002, true},
		{"the object wins", `{"id":"a","prompt":"9","completion":"9","pricing":{"prompt":"0.1","completion":"0.2"}}`, 0.1, 0.2, true},
		{"a member missing from the object", `{"id":"a","completion":"0.2","pricing":{"prompt":"0.1"}}`, 0.1, 0.2, true},
		{"free", `{"id":"a","pricing":{"prompt":"0","completion":"0"}}`, 0, 0, true},
		{"no price", `{"id":"a"}`, 0, 0, false},
		{"one price", `{"id":"a","pricing":{"prompt":"0.1"}}`, 0, 0, false},
		{"a route-dependent price", `{"id":"a","pricing":{"prompt":"-1","completion":"-1"}}`, 0, 0, false},
		{"junk", `{"id":"a","pricing":{"prompt":"free","completion":"0"}}`, 0, 0, false},
	} {
		var m Model
		if err := json.Unmarshal([]byte(tc.body), &m); err != nil {
			t.Errorf("%s: decoding: %v", tc.name, err)
			continue
		}
		p, c, ok := m.Prices()
		if ok != tc.ok || p != tc.prompt || c != tc.comp {
			t.Errorf("%s: Prices() = %v, %v, %v, want %v, %v, %v",
				tc.name, p, c, ok, tc.prompt, tc.comp, tc.ok)
		}
	}
}

// Reading the prices from the pricing object does not change which models are
// free.
func TestFreeReadsThePricingObjectToo(t *testing.T) {
	var free, paid Model
	if err := json.Unmarshal([]byte(`{"id":"a","pricing":{"prompt":"0","completion":"0"}}`), &free); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"id":"b","pricing":{"prompt":"0.000001","completion":"0"}}`), &paid); err != nil {
		t.Fatal(err)
	}
	if !free.Free() {
		t.Error("a model priced at nothing in the pricing object is not free")
	}
	if paid.Free() {
		t.Error("a model with a price in the pricing object is free")
	}
}
