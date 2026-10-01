package tui

import (
	"strings"
	"testing"
)

// The frame must never be taller than the terminal, at any width and at any
// number of pasted lines. A frame that runs past the bottom pushes the prompt
// off the screen, and the reader loses the ability to type a next message.
func TestFrameNeverExceedsHeight(t *testing.T) {
	var pasted []string
	for i := 0; i < 40; i++ {
		pasted = append(pasted, "pasted line")
	}

	for _, height := range []int{1, 2, 5, 8, 12, 20, 30, 60} {
		for _, width := range []int{1, 2, 3, 5, 10, 40, 200} {
			for n := 0; n <= len(pasted); n++ {
				f := Frame{
					Title:  "a title",
					Reply:  []string{"a reply line", "another reply line"},
					Input:  "some input text",
					Pasted: pasted[:n],
				}
				lines := Render(f, height, width)
				if len(lines) > height {
					t.Errorf("height=%d width=%d pasted=%d: frame is %d rows",
						height, width, n, len(lines))
				}
			}
		}
	}
}

// No row may be wider than the terminal. A row past the edge wraps, and a
// wrapped row pushes everything below it down, so the layout comes apart.
func TestNoRowExceedsWidth(t *testing.T) {
	for _, width := range []int{1, 2, 3, 4, 5, 6, 8, 10, 40} {
		for _, height := range []int{8, 20, 40} {
			f := Frame{
				Title:  "a title",
				Reply:  []string{"a reply line"},
				Input:  "some input text that is fairly long",
				Pasted: []string{"pasted", "lines"},
				Status: Status{Model: "some/model", Host: "a-host"},
			}
			for i, l := range Render(f, height, width) {
				if n := len([]rune(l)); n > width {
					t.Errorf("width=%d height=%d row=%d is %d wide: %q",
						width, height, i, n, l)
				}
			}
		}
	}
}

// The prompt row must fit a terminal too narrow to hold the marker and the text
// together. A reader on a narrow terminal should see the tail of what is being
// typed rather than the marker alone.
func TestPromptFitsNarrowTerminal(t *testing.T) {
	for _, width := range []int{1, 2, 3, 4, 5} {
		f := Frame{Input: "hello there"}
		lines := Render(f, 20, width)
		var prompt string
		for _, l := range lines {
			if strings.HasPrefix(l, ">") {
				prompt = l
			}
		}
		if len([]rune(prompt)) > width {
			t.Errorf("width=%d: prompt row is %d wide: %q",
				width, len([]rune(prompt)), prompt)
		}
	}
}

// A pasted paste must not cost the reader the prompt. The pane gives up rows
// rather than the frame overflowing, so a paste and a prompt both remain.
func TestPasteDoesNotCostThePrompt(t *testing.T) {
	var pasted []string
	for i := 0; i < 20; i++ {
		pasted = append(pasted, "pasted line")
	}
	f := Frame{Pasted: pasted, Input: "hi"}
	lines := Render(f, 24, 40)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "> hi") {
		t.Errorf("the prompt was pushed off the screen:\n%s", joined)
	}
}

// The frame must hold its shape under every combination of width, height,
// scroll offset and paste length. The rows the client writes are bounded on
// both axes at once, so the two bounds are checked together rather than one
// at a time.
func TestFrameShapeHoldsAtEverySize(t *testing.T) {
	var pasted []string
	for i := 0; i < 12; i++ {
		pasted = append(pasted, "pasted line")
	}
	reply := []string{"a reply", "a reply that wraps when the pane is narrow"}

	for _, width := range []int{1, 2, 3, 4, 6, 12, 40, 120} {
		for _, height := range []int{1, 2, 3, 5, 8, 12, 24, 60} {
			for _, scroll := range []int{0, 1, 7, 100, 1 << 30} {
				for n := range pasted {
					f := Frame{
						Title:  "a title",
						Reply:  reply,
						Input:  "some input",
						Pasted: pasted[:n],
						Scroll: scroll,
					}
					lines := Render(f, height, width)
					if len(lines) > height {
						t.Fatalf("h=%d w=%d scroll=%d pasted=%d: %d rows",
							height, width, scroll, n, len(lines))
					}
					for i, l := range lines {
						if c := len([]rune(l)); c > width {
							t.Fatalf("h=%d w=%d scroll=%d pasted=%d row %d is %d: %q",
								height, width, scroll, n, i, c, l)
						}
					}
				}
			}
		}
	}
}

// The status bar is drawn from box-drawing and other multibyte characters, so
// its width must be counted in columns rather than in bytes. A rule counted in
// bytes is three times too wide, and a status bar that is three times too wide
// wraps and pushes the rest of the frame down.
func TestStatusBarWidthIsCountedInColumns(t *testing.T) {
	s := Status{Model: "some/model", Context: "10%", Host: "a-host"}
	for _, width := range []int{4, 7, 10, 20, 40, 80} {
		line := StatusLine(s, width)
		if n := len([]rune(line)); n > width {
			t.Errorf("width=%d: status line is %d columns: %q", width, n, line)
		}
	}
}
