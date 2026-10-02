package tui

import (
	"strings"
	"testing"
)

// The question is drawn as a box, since it is the one thing on screen that asks
// the reader to do something rather than telling them something.

// The box is closed on all four sides.
func TestTheQuestionIsDrawnAsABox(t *testing.T) {
	rows := confirmBox(Frame{Confirm: "run go build ./... here?"}, 40)
	if len(rows) < 3 {
		t.Fatalf("the box is %d rows: %q", len(rows), rows)
	}
	if !strings.HasPrefix(rows[0], boxTopLeft) {
		t.Errorf("the top is %q, want a corner", rows[0])
	}
	last := rows[len(rows)-1]
	if !strings.HasPrefix(last, boxBottomLeft) || !strings.HasSuffix(last, boxBottomRight) {
		t.Errorf("the bottom is %q, want two corners", last)
	}
	for i, row := range rows[1 : len(rows)-1] {
		if !strings.HasPrefix(row, boxVertical) || !strings.HasSuffix(row, boxVertical) {
			t.Errorf("row %d is %q, want two sides", i, row)
		}
	}
}

// A box of ragged rows is not a box.
func TestTheBoxIsAsWideAsItIsTall(t *testing.T) {
	for _, width := range []int{20, 40, 60, 100} {
		for _, row := range confirmBox(Frame{Confirm: "run go"}, width) {
			if got := displayWidth(row); got != width {
				t.Errorf("at width %d a row is %d columns: %q", width, got, row)
			}
		}
	}
}

// The question is folded rather than cut, since the command it asks about is
// the one part that must not be hidden.
func TestALongQuestionIsFoldedInsideTheBox(t *testing.T) {
	long := "run " + strings.Repeat("a very long argument ", 12) + "here?"
	rows := confirmBox(Frame{Confirm: long}, 40)

	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, strings.TrimSpace(long[:20])) {
		t.Errorf("the question does not start where it should:\n%s", joined)
	}
	if !strings.Contains(joined, "here?") {
		t.Errorf("the question was cut rather than folded:\n%s", joined)
	}
	if len(rows) < 4 {
		t.Errorf("a question this long was drawn on %d rows", len(rows))
	}
}

// A terminal too narrow for a box gets the bare question. A box two columns
// wide with the text cut to nothing inside it is worse than no box.
func TestANarrowTerminalGetsTheBareQuestion(t *testing.T) {
	rows := confirmBox(Frame{Confirm: "run go?"}, 5)

	if len(rows) != 1 {
		t.Fatalf("a narrow terminal was given %d rows: %q", len(rows), rows)
	}
	if strings.Contains(rows[0], boxVertical) {
		t.Errorf("a box was drawn where there is no room for one: %q", rows[0])
	}
}

// No question, no box.
func TestNoQuestionDrawsNoBox(t *testing.T) {
	if rows := confirmBox(Frame{}, 40); rows != nil {
		t.Errorf("a frame with no question drew %q", rows)
	}
	if got := confirmBoxRows(nil, 20); got != 0 {
		t.Errorf("a frame with no question took %d rows", got)
	}
}

// The choice the reader has moved to is shown inside the box, beside the
// question asking for it.
func TestTheChoiceIsShownInTheBox(t *testing.T) {
	rows := confirmBox(Frame{
		Confirm:       "run go?",
		ConfirmChoice: "yes, once",
	}, 40)

	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "yes, once") {
		t.Errorf("the choice is not in the box:\n%s", joined)
	}
	// Marked, so a reader can see what they are about to choose rather than
	// whether they have chosen it.
	if !strings.Contains(joined, "> yes, once") {
		t.Errorf("the choice is not marked:\n%s", joined)
	}
}

// The box is budgeted rather than drawn past the budget, since a box pushed
// off the bottom takes the prompt with it and a reader with no prompt cannot
// answer the question.
func TestTheBoxIsBoundedByTheRoom(t *testing.T) {
	box := confirmBox(Frame{Confirm: strings.Repeat("a long question ", 8)}, 40)

	if got := confirmBoxRows(box, 2); got != 2 {
		t.Errorf("two rows of room took %d", got)
	}
	if got := confirmBoxRows(box, 0); got != 0 {
		t.Errorf("no room took %d rows", got)
	}
	if got := confirmBoxRows(box, 100); got != len(box) {
		t.Errorf("plenty of room took %d of %d rows", got, len(box))
	}
}

// A selection out of the box is the text, not the text with padding after it.
func TestTheBoxCarriesNoEscapeSequences(t *testing.T) {
	for _, row := range confirmBox(Frame{Confirm: "run go?", ConfirmChoice: "yes"}, 40) {
		if strings.Contains(row, "\x1b") {
			t.Errorf("a row of the box carries an escape: %q", row)
		}
	}
}

// The box is drawn on the input block rather than in the pane, so it does not
// scroll away into the history.
func TestTheBoxIsAboveThePrompt(t *testing.T) {
	rows := Render(Frame{
		Reply:   []string{"a reply"},
		Confirm: "run go?",
	}, 24, 40)

	at := -1
	for i, row := range rows {
		if strings.HasPrefix(row, boxTopLeft) {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no box in the frame:\n%s", strings.Join(rows, "\n"))
	}
	promptAt := promptRow(rows)
	if promptAt < 0 {
		t.Fatalf("no prompt in the frame:\n%s", strings.Join(rows, "\n"))
	}
	if at > promptAt {
		t.Errorf("the box is at row %d, below the prompt at %d", at, promptAt)
	}
}
