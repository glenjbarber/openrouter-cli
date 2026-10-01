package tui

import (
	"strings"
	"testing"
)

// The header yields before the prompt does. A frame that keeps the status bar
// and loses the prompt leaves a reader with no way to type a next message,
// which is the one row the interface cannot do without.
func TestShortFrameKeepsThePrompt(t *testing.T) {
	for _, height := range []int{1, 2, 3, 4, 5, 6, 7} {
		for _, width := range []int{2, 20, 80} {
			for _, input := range []string{"", "/connect", "something longer to type"} {
				f := Frame{Title: "openrouter-cli", Input: input}
				out := Render(f, height, width)
				if promptRow(out) < 0 {
					t.Errorf("height=%d width=%d: no prompt in %q",
						height, width, out)
				}
				if len(out) > height {
					t.Errorf("height=%d width=%d: %d rows", height, width, len(out))
				}
			}
		}
	}
}

// The header is given up from the top. The title goes first and the status bar
// goes last, so a reader who has lost rows has lost the heading rather than the
// figures, and a row that was kept still has everything below it.
func TestHeaderYieldsFromTheTop(t *testing.T) {
	for height := 1; height <= 12; height++ {
		joined := strings.Join(Render(Frame{Title: "TITLEROW", Input: "hi"}, height, 30), "\n")
		if strings.Contains(joined, "TITLEROW") && !strings.Contains(joined, "Provider") {
			t.Errorf("height=%d: the title was kept and the status bar dropped: %q",
				height, joined)
		}
		if strings.Contains(joined, ruleRune) && !strings.Contains(joined, "Provider") {
			t.Errorf("height=%d: the rule was kept and the status bar dropped: %q",
				height, joined)
		}
	}
}

// The frame is cut to the height rather than padded up to a minimum. A frame of
// rows the terminal does not have is written off the bottom, taking the prompt
// with it.
func TestFrameIsCutNotPadded(t *testing.T) {
	for height := 1; height <= 24; height++ {
		out := Render(Frame{Reply: []string{"a reply"}, Input: "b"}, height, 40)
		if len(out) > height {
			t.Errorf("height=%d: %d rows, want at most %d", height, len(out), height)
		}
	}
}
