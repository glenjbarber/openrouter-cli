package tui

import (
	"strings"
	"testing"
)

// A shifted arrow is reported as a modified sequence by most terminals, and the
// modifier has to be read out of the parameters rather than guessed from the
// length, since the forms differ between terminals.

// A shifted arrow is a page, not a recall.
func TestAShiftedArrowIsQueuedAsAPage(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  string
		want byte
	}{
		{"shift up", "\x1b[1;2A", keyPageUp},
		{"shift down", "\x1b[1;2B", keyPageDown},
	} {
		final, shifted, rest, ok := takeSequenceWithModifier([]byte(tc.seq))
		if !ok {
			t.Errorf("%s: the sequence was not taken", tc.name)
			continue
		}
		if len(rest) != 0 {
			t.Errorf("%s: %d bytes left over", tc.name, len(rest))
		}
		if !shifted {
			t.Errorf("%s: the shift was not read", tc.name)
			continue
		}
		got := final
		if got == keyUp {
			got = keyPageUp
		}
		if got == keyDown {
			got = keyPageDown
		}
		if got != tc.want {
			t.Errorf("%s: queued as %d, want %d", tc.name, got, tc.want)
		}
	}
}

// A plain arrow carries no parameter, and must not be read as a shifted one.
// The final bytes that are also digits are the case where getting this wrong
// makes every arrow a page.
func TestAPlainArrowIsNotShifted(t *testing.T) {
	for _, seq := range []string{"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D"} {
		_, shifted, _, ok := takeSequenceWithModifier([]byte(seq))
		if !ok {
			t.Errorf("%q: the sequence was not taken", seq)
			continue
		}
		if shifted {
			t.Errorf("%q: a plain arrow was read as shifted", seq)
		}
	}
}

// Only the shift is read. Another modifier is a key this reader has no use
// for, and treating it as a page would be a guess.
func TestAnotherModifierIsNotAPage(t *testing.T) {
	for _, seq := range []string{"\x1b[1;3A", "\x1b[1;5A", "\x1b[1;9A"} {
		_, shifted, _, ok := takeSequenceWithModifier([]byte(seq))
		if !ok {
			t.Errorf("%q: the sequence was not taken", seq)
			continue
		}
		if shifted {
			t.Errorf("%q: another modifier was read as a shift", seq)
		}
	}
}

func TestAMouseReportIsNotAKey(t *testing.T) {
	// The wheel reports the reader acts on are modified sequences too, and a
	// report read as a page would scroll on its own.
	for _, seq := range []string{"\x1b[<64;10;5M", "\x1b[<65;10;5M"} {
		if _, _, _, ok := takeSequenceWithModifier([]byte(seq)); ok {
			t.Errorf("%q: a mouse report was taken as a key", seq)
		}
	}
}

func TestAnIncompleteSequenceIsLeftAlone(t *testing.T) {
	// A sequence arriving in pieces must not be cut in half, or the tail is
	// read as keys and a bare escape ends the session.
	if _, _, _, ok := takeSequenceWithModifier([]byte("\x1b[1;")); ok {
		t.Error("an incomplete sequence was taken whole")
	}
}

// The page is the height of the pane, so the row a reader was on before is
// still on screen after.
func TestAPageMovesByTheHeightOfThePane(t *testing.T) {
	s := pagedSession(t, 40)

	s.page(-1)

	rows := 40 - headerRowCount - inputRowsBare - 1
	if s.pagingOffset() != rows {
		t.Errorf("a page back moved %d lines, want %d", s.pagingOffset(), rows)
	}
}

func TestAPageDownTowardTheBottomStops(t *testing.T) {
	s := pagedSession(t, 40)

	s.page(-1)
	s.page(1)

	if got := s.pagingOffset(); got != 0 {
		t.Errorf("a page back and forth left the view at %d, want the bottom", got)
	}
}

func TestAPageDownFromTheBottomDoesNothing(t *testing.T) {
	s := pagedSession(t, 40)

	s.page(1)

	if got := s.pagingOffset(); got != 0 {
		t.Errorf("a page down from the bottom moved to %d", got)
	}
}

// A short terminal still has a page, since a page of one line is better than a
// key that does nothing.
func TestAPageOnAShortTerminalIsOneLine(t *testing.T) {
	s := pagedSession(t, 6)

	s.page(-1)

	if got := s.pagingOffset(); got != 1 {
		t.Errorf("a page on a short terminal moved %d lines, want one", got)
	}
}

func TestTheHintRowNamesThePagingKeys(t *testing.T) {
	got := (hintState{overlay: hintCompose}).hints()
	joined := strings.Join(got, " ")

	if !strings.Contains(joined, "page") {
		t.Errorf("the hint row does not name the paging keys: %v", got)
	}
}

// pagedSession builds a session whose screen reports a height and whose
// conversation is long enough to page through.
//
// The history has to be there for a page to move: the renderer clamps the
// offset to what the pane can show, so a page in an empty conversation clamps to
// zero and the page looks as though it did nothing.
func pagedSession(t *testing.T, height int) *Session {
	t.Helper()
	s, _ := screenCapture(t)
	s.height = height
	s.width = 80
	conv := NewConversation()
	for i := 0; i < 200; i++ {
		conv.Record("a question", "an answer that is long enough to be a line")
	}
	sess := &Session{
		conv:      conv,
		mainConv:  conv,
		screen:    s,
		spinner:   NewSpinner(),
		windows:   newContextLength(),
		approvals: newApprovalState(),
		tools:     &toolSet{dir: t.TempDir()},
	}
	// The pane is drawn from the frame rather than from the conversation, so
	// the history has to be on the frame as well as in the conversation. A
	// session whose frame is empty has nothing to scroll through, whatever the
	// conversation holds.
	var reply []string
	for i := 0; i < 200; i++ {
		reply = append(reply, "a line of history")
	}
	sess.frame.Reply = reply
	return sess
}

// pagingOffset is the scroll offset read for a test, under the lock.
func (s *Session) pagingOffset() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scroll
}
