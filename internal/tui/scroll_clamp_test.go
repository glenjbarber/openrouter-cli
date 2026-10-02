package tui

import "testing"

// The offset the renderer settled on is reported, since a session holding an
// offset the pane cannot show would need a notch per line to come back from.
func TestRenderReportsTheOffsetItDrewAt(t *testing.T) {
	reply := make([]string, 400)
	for i := range reply {
		reply[i] = "line"
	}
	rows, drawn, _ := render(Frame{Reply: reply, Scroll: 1000}, 24, 80)
	if drawn >= 1000 {
		t.Errorf("offset reported as %d, want it clamped below what was asked for", drawn)
	}
	if drawn <= 0 {
		t.Errorf("offset reported as %d, want a full pane of history to remain", drawn)
	}
	if len(rows) > 24 {
		t.Errorf("%d rows drawn into a 24 row terminal", len(rows))
	}
}

// Scrolling further up than there is history used to leave the session holding
// an offset the pane could not show, so a reader who scrolled up a long way
// had a notch to come down for every line they had scrolled past, including
// the lines that did not exist. The clamp the renderer applies is adopted back
// into the session, so the offset is never larger than the history warrants.
func TestAnOffsetThePaneCannotShowIsAdoptedBack(t *testing.T) {
	s, _ := auditSession(t, "")
	reply := make([]string, 120)
	for i := range reply {
		reply[i] = "line"
	}
	s.mu.Lock()
	s.frame.Reply = reply
	s.mu.Unlock()

	for range 200 {
		s.scrollBy(mouseUp)
	}
	s.mu.Lock()
	wentTooFar := s.scroll
	s.mu.Unlock()
	if wentTooFar < 100 {
		t.Fatalf("scroll = %d after scrolling up, want the setup to have gone past the top", wentTooFar)
	}

	s.paintNow()

	s.mu.Lock()
	after := s.scroll
	s.mu.Unlock()
	if after >= wentTooFar {
		t.Fatalf("scroll = %d after painting, want it adopted back from %d", after, wentTooFar)
	}
	if after > len(reply) {
		t.Errorf("scroll = %d, want no more than the %d lines there are", after, len(reply))
	}

	// Coming down costs one notch per three lines of history, and no more.
	notches := 0
	for {
		s.scrollBy(mouseDown)
		notches++
		if s.scrollAtBottom() {
			break
		}
		if notches > 1000 {
			t.Fatalf("still not at the bottom after %d notches", notches)
		}
	}
	want := (after + scrollStep - 1) / scrollStep
	if notches != want {
		t.Errorf("took %d notches to come back down from %d, want %d", notches, after, want)
	}
}
