package tui

import (
	"strings"
	"testing"
	"unicode"
)

// A row must be measured in columns rather than in characters, since a
// character two columns wide counted as one leaves the row twice as wide as
// the pane and the terminal wraps it. Every row of every frame is checked
// against a column count taken here rather than in the test, so that the
// measurement under test is the one the renderer used.
func TestFrameRowsFitInColumns(t *testing.T) {
	replies := []string{
		strings.Repeat("wide text ", 20) + "你好世界你好世界你好世界你好世界你好世界你好世界",
		strings.Repeat("emoji ", 20) + "\U0001F600\U0001F680\U0001F600\U0001F680\U0001F600",
		strings.Repeat("accented ", 20) + "cafe\u0301 nai\u0308ve re\u0301sume\u0301",
		"- 你好世界你好世界你好世界你好世界你好世界你好世界 with a marker\n" +
			"  1. an ordered item that is wide 你好世界你好世界你好世界你好世界",
		"# 标题标题标题标题标题标题标题标题\n\ntext after a wide heading",
	}

	for height := 1; height <= 14; height++ {
		for width := 1; width <= 40; width++ {
			for _, reply := range replies {
				f := Frame{
					Title:  "a title 你好",
					Reply:  []string{reply},
					Input:  "an input 你好世界",
					Status: Status{Model: "some/model", Host: "a-host"},
				}
				for i, row := range Render(f, height, width) {
					if n := displayWidth(row); n > width {
						t.Errorf("h=%d w=%d row=%d is %d columns: %q",
							height, width, i, n, row)
					}
				}
			}
		}
	}
}

// Prose carrying wide characters is folded to the columns rather than to the
// characters, since a fold that counted characters would produce rows twice as
// wide as the pane.
func TestWrapFoldsToColumns(t *testing.T) {
	for _, width := range []int{4, 8, 12, 20} {
		rows := WrapBlock("你好 世界 你好 世界 你好 世界 你好 世界", width)
		for i, r := range rows {
			if n := displayWidth(r); n > width {
				t.Errorf("width=%d row=%d is %d columns: %q", width, i, n, r)
			}
		}
		joined := strings.Join(rows, "")
		if strings.Count(joined, "你好") != 4 {
			t.Errorf("width=%d: the fold lost characters: %q", width, joined)
		}
	}
}

// A word wider than the pane is moved whole onto a line of its own, and a
// word of wide characters is no different. A row that is over the edge is the
// one failure the fold is there to prevent, so the rule that causes it has to
// be stated here too.
func TestWrapKeepsAWholeWordTooWideToFit(t *testing.T) {
	word := strings.Repeat("你", 10)
	for _, width := range []int{4, 8, 12} {
		rows := WrapBlock("lead in "+word+" and a tail", width)
		if len(rows) < 2 {
			t.Fatalf("width=%d: the text was not folded: %q", width, rows)
		}
		found := false
		for _, r := range rows {
			if strings.Contains(r, word) {
				found = true
			}
		}
		if !found {
			t.Errorf("width=%d: the word was cut rather than moved whole: %q",
				width, rows)
		}
	}
}

// A cut must land between characters and never inside one. A character left
// half formed is copied out as the replacement character, and a two-column
// character cut in half leaves one column past the edge.
func TestTruncateCutsOnACharacterBoundary(t *testing.T) {
	for _, s := range []string{
		strings.Repeat("你", 20),
		strings.Repeat("a你b", 20),
		strings.Repeat("e\u0301", 20),
		strings.Repeat("x\U0001F600y", 20),
	} {
		for width := 1; width <= 30; width++ {
			got := truncate(s, width)
			if n := displayWidth(got); n > width {
				t.Errorf("truncate(%q, %d) = %q, which is %d columns",
					s, width, got, n)
			}
			for _, r := range got {
				if r == unicode.ReplacementChar {
					t.Errorf("truncate(%q, %d) = %q, which is split mid-character",
						s, width, got)
				}
			}
		}
	}
}

// The ellipsis is counted once. The text is shortened by the columns the
// ellipsis occupies, so a cut row is exactly the width asked for rather than
// three columns wider than it.
func TestTruncateCountsTheEllipsisOnce(t *testing.T) {
	for width := 4; width <= 20; width++ {
		got := truncate(strings.Repeat("w", 40), width)
		if n := displayWidth(got); n != width {
			t.Errorf("truncate at %d is %d columns, want %d: %q",
				width, n, width, got)
		}
		if !strings.HasSuffix(got, ellipsis) {
			t.Errorf("truncate at %d = %q, want the cut marked", width, got)
		}
	}
}

