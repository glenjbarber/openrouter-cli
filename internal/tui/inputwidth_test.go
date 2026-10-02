package tui

import (
	"strings"
	"testing"
)

// The composed line is drawn across three quarters of the terminal rather than
// all of it, so that a long line does not carry the eye off the end of the
// screen and the conversation above it has a visible edge.

// The prompt is three quarters of the terminal on a terminal wide enough for it
// to mean something.
func TestThePromptIsThreeQuartersWide(t *testing.T) {
	for _, width := range []int{40, 60, 100, 200} {
		got := inputWidth(width)
		want := width * 3 / 4
		if got != want {
			t.Errorf("at %d columns the prompt is %d, want %d", width, got, want)
		}
	}
}

// A terminal too narrow for the fraction keeps the whole width, since three
// quarters of nothing shows nothing of what is being typed.
func TestANarrowTerminalKeepsTheWholeWidth(t *testing.T) {
	for _, width := range []int{1, 4, 7} {
		if got := inputWidth(width); got != width {
			t.Errorf("at %d columns the prompt is %d, want the whole width",
				width, got)
		}
	}
}

// A long line is cut at the prompt width rather than at the terminal width.
func TestALongLineStopsAtThePromptWidth(t *testing.T) {
	rows := Render(Frame{
		Title: "t",
		Input: strings.Repeat("x", 400),
	}, 24, 60)

	for _, row := range rows {
		if !strings.HasPrefix(row, "> ") {
			continue
		}
		if got := displayWidth(row); got > inputWidth(60) {
			t.Errorf("the prompt is %d columns, want at most %d: %q",
				got, inputWidth(60), row)
		}
		return
	}
	t.Fatal("no prompt in the frame")
}

// The tail is what is kept, since the reader is looking at the end of what they
// are writing and the end is the only part they cannot read from memory.
func TestALongLineKeepsItsTail(t *testing.T) {
	rows := Render(Frame{
		Title: "t",
		Input: "start" + strings.Repeat("x", 400) + "END",
	}, 24, 60)

	for _, row := range rows {
		if strings.HasPrefix(row, "> ") {
			if !strings.HasSuffix(row, "END") {
				t.Errorf("the prompt does not end where the line does: %q", row)
			}
			if !strings.HasPrefix(row, "> ...x") {
				t.Errorf("the head was kept rather than the tail: %q", row)
			}
			return
		}
	}
	t.Fatal("no prompt in the frame")
}

// A short line is not padded out to the prompt width, since a row of trailing
// spaces is padding a selection would carry.
func TestAShortLineIsNotPadded(t *testing.T) {
	rows := Render(Frame{Title: "t", Input: "hi"}, 24, 60)

	for _, row := range rows {
		if strings.HasPrefix(row, "> ") && row != "> hi" {
			t.Errorf("a short prompt was padded: %q", row)
		}
	}
}

// The rest of the frame is still drawn across the whole terminal, since the
// width is the composed line's alone and the conversation is not narrowed by it.
func TestTheRestOfTheFrameIsFullWidth(t *testing.T) {
	rows := Render(Frame{
		Title: "t",
		Reply: []string{strings.Repeat("y", 60)},
		Input: "hi",
	}, 24, 60)

	found := false
	for _, row := range rows {
		if strings.HasPrefix(row, strings.Repeat("y", 40)) {
			found = true
			if displayWidth(row) != 60 {
				t.Errorf("a reply is %d columns, want the whole terminal", displayWidth(row))
			}
		}
	}
	if !found {
		t.Error("the reply was not drawn across the width")
	}
}

// The caret sits inside the prompt rather than at the edge of the terminal,
// since the prompt is where the reader is typing.
func TestTheCaretIsInsideTheNarrowerPrompt(t *testing.T) {
	sc, read := screenCapture(t)
	rows := Render(Frame{Title: "t", Input: strings.Repeat("x", 400)}, 24, 60)

	sc.DrawFrame(rows, framePaint{twiddle: -1})
	got := read()

	at := strings.LastIndex(got, ";")
	if at < 0 {
		t.Fatalf("the caret was not placed: %q", got)
	}
	rest := got[at+1:]
	end := strings.IndexFunc(rest, func(r rune) bool { return r == 'H' })
	if end < 0 {
		t.Fatalf("the caret was not placed: %q", got)
	}
	col := rest[:end]
	if len(col) == 0 || col[0] < '0' || col[0] > '9' {
		t.Fatalf("the caret column is %q", col)
	}
	var n int
	for _, r := range col {
		n = n*10 + int(r-'0')
	}
	// The caret sits one column past the last character, which is where the
	// next one is written, so it is at most the prompt width plus that one.
	if n > inputWidth(60)+1 {
		t.Errorf("the caret is at column %d, past the prompt width of %d",
			n, inputWidth(60))
	}
}
