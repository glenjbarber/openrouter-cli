package tui

import (
	"strings"
	"testing"
)

// promptRow returns the index of the prompt row, which is the last row the
// client marks with the prompt marker. The row is found by content rather than
// by offset, since the header grows and the division comes and goes with the
// height of the terminal.
func promptRow(lines []string) int {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "> ") {
			return i
		}
	}
	return -1
}

// A key is named only when it does something in the state being described. A
// row that promised a key which did nothing would be worse than no row, since
// a reader would press it and conclude the client had hung.
func TestHintRowNamesOnlyKeysThatAct(t *testing.T) {
	tests := []struct {
		name string
		st   hintState
		want []string
	}{
		{
			name: "an idle prompt names the key that sends and the key that breaks",
			st:   hintState{},
			want: []string{"Send [enter]", "[ctrl]+j newline", "[shift]+arrows page", "[ctrl]+b n/p window"},
		},
		{
			name: "history alone adds nothing, since it is no longer named",
			st:   hintState{history: true},
			want: []string{"Send [enter]", "[ctrl]+j newline", "[shift]+arrows page", "[ctrl]+b n/p window"},
		},
		{
			// Enter queues rather than sends while a model is working, and
			// escape stops the model with whatever is in hand, so the row
			// names what the keys do in that state rather than what they do
			// on an idle prompt.
			name: "a model working renames enter and names escape",
			st:   hintState{busy: true},
			want: []string{"Enter queue", "Esc stop and send"},
		},
		{
			name: "the wheel alone adds nothing, since it is no longer named",
			st:   hintState{mouse: true},
			want: []string{"Send [enter]", "[ctrl]+j newline", "[shift]+arrows page", "[ctrl]+b n/p window"},
		},
		{
			name: "the filter names completion, choosing and closing",
			st:   hintState{overlay: hintListing, history: true, mouse: true},
			want: []string{"Tab cycle", "Enter choose", "Esc close"},
		},
		{
			name: "the search names jumping and closing",
			st:   hintState{overlay: hintSearch, busy: true},
			want: []string{"Enter jump to match", "Esc close"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.st.hints()
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("hints = %q, want %q", got, tt.want)
			}
		})
	}
}

// A half a phrase names a key and not what the key does, which is the one
// thing the row exists to say. Entries are therefore dropped whole rather than
// cut when the row is too narrow.
func TestHintLineDropsWholeEntriesRatherThanCutting(t *testing.T) {
	hints := hintState{history: true, busy: true, mouse: true}.hints()

	for width := 1; width <= 60; width++ {
		line := hintLine(hints, width)
		if n := len([]rune(line)); n > width {
			t.Errorf("width=%d: row is %d columns: %q", width, n, line)
		}
		if line == "" {
			continue
		}
		// Every entry shown must be one of the entries offered, in order and
		// unbroken. A cut entry would end in the ellipsis of a truncation
		// rather than in the whole of a name.
		shown := strings.Split(line, hintSep)
		for i, s := range shown {
			if i >= len(hints) {
				t.Fatalf("width=%d: more entries shown than offered: %q", width, shown)
			}
			if s != hints[i] {
				t.Errorf("width=%d: entry %d = %q, want %q", width, i, s, hints[i])
			}
		}
	}
}

// A row too narrow for even one entry is left empty. An entry that does not
// fit is not a hint, it is a fragment.
func TestHintLineIsEmptyWhenNothingFits(t *testing.T) {
	hints := hintState{}.hints()
	if got := hintLine(hints, 0); got != "" {
		t.Errorf("width=0: row = %q, want empty", got)
	}
	if got := hintLine(hints, -3); got != "" {
		t.Errorf("width=-3: row = %q, want empty", got)
	}
	if got := hintLine(hints, len(hints[0])-1); got != "" {
		t.Errorf("row = %q, want empty when the first entry does not fit", got)
	}
	if got := hintLine(nil, 40); got != "" {
		t.Errorf("no entries: row = %q, want empty", got)
	}
}

