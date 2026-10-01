package tui

import (
	"strings"
	"testing"
)

// The frame must never be taller than the terminal, at any width and at any
// number of pasted lines or queued messages. A frame that runs past the bottom
// pushes the prompt off the screen, and the reader loses the ability to type a
// next message.
//
// The two blocks are varied together, since a queue arrives while a paste may
// still be on the screen and the budget is spent on the paste first.
func TestFrameNeverExceedsHeight(t *testing.T) {
	var blocks []string
	for i := 0; i < 40; i++ {
		blocks = append(blocks, "a block line")
	}

	for _, height := range []int{1, 2, 5, 8, 12, 20, 30, 60} {
		for _, width := range []int{1, 2, 3, 5, 10, 40, 200} {
			for n := 0; n <= len(blocks); n++ {
				f := Frame{
					Title:  "a title",
					Reply:  []string{"a reply line", "another reply line"},
					Input:  "some input text",
					Pasted: blocks[:n],
				}
				lines := Render(f, height, width)
				if len(lines) > height {
					t.Errorf("height=%d width=%d pasted=%d: frame is %d rows",
						height, width, n, len(lines))
				}
				for q := 0; q <= len(blocks); q += 7 {
					f.Queued = blocks[:q]
					lines := Render(f, height, width)
					if len(lines) > height {
						t.Errorf("height=%d width=%d queued=%d: frame is %d rows",
							height, width, q, len(lines))
					}
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

// A block above the prompt must not cost the reader the prompt. The pane gives
// up rows rather than the frame overflowing, so the block and the prompt both
// remain. A reader who cannot see the prompt cannot answer the model, which is
// the same failure whether the rows were taken by a paste or by a queue.
func TestABlockAboveThePromptDoesNotCostThePrompt(t *testing.T) {
	var blocks []string
	for i := 0; i < 20; i++ {
		blocks = append(blocks, "a block line")
	}
	for _, tc := range []struct {
		name string
		f    Frame
	}{
		{"a paste", Frame{Pasted: blocks, Input: "hi"}},
		{"a queue", Frame{Queued: blocks, Input: "hi"}},
		{"a paste under a queue", Frame{Pasted: blocks, Queued: blocks, Input: "hi"}},
	} {
		lines := Render(tc.f, 24, 40)
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "> hi") {
			t.Errorf("%s pushed the prompt off the screen:\n%s", tc.name, joined)
		}
	}
}

// A queue too long for the rows it was given is cut and the remainder reported,
// since a queue cut without saying so reads as one that was never made.
func TestALongQueueIsCutAndReported(t *testing.T) {
	var queued []string
	for i := 0; i < 12; i++ {
		queued = append(queued, "a queued message")
	}
	joined := strings.Join(Render(Frame{Queued: queued, Input: "hi"}, 24, 40), "\n")
	if !strings.Contains(joined, "more queued messages") {
		t.Errorf("the queue was cut without saying so:\n%s", joined)
	}
	if strings.Count(joined, "a queued message") > maxBlockRows {
		t.Errorf("the queue was drawn past the rows it was given:\n%s", joined)
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
						Queued: pasted[:n/2],
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
