package tui

import "testing"

// A queued line is prefixed before it is sent, so that the model reads it as a
// follow-up or an urgent interruption rather than as a question standing on its
// own. A blank line joining the two reads as a new topic, which is the opposite
// of what a queued line is.
func TestAQueuedLineIsPrefixedWhenItIsSent(t *testing.T) {
	if got := asQueued("an update"); got != "[QUEUED] an update" {
		t.Errorf("asQueued gave %q, want the prefix", got)
	}
}

// The pane is not marked twice. The queue is already drawn with queuedMarker,
// and a row reading "queued [QUEUED] an update" says the same thing twice.
func TestThePaneIsNotPrefixedAsWellAsMarked(t *testing.T) {
	if got := asQueued("an update"); got != queuedPrefix+"an update" {
		t.Errorf("asQueued gave %q, want the marker added once", got)
	}
}

// A reader who typed the marker themselves should not be shown it twice, and a
// line queued twice by a retry would otherwise grow a second copy.
func TestAPrefixAlreadyPresentIsNotDoubled(t *testing.T) {
	line := "[QUEUED] an update"
	if got := asQueued(line); got != line {
		t.Errorf("asQueued gave %q, want it left as it is", got)
	}
}

// A line that is nothing but whitespace is dropped rather than marked, since a
// marker alone would be a queued message with no message in it.
func TestAnEmptyQueuedLineIsDropped(t *testing.T) {
	got := queuedTexts([]string{"an update", "   ", ""})
	if len(got) != 1 {
		t.Fatalf("queuedTexts gave %q, want the empty lines dropped", got)
	}
	if got[0] != "[QUEUED] an update" {
		t.Errorf("queuedTexts gave %q, want the prefix", got[0])
	}
}

// A queue with nothing in it is nothing rather than a slice of one empty line.
func TestAnEmptyQueueMarksNothing(t *testing.T) {
	if got := queuedTexts(nil); got != nil {
		t.Errorf("queuedTexts(nil) = %q, want nil", got)
	}
}

// Every line of the queue is marked, since a stop folds them all into one
// update and an unmarked line among marked ones reads as a plain question.
func TestEveryQueuedLineIsMarked(t *testing.T) {
	got := queuedTexts([]string{"first", "second"})
	if len(got) != 2 {
		t.Fatalf("queuedTexts gave %q, want two lines", got)
	}
	for i, line := range got {
		if !hasPrefix(line, queuedPrefix) {
			t.Errorf("line %d = %q, want the prefix", i, line)
		}
	}
}

// hasPrefix reports whether s begins with prefix, kept here so the test does
// not reach for strings in a file that is about one constant.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