// The row sits directly above the prompt, so that it reads as a caption for the
// input line and the prompt stays the last row of the block. The caret is
// placed on the last row, so a row below the prompt would move it.
func TestHintRowSitsAboveThePrompt(t *testing.T) {
	f := Frame{Input: "a message", Hints: hintState{history: true}.hints()}
	lines := Render(f, 24, 40)

	row := promptRow(lines)
	if row < 1 {
		t.Fatalf("no prompt row, or none with a row above it:\n%q", lines)
	}
	if above := lines[row-1]; !strings.Contains(above, "Send [enter]") {
		t.Errorf("row above the prompt = %q, want the hint row", above)
	}
	if lines[row] != "> a message" {
		t.Errorf("prompt row = %q", lines[row])
	}
}

// A hint row costs its own row and nothing else. The prompt must survive
// wherever it survived before, since a reader who cannot type a next message
// is worse off than one who does not know about a key, and the frame must
// stay inside the terminal on both axes at once.
func TestHintRowCostsOnlyItsOwnRow(t *testing.T) {
	var pasted []string
	for i := 0; i < 12; i++ {
		pasted = append(pasted, "pasted line")
	}
	reply := []string{"a reply", "a reply that wraps when the pane is narrow"}
	hints := hintState{history: true, busy: true, mouse: true}.hints()

	for _, width := range []int{1, 2, 4, 12, 40, 120} {
		for _, height := range []int{1, 3, 8, 12, 24, 60} {
			for _, scroll := range []int{0, 3, 1 << 20} {
				for n := range pasted {
					base := Frame{Title: "t", Reply: reply, Input: "some input",
						Pasted: pasted[:n], Scroll: scroll}
					with := base
					with.Hints = hints

					a := Render(base, height, width)
					b := Render(with, height, width)

					if len(b) > height {
						t.Fatalf("h=%d w=%d scroll=%d pasted=%d: %d rows",
							height, width, scroll, n, len(b))
					}
					for i, l := range b {
						if c := len([]rune(l)); c > width {
							t.Fatalf("h=%d w=%d scroll=%d pasted=%d row %d is %d: %q",
								height, width, scroll, n, i, c, l)
						}
					}
					if d := len(b) - len(a); d > 1 || d < 0 {
						t.Fatalf("h=%d w=%d scroll=%d pasted=%d: frame grew by %d rows",
							height, width, scroll, n, d)
					}
					if promptRow(a) >= 0 && promptRow(b) < 0 {
						t.Fatalf("h=%d w=%d scroll=%d pasted=%d: the hint row cost the prompt",
							height, width, scroll, n)
					}
				}
			}
		}
	}
}

// A paste is reported before the hint is named, since a paste the reader
// cannot see reads as a lost paste while a missing hint costs nothing.
func TestHintRowYieldsToALandedPaste(t *testing.T) {
	pasted := []string{"one", "two", "three", "four", "five", "six", "seven"}
	f := Frame{Pasted: pasted, Input: "hi", Hints: hintState{}.hints()}
	lines := Render(f, 24, 40)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "pasted") && !strings.Contains(joined, "one") {
		t.Fatalf("the paste is not shown at all:\n%q", lines)
	}
	if !strings.Contains(joined, "more pasted lines") {
		t.Errorf("the cut paste is not reported:\n%q", lines)
	}
	if !strings.Contains(joined, "Send [enter]") {
		t.Errorf("the hint row is missing where there was room for it:\n%q", lines)
	}
	if promptRow(lines) < 0 {
		t.Errorf("the prompt is missing:\n%q", lines)
	}
}

// A terminal too short to hold the hint and the prompt holds the prompt. The
// row names keys, and a session with no way to type a next message is not one
// the reader can carry on in.
func TestHintRowIsDroppedBeforeThePrompt(t *testing.T) {
	found := 0
	for height := 8; height <= 40; height++ {
		base := Frame{Input: "hi"}
		if promptRow(Render(base, height, 40)) < 0 {
			continue
		}
		found++
		with := base
		with.Hints = hintState{history: true}.hints()
		if got := promptRow(Render(with, height, 40)); got < 0 {
			t.Errorf("height=%d: the prompt was pushed off by the hint row", height)
		}
	}
	if found == 0 {
		t.Fatal("no height kept the prompt, so the test proves nothing")
	}
}
