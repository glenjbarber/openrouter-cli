package tui

import (
	"strings"
	"testing"
)

// A newline inside a paste must not submit the line. Without the markers a
// paste is indistinguishable from typing, and the first newline would send half
// of it as a message.
func TestPasteDoesNotSubmitOnNewline(t *testing.T) {
	body := pasteStart + "first line\nsecond line\nthird line" + pasteEnd + "\r"
	le := NewLineEditor(strings.NewReader(body))

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	want := "first line\nsecond line\nthird line"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A paste and what is typed after it are one message, joined in the order they
// were entered.
func TestPasteJoinsWithTypedText(t *testing.T) {
	body := pasteStart + "pasted" + pasteEnd + "typed\r"
	le := NewLineEditor(strings.NewReader(body))

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "pasted\ntyped" {
		t.Errorf("got %q, want %q", got, "pasted\ntyped")
	}
}

// A terminal without bracketed paste enabled sends the text bare, and it must
// still work rather than be mangled.
func TestPasteWithoutMarkersStillWorks(t *testing.T) {
	le := NewLineEditor(strings.NewReader("just typing\r"))

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "just typing" {
		t.Errorf("got %q, want %q", got, "just typing")
	}
}

func TestStripPaste(t *testing.T) {
	if got := stripPaste(pasteStart + "x" + pasteEnd); got != "x" {
		t.Errorf("got %q, want %q", got, "x")
	}
	if got := stripPaste("x"); got != "x" {
		t.Errorf("got %q, want it unchanged", got)
	}
}

// A trailing break belongs to the paste, not to a request for an empty line.
func TestNormalisePasteDropsTrailingBreak(t *testing.T) {
	got := normalisePaste("one\ntwo\n")
	if len(got) != 2 {
		t.Errorf("got %q, want two lines", got)
	}
}

// A carriage return and newline together are one break, since that is what a
// terminal sends for a single newline.
func TestNormalisePasteCollapsesCRLF(t *testing.T) {
	got := normalisePaste("one\r\ntwo")
	if len(got) != 2 {
		t.Errorf("got %q, want two lines", got)
	}
}

func TestNormalisePasteEmpty(t *testing.T) {
	if got := normalisePaste("\n\n"); got != nil {
		t.Errorf("got %q, want nil", got)
	}
}

// A paste arriving in two blocks must be held until its closing marker appears,
// rather than the second half being read as keys.
func TestTakePasteHoldsPartial(t *testing.T) {
	buf := []byte(pasteStart + "half")
	if _, _, ok := takePaste(buf); ok {
		t.Error("an unterminated paste was taken, want it held")
	}
	if !mousePrefix(buf) {
		t.Log("note: the partial is not a mouse prefix, so it is held by the paste check")
	}
}

func TestTakePasteSplitsRemainder(t *testing.T) {
	buf := []byte(pasteStart + "body" + pasteEnd + "tail")
	text, rest, ok := takePaste(buf)
	if !ok {
		t.Fatal("takePaste did not find a complete paste")
	}
	if text != "body" {
		t.Errorf("text = %q, want %q", text, "body")
	}
	if string(rest) != "tail" {
		t.Errorf("rest = %q, want the bytes after the closing marker", string(rest))
	}
}

// A paste must be reported, or a large one reads as a frozen interface.
func TestPasteIsReported(t *testing.T) {
	body := pasteStart + "a\nb" + pasteEnd + "\r"
	le := NewLineEditor(strings.NewReader(body))

	var seen int
	le.OnPaste = func(lines []string) { seen = len(lines) }

	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if seen != 2 {
		t.Errorf("OnPaste reported %d lines, want 2", seen)
	}
}

// A nil callback must not panic, which a non-interactive reader relies on.
func TestPasteNilCallback(t *testing.T) {
	le := NewLineEditor(strings.NewReader(pasteStart + "x" + pasteEnd + "\r"))
	le.OnPaste = nil
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "x" {
		t.Errorf("got %q, want %q", got, "x")
	}
}

// The pasted lines are shown above the prompt, since a paste cannot fit on one
// row.
func TestRenderShowsPastedLines(t *testing.T) {
	lines := Render(Frame{Pasted: []string{"one", "two"}}, 12, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "one") || !strings.Contains(joined, "two") {
		t.Errorf("frame is missing the pasted lines:\n%s", joined)
	}
	if !strings.Contains(joined, "> ") {
		t.Error("the prompt is missing")
	}
}

// A large paste is cut to a fixed number of rows, since a paste of a thousand
// lines would otherwise fill the screen and push the status bar off it.
func TestRenderCapsPastedLines(t *testing.T) {
	pasted := make([]string, 50)
	for i := range pasted {
		pasted[i] = "line"
	}
	lines := Render(Frame{Pasted: pasted}, 30, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "more pasted lines") {
		t.Error("the overflow was not reported")
	}
}