// A width too narrow for the ellipsis carries the text and nothing else,
// rather than three dots that tell the reader nothing about what they cannot
// see.
func TestTruncateBelowTheEllipsis(t *testing.T) {
	for width := 1; width <= 3; width++ {
		got := truncate(strings.Repeat("w", 40), width)
		if n := displayWidth(got); n != width {
			t.Errorf("truncate at %d is %d columns: %q", width, n, got)
		}
		if strings.Contains(got, ellipsis) {
			t.Errorf("truncate at %d = %q, want no ellipsis", width, got)
		}
	}
}

// A combining mark belongs to the character before it. A cut that separated
// them would leave a mark at the head of a row, drawn over whatever came
// there next, and a row is plain text that a reader copies.
func TestCutKeepsAMarkWithItsCharacter(t *testing.T) {
	// Five characters, each written with its accent as a mark of its own, and
	// each base letter appearing in one pair only, so a base in the result
	// that is not followed by its mark is a mark that was cut away.
	pairs := []string{"a\u0300", "e\u0301", "i\u0308", "o\u0302", "u\u0303"}
	marked := strings.Join(pairs, "")

	for width := 1; width <= 16; width++ {
		for _, cut := range []func(string, int) string{truncate, tail} {
			got := cut(marked, width)
			for _, p := range pairs {
				if strings.ContainsRune(got, rune(p[0])) && !strings.Contains(got, p) {
					t.Errorf("width=%d: %q lost the mark belonging to %q",
						width, got, string(p[0]))
				}
			}
		}
	}
}

// The tail keeps its own width. A body that does not fit shows the end of it,
// which is the part still being composed.
func TestTailFitsItsWidth(t *testing.T) {
	for _, s := range []string{
		strings.Repeat("w", 40),
		strings.Repeat("你", 40),
		strings.Repeat("e\u0301", 40),
	} {
		for n := 1; n <= 30; n++ {
			got := tail(s, n)
			if w := displayWidth(got); w > n {
				t.Errorf("tail(%q, %d) = %q, which is %d columns", s, n, got, w)
			}
		}
	}
}

// The status bar is measured in columns, so a value carrying wide characters
// does not cost the bar more room than it takes. Measured in bytes it would
// drop fields that would have fitted, since two bytes were read for one
// column.
func TestStatusBarKeepsFieldsThatFitInColumns(t *testing.T) {
	s := Status{Model: "你好世界", State: "idle", Host: "你好-host"}
	for _, width := range []int{10, 20, 30, 40, 60, 80, 120} {
		for name, line := range map[string]string{
			"top":   TopLine(s, width),
			"input": InputLine(s, width),
		} {
			if n := displayWidth(line); n > width {
				t.Errorf("width=%d: the %s bar is %d columns: %q", width, name, n, line)
			}
		}
	}
	// Wide enough for the model and the host, so each must be on its bar.
	if line := InputLine(s, 120); !strings.Contains(line, "你好世界") {
		t.Errorf("a wide bar dropped the model, which fitted: %q", line)
	}
	if line := TopLine(s, 120); !strings.Contains(line, "你好-host") {
		t.Errorf("a wide bar dropped the host, which fitted: %q", line)
	}
}

// The width of a character is what a terminal gives it: two for a wide form,
// none for a mark. These are the figures the rest of the arithmetic rests on,
// so they are stated here rather than assumed.
func TestRuneColumns(t *testing.T) {
	tests := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{' ', 1},
		{0x7e, 1},
		{0x2500, 1},  // box drawing.
		{0x280b, 1},  // braille, used by the twiddle.
		{0x4e00, 2},  // CJK unified ideograph.
		{0x3042, 2},  // Hiragana.
		{0xac00, 2},  // Hangul syllable.
		{0x1f600, 2}, // emoji.
		{0x0301, 0},  // combining acute.
		{0x20dd, 0},  // enclosing circle.
		{0x200d, 0},  // zero width joiner.
		{0xfe0f, 0},  // variation selector.
		{0x00e9, 1},  // precomposed acute.
	}
	for _, tt := range tests {
		if got := runeColumns(tt.r); got != tt.want {
			t.Errorf("runeColumns(%U) = %d, want %d", tt.r, got, tt.want)
		}
	}
}

// The caret goes where the next character will appear. A column counted in
// bytes puts it to the right of the end of the input on any line carrying a
// multibyte character.
func TestCaretColumnFollowsTheInput(t *testing.T) {
	for _, input := range []string{"hi", "hé", "你好", "é"} {
		s, read := captureScreen(t, false)
		s.height, s.width = 24, 40
		s.Draw(Render(Frame{Input: input}, 24, 40))
		out := read()

		place := caretPlace(out)
		if place == "" {
			t.Fatalf("input=%q: the caret was not placed", input)
		}
		want := displayWidth("> "+input) + 1
		if got := caretColumn(place); got != want {
			t.Errorf("input=%q: the caret is at column %d, want %d",
				input, got, want)
		}
	}
}
