package tui

import (
	"strings"
	"testing"
)

// A line that has been sent is followed by a blank row, so the reply below it
// is not read as a continuation of the question. The row is in the pane rather
// than drawn around the reply, since a selection out of the pane has to yield
// the text with nothing in it.
func TestASentLineIsFollowedByABlankRow(t *testing.T) {
	s, _ := auditSession(t, auditStream)

	s.send(s.ctx, s.conv, "the question")

	if len(s.frame.Reply) < 3 {
		t.Fatalf("the pane holds %d rows, want the question, a blank and a reply: %q",
			len(s.frame.Reply), s.frame.Reply)
	}
	if got := s.frame.Reply[0]; got != "> the question" {
		t.Errorf("row 0 = %q, want the line that was sent", got)
	}
	if got := s.frame.Reply[1]; got != "" {
		t.Errorf("row 1 = %q, want a blank row under the line that was sent", got)
	}
	// The reply row opens with the rule that divides it from the question, so
	// it is compared with the reply as it is drawn rather than as it is said.
	if got := s.frame.Reply[2]; !strings.HasPrefix(got, withResponseRule("Hello")) {
		t.Errorf("row 2 = %q, want the reply under its rule", got)
	}
}

// The blank row is a row of the frame, not padding the renderer happened to
// add, so it survives the folding of the reply that follows it. The rows are
// read through the renderer rather than out of a capture, since the capture
// carries the escape sequences between rows rather than a newline per row.
func TestTheBlankRowIsDrawnUnderTheQuestion(t *testing.T) {
	s, _ := auditSession(t, auditStream)

	s.send(s.ctx, s.conv, "the question")

	s.mu.Lock()
	frame := s.frame
	s.mu.Unlock()
	rows := Render(frame, 24, 40)
	asked := -1
	for i, row := range rows {
		if strings.Contains(row, "> the question") {
			asked = i
			break
		}
	}
	if asked < 0 {
		t.Fatalf("the frame does not carry the line that was sent: %q", rows)
	}
	if asked+1 >= len(rows) {
		t.Fatalf("the question is the last row of the frame, so nothing follows it: %q", rows)
	}
	if next := rows[asked+1]; strings.TrimSpace(next) != "" {
		t.Errorf("the row below the question is %q, want a blank row", next)
	}
}

// The two refusals a turn can make are not followed by a blank row, since the
// reason they give belongs with the line that provoked it rather than under it.
func TestARefusedLineKeepsItsReasonBesideIt(t *testing.T) {
	s, _ := auditSession(t, auditStream)
	s.conv.SetModel("")

	s.send(s.ctx, s.conv, "the question")

	if len(s.frame.Reply) != 2 {
		t.Fatalf("the pane holds %d rows, want the line and the reason: %q",
			len(s.frame.Reply), s.frame.Reply)
	}
	if got := s.frame.Reply[1]; !strings.Contains(got, "no model is selected") {
		t.Errorf("row 1 = %q, want the reason the turn was refused", got)
	}
}
