package tui

import (
	"strings"
	"testing"
)

// A reply is divided from what came before it, so that two exchanges read as two
// rather than as one run of text.
func TestARepliesCarriesADelimiter(t *testing.T) {
	got := withResponseRule("the answer")
	if !strings.HasPrefix(got, responseRuleRow) {
		t.Errorf("withResponseRule gave %q, want the rule above the reply", got)
	}
	if !strings.HasSuffix(got, "the answer") {
		t.Errorf("withResponseRule gave %q, want the reply after the rule", got)
	}
}

// The rule is a box-drawing character rather than an ASCII dash, since a run of
// dashes reads as text and a rule reads as a rule. It is the same character the
// frame is divided with, so the two do not look like different hands.
func TestTheDelimiterIsTheRuleCharacter(t *testing.T) {
	if responseRule != ruleRune {
		t.Errorf("responseRule = %q, want it to be the rule character %q",
			responseRule, ruleRune)
	}
}

// The rule is plain text, since a selection out of the pane is copied as
// whatever is on the screen and a sequence in the row would be copied with it.
func TestTheDelimiterCarriesNoEscape(t *testing.T) {
	if strings.Contains(responseRuleRow, "\x1b") {
		t.Errorf("the delimiter carries an escape: %q", responseRuleRow)
	}
}

// A reply that is nothing draws no rule, since a rule above a blank row reads
// as the frame having drawn something for no reason. A model returning nothing
// is reported in words instead.
func TestAnEmptyReplyDrawsNoDelimiter(t *testing.T) {
	for _, empty := range []string{"", "   ", "\n"} {
		if got := withResponseRule(empty); strings.Contains(got, responseRuleRow) {
			t.Errorf("withResponseRule(%q) = %q, want nothing", empty, got)
		}
	}
}

// The trailing break is trimmed, so that a reply carrying one does not gain a
// second blank row between the rule and the text.
func TestTheTrailingBreakIsTrimmed(t *testing.T) {
	got := withResponseRule("the answer\n\n")
	if strings.HasSuffix(got, "\n") {
		t.Errorf("withResponseRule gave %q, want no trailing break", got)
	}
}

// The row count is what the budget needs, and a reply with no text takes
// nothing.
func TestTheDelimiterIsBudgetedAtOneRow(t *testing.T) {
	if got := responseRuleRows("the answer"); got != 1 {
		t.Errorf("responseRuleRows gave %d, want 1", got)
	}
	if got := responseRuleRows("  "); got != 0 {
		t.Errorf("responseRuleRows gave %d, want 0 for a reply with no text", got)
	}
}

// The rule survives folding and is drawn on a row of its own rather than being
// wrapped into the text, since a reply is folded at draw time and a rule inside
// a folded row reads as a character the model wrote.
func TestTheRuleIsDrawnOnARowOfItsOwn(t *testing.T) {
	rows := Render(Frame{
		Reply: []string{withResponseRule("the first answer")},
	}, 24, 40)

	at := -1
	for i, row := range rows {
		if row == responseRule {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no rule in the frame:\n%s", strings.Join(rows, "\n"))
	}
	// The reply is on a later row rather than the same one, so the rule did not
	// fold into the text.
	if at+1 >= len(rows) || !strings.Contains(rows[at+1], "the first answer") {
		t.Errorf("the rule is on row %d and the answer is not below it:\n%s",
			at, strings.Join(rows, "\n"))
	}
}

// The rule leaves the frame a row taller for every reply, and that row is
// budgeted rather than taken from the pane, so a conversation with rules in it
// does not scroll under the reader.
func TestTheDelimiterAddsARowThatFits(t *testing.T) {
	rows := Render(Frame{
		Reply: []string{
			withResponseRule("the first answer"),
			withResponseRule("the second answer"),
		},
	}, 24, 40)

	if len(rows) != 24 {
		t.Errorf("the frame is %d rows, want it to fit the 24 it was drawn at", len(rows))
	}
	n := 0
	for _, row := range rows {
		if row == responseRule {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d rules were drawn, want one per reply:\n%s", n, strings.Join(rows, "\n"))
	}
}
