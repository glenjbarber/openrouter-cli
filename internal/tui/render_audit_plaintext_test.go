package tui

import (
	"strings"
	"testing"
	"unicode"
)

// The frame is plain text. A selection taken out of the pane is copied as
// whatever is on the screen, so a control byte inside a row would be copied
// out with the prose around it and would act on whatever the reader pasted it
// into. It would also act on this terminal, since an escape written into a row
// moves the cursor and restyles the rest of the frame. Nothing in a row may
// therefore be a control character: the sequences the interface uses belong to
// the terminal and are written around the frame, never into it.
func TestFrameCarriesNoControlByte(t *testing.T) {
	replies := []string{
		"plain prose with no markup at all",
		"# a heading\n\n- a list item\n\n**bold** and *italic* and `code`",
		"```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```",
		"windows line endings\r\nand a second line\r\n",
		"a bare carriage return\rafter which the rest of the row is overtyped",
		"an escape \x1b[31mthat would recolour the frame\x1b[0m and a cursor move",
		"a tab\tinside one line",
		"emoji and wide text: 你好世界 \U0001F600 end",
		"combining: éèê end",
		strings.Repeat("a long paragraph that has to fold ", 12),
	}

	for height := 1; height <= 14; height++ {
		for width := 1; width <= 80; width++ {
			for _, reply := range replies {
				f := Frame{
					Title:  "a title\r\nwith a break",
					Reply:  []string{reply},
					Input:  "some input text\rwith a break",
					Status: Status{Model: "some/model", Host: "a-host"},
					Hint:   "a hint with a marker * and a hash #",
				}
				for i, row := range Render(f, height, width) {
					for _, r := range row {
						if r == '\t' {
							continue
						}
						if unicode.IsControl(r) {
							t.Errorf("h=%d w=%d row=%d carries %U: %q",
								height, width, i, r, row)
							break
						}
					}
				}
			}
		}
	}
}

// The sequences the client emits are written around the frame by Draw. Render
// adds none of its own, so a row it returns holds nothing but text.
func TestRenderAddsNoSequenceOfItsOwn(t *testing.T) {
	out := Render(Frame{
		Title:  "a title",
		Reply:  []string{"a reply"},
		Input:  "an input",
		Status: Status{Model: "some/model", Host: "a-host"},
	}, 24, 60)
	for _, row := range out {
		if strings.ContainsRune(row, 0x1b) {
			t.Errorf("row carries an escape: %q", row)
		}
	}
}

// A rule is a box-drawing character, which is three bytes and one column. A
// frame that counted it in bytes would report it three times too wide and wrap
// onto the next row, so the rule is checked in columns and in bytes.
func TestRuleIsOneColumnPerCharacter(t *testing.T) {
	for width := 1; width <= 40; width++ {
		got := rule(width)
		if len([]rune(got)) != width {
			t.Errorf("rule(%d) is %d columns, want %d",
				width, len([]rune(got)), width)
		}
		if len(got) != width*len(ruleRune) {
			t.Errorf("rule(%d) is %d bytes, want %d",
				width, len(got), width*len(ruleRune))
		}
	}
}
