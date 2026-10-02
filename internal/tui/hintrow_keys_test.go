package tui

import (
	"strings"
	"testing"
)

// The break key is named on the idle prompt, since a reader holding a
// multi-line message has no other way to find it. Terminals that send nothing
// for shift with enter make it the only one that acts everywhere.
//
// The composite literal is parenthesised in the range clause because a literal
// of the form TypeName{ is not permitted in the header of a control statement,
// where the parser cannot tell it from the brace that opens the block. In an
// assignment the same call needs no parentheses, which is why the table in
// hintrow_test.go spells it without them.
func TestHintRowNamesTheBreakKeyOnAnIdlePrompt(t *testing.T) {
	hints := hintState{}.hints()
	for _, got := range hints {
		if strings.Contains(got, "ctrl") {
			return
		}
	}
	t.Errorf("hints = %q, want the break key named", hints)
}

// While a model is working, Enter queues and escape stops with the line in
// hand, so the break key is not named there. A reader is not composing a
// multi-line message while waiting for a reply, and the row names what the
// keys do in the state being described.
func TestHintRowDoesNotNameTheBreakKeyWhileWorking(t *testing.T) {
	hints := hintState{busy: true}.hints()
	for _, got := range hints {
		if strings.Contains(got, "ctrl") {
			t.Errorf("hints = %q, want no break key while a model works", hints)
		}
	}
}

// The row names what the keys do in the state being described, and the two
// entries sit in the same order whatever the state, so the row can be read at a
// glance rather than rescanned.
func TestHintRowKeepsItsOrderAcrossStates(t *testing.T) {
	idle := hintState{}.hints()
	busy := hintState{busy: true}.hints()

	if len(idle) == 0 || len(busy) == 0 {
		t.Fatal("a state named no keys at all")
	}
	if idle[0] != "Send [enter]" {
		t.Errorf("the first entry is %q, want the key that sends", idle[0])
	}
	if busy[0] != "Enter queue" {
		t.Errorf("the first entry is %q, want what Enter does while working", busy[0])
	}
}

// The break key is named second on an idle prompt, after the key that sends.
// A reader reads the row left to right and looks for what sends first, since
// that is what they came for.
func TestHintRowNamesTheSendKeyBeforeTheBreakKey(t *testing.T) {
	hints := hintState{}.hints()
	if len(hints) < 2 {
		t.Fatalf("hints = %q, want both entries", hints)
	}
	if hints[0] != "Send [enter]" || !strings.Contains(hints[1], "ctrl") {
		t.Errorf("hints = %q, want send first and the break key second", hints)
	}
}
