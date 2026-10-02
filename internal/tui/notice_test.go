package tui

import (
	"strings"
	"testing"
)

// The notice is said about the line being composed, so it belongs beside the
// prompt rather than in the pane.

// The notice is drawn above the prompt and below the header, so a reader sees
// it where they pressed Tab.
func TestTheNoticeIsAboveThePrompt(t *testing.T) {
	rows := Render(Frame{
		Title:  "t",
		Notice: "nothing matches /zzz",
		Input:  "/zzz",
	}, 24, 50)

	at := -1
	for i, row := range rows {
		if strings.Contains(row, "nothing matches") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("the notice is not in the frame:\n%s", strings.Join(rows, "\n"))
	}
	promptAt := promptRow(rows)
	if promptAt < 0 {
		t.Fatalf("no prompt in the frame:\n%s", strings.Join(rows, "\n"))
	}
	if at > promptAt {
		t.Errorf("the notice is at row %d, below the prompt at %d", at, promptAt)
	}
}

// The notice is not written into the pane. A completion that matched nothing
// written among the replies becomes output the reader has to read back through
// the conversation to find.
func TestTheNoticeIsNotInThePane(t *testing.T) {
	rows := Render(Frame{
		Title:  "t",
		Notice: "nothing matches /zzz",
		Reply:  []string{"a reply"},
	}, 24, 50)

	inPane := false
	for _, row := range rows[:headerRowCount] {
		if strings.Contains(row, "nothing matches") {
			inPane = true
		}
	}
	if inPane {
		t.Errorf("the notice is in the header:\n%s", strings.Join(rows, "\n"))
	}
}

// The notice does not cost the prompt its row.
func TestTheNoticeYieldsToThePrompt(t *testing.T) {
	rows := Render(Frame{Title: "t", Notice: "nothing matches", Input: "hi"}, 10, 40)

	if promptRow(rows) < 0 {
		t.Errorf("a short terminal lost the prompt to the notice:\n%s",
			strings.Join(rows, "\n"))
	}
}

// A frame with no notice shows nothing extra, which is the ordinary case.
func TestNoNoticeCostsNoRow(t *testing.T) {
	with := len(Render(Frame{Title: "t", Input: "hi"}, 24, 40))
	without := len(Render(Frame{Title: "t", Input: "hi"}, 24, 40))
	if with != without {
		t.Errorf("two identical frames differ in height: %d and %d", with, without)
	}
}
