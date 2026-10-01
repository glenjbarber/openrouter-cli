package openrouter

import "testing"

// A model with both prices at zero costs nothing, and is free.
func TestModelFree(t *testing.T) {
	m := Model{PromptPrice: "0", CompletionPrice: "0"}
	if !m.Free() {
		t.Error("Free = false for a zero-priced model")
	}
}

// A model with any charge is not free, whatever the other price is.
func TestModelNotFreeWhenCharged(t *testing.T) {
	for _, tc := range []struct{ prompt, completion string }{
		{"0.0000015", "0"},
		{"0", "0.000002"},
		{"0.0000015", "0.0000075"},
	} {
		m := Model{PromptPrice: tc.prompt, CompletionPrice: tc.completion}
		if m.Free() {
			t.Errorf("Free = true for %+v, want a charged model", tc)
		}
	}
}

// An absent price is not treated as free. The endpoint omits the field for a
// model it does not price, and assuming otherwise would list a paid model as
// free and spend an allowance on it.
func TestModelAbsentPriceIsNotFree(t *testing.T) {
	for _, m := range []Model{
		{},
		{PromptPrice: "0"},
		{CompletionPrice: "0"},
	} {
		if m.Free() {
			t.Errorf("Free = true for %+v, want an unpriced model treated as paid", m)
		}
	}
}

// A price the endpoint quotes in exponent form is still read, since it is
// quoted that way in practice.
func TestModelExponentPrice(t *testing.T) {
	m := Model{PromptPrice: "0e0", CompletionPrice: "0e0"}
	if !m.Free() {
		t.Error("Free = false for a zero price written in exponent form")
	}
	m2 := Model{PromptPrice: "1.5e-6", CompletionPrice: "0"}
	if m2.Free() {
		t.Error("Free = true for a charged price in exponent form")
	}
}

// A price that cannot be read is not treated as free.
func TestModelUnreadablePriceIsNotFree(t *testing.T) {
	m := Model{PromptPrice: "free", CompletionPrice: "0"}
	if m.Free() {
		t.Error("Free = true for an unreadable price")
	}
}
