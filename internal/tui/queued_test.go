package tui

import "testing"

// A queued line is sent as it was typed. The queue marks a line on the screen
// rather than in the text the model is sent, so what the model reads is the
// line itself and nothing is added to it.
func TestAQueuedLineIsSentAsItWasTyped(t *testing.T) {
	if got := queuedLines([]string{"an update"}); len(got) != 1 || got[0] != "an update" {
		t.Errorf("queuedLines gave %q, want the line as it was typed", got)
	}
}

// The pane is not marked twice. The queue is already drawn with queuedMarker, and
// a row reading "queued an update" says the same thing in two ways.
func TestThePaneIsNotPrefixedAsWellAsMarked(t *testing.T) {
	if got := queuedLines([]string{"an update"}); len(got) != 1 || got[0] != "an update" {
		t.Errorf("queuedLines gave %q, want no prefix added to the text", got)
	}
}

// A line that is nothing but whitespace is dropped rather than carried, since
// amend already joins the lines with a blank one.
func TestAnEmptyQueuedLineIsDropped(t *testing.T) {
	got := queuedLines([]string{"an update", "   ", ""})
	if len(got) != 1 {
		t.Fatalf("queuedLines gave %q, want the empty lines dropped", got)
	}
	if got[0] != "an update" {
		t.Errorf("queuedLines gave %q, want the line as it was typed", got[0])
	}
}

// A queue with nothing in it is nothing rather than a slice of one empty line.
func TestAnEmptyQueueMarksNothing(t *testing.T) {
	if got := queuedLines(nil); got != nil {
		t.Errorf("queuedLines(nil) = %q, want nil", got)
	}
}

// Every line of the queue is kept in the order it was sent, since a stop folds
// them all into one update and the order they were committed in is the order
// they were meant in.
func TestEveryQueuedLineIsKeptInOrder(t *testing.T) {
	got := queuedLines([]string{"first", "second"})
	if len(got) != 2 {
		t.Fatalf("queuedLines gave %q, want two lines", got)
	}
	if got[0] != "first" || got[1] != "second" {
		t.Errorf("queuedLines gave %q, want them in the order they were sent", got)
	}
}