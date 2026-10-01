package tui

import (
	"strings"
	"testing"
)

// A pasted block takes only what is left once the prompt has been accounted
// for. Drawing the block in full put the prompt off the bottom of a short
// terminal, which leaves the reader no way to write a next message, and a
// paste of any size could do it.
func TestPasteNeverCostsThePromptAtAnyHeight(t *testing.T) {
	var pasted []string
	for i := 0; i < 30; i++ {
		pasted = append(pasted, "pasted line")
	}

	for height := 1; height <= 16; height++ {
		for width := 1; width <= 6; width++ {
			for n := 0; n <= len(pasted); n++ {
				f := Frame{
					Title:  "a title",
					Reply:  []string{"a reply line"},
					Input:  "some input text",
					Pasted: pasted[:n],
				}
				out := Render(f, height, width)
				if len(out) > height {
					t.Errorf("h=%d w=%d n=%d: frame is %d rows",
						height, width, n, len(out))
				}
				// The marker is looked for by its first character, since a
				// terminal of one column can show nothing else of the row.
				found := false
				for _, l := range out {
					if strings.HasPrefix(l, ">") {
						found = true
					}
				}
				if !found {
					t.Errorf("h=%d w=%d n=%d: no prompt in %q",
						height, width, n, out)
				}
			}
		}
	}
}

// A paste cut to fit the room left is reported, since a block cut without
// saying so reads as a paste that arrived short. A block that fits whole is
// not reported, since there is nothing to report.
func TestPasteOverflowIsReported(t *testing.T) {
	tests := []struct {
		pasted int
		height int
		want   bool
	}{
		{pasted: 3, height: 24, want: false},
		{pasted: 8, height: 24, want: true},
		{pasted: 0, height: 24, want: false},
		{pasted: 40, height: 24, want: true},
	}
	for _, tt := range tests {
		pasted := make([]string, tt.pasted)
		for i := range pasted {
			pasted[i] = "pasted line"
		}
		joined := strings.Join(Render(Frame{Pasted: pasted, Input: "hi"},
			tt.height, 60), "\n")
		if got := strings.Contains(joined, "more pasted lines"); got != tt.want {
			t.Errorf("pasted=%d height=%d: notice = %v, want %v\n%s",
				tt.pasted, tt.height, got, tt.want, joined)
		}
	}
}

// The rows a paste is given come out of the budget, so a block is cut rather
// than drawn past what the terminal has. The layout is the arithmetic behind
// that, and it is checked on its own so that a change to the constants shows
// up here rather than as a frame that no longer fits.
func TestPasteLayoutHonoursItsBudget(t *testing.T) {
	for budget := 0; budget <= 10; budget++ {
		for n := 0; n <= 40; n++ {
			shown, notice := blockLayout(n, budget)
			rows := shown
			if notice {
				rows++
			}
			if rows > budget {
				t.Errorf("budget=%d n=%d: %d rows drawn", budget, n, rows)
			}
			if shown > n {
				t.Errorf("budget=%d n=%d: %d lines shown, more than were pasted",
					budget, n, shown)
			}
			if n > 0 && budget > 0 && shown == 0 && !notice {
				t.Errorf("budget=%d n=%d: a block was neither shown nor reported",
					budget, n)
			}
		}
	}
}

// A paste that fits shows every line, and one that does not is cut to the
// maximum rather than to whatever the terminal happens to have.
func TestPasteLayoutKeepsTheFirstLines(t *testing.T) {
	shown, notice := blockLayout(8, 6)
	if shown != maxBlockRows || !notice {
		t.Errorf("shown = %d, notice = %v, want %d and true",
			shown, notice, maxBlockRows)
	}
	if shown, notice := blockLayout(3, 6); shown != 3 || notice {
		t.Errorf("shown = %d, notice = %v, want 3 and false", shown, notice)
	}
}
