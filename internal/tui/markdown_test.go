package tui

import (
	"strings"
	"testing"
)

// fence is a block marker, spelled out rather than written into a fixture. A
// backtick cannot sit inside the raw string a fixture would otherwise need, and
// a marker written as an ordinary string reads as noise.
const fence = "```"

// wantRows compares the rows a render produced with the rows it should have
// produced.
func wantRows(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// A construct is rendered by what it leaves, so this table states what each one
// is allowed to change. A marker that is not a construct is left exactly as it
// was written, since taking one for a construct would alter a sentence about
// sizes, a tag, or the name of a variable.
func TestRenderMarkdownLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "heading level one", in: "# Title", want: "# Title"},
		{name: "heading level two", in: "## Title", want: "## Title"},
		{name: "heading level six", in: "###### Title", want: "###### Title"},
		{name: "seven hashes are prose", in: "####### Title", want: "####### Title"},
		{name: "hash without a space is a tag", in: "#hashtag", want: "#hashtag"},
		{name: "issue reference is prose", in: "see issue #42 for details", want: "see issue #42 for details"},
		{name: "a lone hash is prose", in: "#", want: "#"},
		{name: "nested heading keeps its indent", in: "  ## Title", want: "  ## Title"},

		{name: "dash bullet", in: "- first", want: "- first"},
		{name: "star bullet", in: "* first", want: "* first"},
		{name: "plus bullet", in: "+ first", want: "+ first"},
		{name: "ordered with a dot", in: "1. first", want: "1. first"},
		{name: "ordered with a bracket", in: "1) first", want: "1) first"},
		{name: "ordered with two figures", in: "12. first", want: "12. first"},
		{name: "nested item keeps its indent", in: "  - child", want: "  - child"},
		{name: "minus before a figure is prose", in: "-5 degrees outside", want: "-5 degrees outside"},
		{name: "figure and point is not an item", in: "1.5 is not a list", want: "1.5 is not a list"},
		{name: "a deeper indent loses the indent", in: "    - too deep", want: "- too deep"},
		{name: "a lone marker is prose", in: "-", want: "-"},

		{name: "strong", in: "this is **strong** text", want: "this is strong text"},
		{name: "emphasis", in: "this is *emphasised* text", want: "this is emphasised text"},
		{name: "underscored strong", in: "this is __strong__ text", want: "this is strong text"},
		{name: "underscored emphasis", in: "this is _emphasised_ text", want: "this is emphasised text"},
		{name: "nested emphasis", in: "**bold with *emphasis* inside**", want: "bold with emphasis inside"},
		{name: "a span on each side", in: "**a** and **b**", want: "a and b"},
		{name: "a triple run", in: "***both***", want: "both"},
		{name: "punctuation after a span", in: "**strong**, then prose", want: "strong, then prose"},
		{name: "markers inside a name survive", in: "the OPENROUTER_API_KEY value", want: "the OPENROUTER_API_KEY value"},
		{name: "a name beside a span", in: "snake_case and _emphasis_", want: "snake_case and emphasis"},
		{name: "multiplication survives", in: "2 * 3 * 4 = 24", want: "2 * 3 * 4 = 24"},
		{name: "a lone asterisk survives", in: "the * character", want: "the * character"},
		{name: "a spaced marker survives", in: "unmatched ** marker", want: "unmatched ** marker"},
		{name: "a long run is not a delimiter", in: "**** four stars", want: "**** four stars"},
		{name: "an escaped marker survives", in: `\*escaped\*`, want: `\*escaped\*`},

		{name: "code keeps its backticks", in: "run `go build ./...` now", want: "run `go build ./...` now"},
		{name: "code keeps a marker inside it", in: "the `*` marker", want: "the `*` marker"},
		{name: "two code spans", in: "`a` and `**`", want: "`a` and `**`"},
		{name: "an unterminated span is a backtick", in: "a lone ` tick", want: "a lone ` tick"},

		{name: "emphasis in a list item", in: "- a **bold** item", want: "- a bold item"},
		{name: "code in a list item", in: "1. run `go test`", want: "1. run `go test`"},
		{name: "emphasis in a heading", in: "## A **bold** heading", want: "## A bold heading"},
		{name: "a closing run stays", in: "## Title ##", want: "## Title ##"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantRows(t, renderMarkdown(c.in, 60), []string{c.want})
		})
	}
}

// A construct whose text is too wide for the pane keeps its marker on the
// first row and indents the rest under it, so that the rows still read as one
// construct rather than as several.
func TestRenderMarkdownFoldsUnderMarker(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{
			name:  "a list item that folds",
			in:    "- a list item that has to fold onto a second row",
			width: 20,
			want: []string{
				"- a list item that",
				"  has to fold onto a",
				"  second row",
			},
		},
		{
			name:  "a heading that folds",
			in:    "## A heading long enough that it must fold",
			width: 20,
			want: []string{
				"## A heading long",
				"   enough that it",
				"   must fold",
			},
		},
		{
			name:  "an ordered item that folds",
			in:    "12. an ordered item that has to fold",
			width: 16,
			want: []string{
				"12. an ordered",
				"    item that",
				"    has to fold",
			},
		},
		{
			name:  "a marker wider than the pane takes a row of its own",
			in:    "12. a long item",
			width: 3,
			want: []string{
				"12.",
				"a",
				"long",
				"item",
			},
		},
		{
			name:  "a folded item keeps every word",
			in:    "- **alpha** beta gamma delta epsilon zeta eta",
			width: 12,
			want: []string{
				"- alpha beta",
				"  gamma",
				"  delta",
				"  epsilon",
				"  zeta eta",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantRows(t, renderMarkdown(c.in, c.width), c.want)
		})
	}
}

// A fenced block is code, and a marker inside it is the character the code
// holds rather than a construct. Stripping an asterisk out of a glob would
// change what the reader was shown.
func TestRenderMarkdownLeavesFencedBlockAlone(t *testing.T) {
	reply := strings.Join([]string{
		"- an item",
		"  " + fence + "md",
		"  # not a heading",
		"  - not a list",
		"  **not strong**",
		"  _not emphasised_",
		"  " + fence,
		"- the next item",
	}, "\n")

	wantRows(t, WrapBlock(reply, 40), []string{
		"- an item",
		fence + "md",
		"  # not a heading",
		"  - not a list",
		"  **not strong**",
		"  _not emphasised_",
		fence,
		"- the next item",
	})
}

// The prose either side of a fence is prose and is rendered as such. A model
// writes the lead-in above the block, and it is a sentence rather than code.
func TestRenderMarkdownRendersProseAroundFence(t *testing.T) {
	reply := strings.Join([]string{
		"The **first** step is shown here " + fence + "sh",
		"echo 2 * 3",
		fence,
		"Then read the *second* one.",
	}, "\n")

	wantRows(t, WrapBlock(reply, 40), []string{
		"The first step is shown here",
		fence + "sh",
		"echo 2 * 3",
		fence,
		"Then read the second one.",
	})
}

// The width of a row is counted in characters rather than in bytes, since a
// reply carries multibyte text and a row cut at a byte boundary would both
// split a character and count as wider than it is.
func TestRenderMarkdownCountsColumns(t *testing.T) {
	rows := WrapBlock("### héllo wörld ünter der pane", 12)

	for i, r := range rows {
		if n := len([]rune(r)); n > 12 {
			t.Errorf("row %d = %q is %d wide, want at most 12", i, r, n)
		}
		if strings.ContainsRune(r, 0xFFFD) {
			t.Errorf("row %q holds a replacement character", r)
		}
	}
	if rows[0] != "### héllo" {
		t.Errorf("row 0 = %q, want the marker to count as four columns", rows[0])
	}
}

// The same reply folds differently at different widths, which is what makes
// rendering belong to the fold rather than to the arrival. The pane width is
// not known until the frame is drawn.
func TestRenderMarkdownFoldsAtTheWidth(t *testing.T) {
	reply := "## A heading with enough words in it that it folds"

	narrow := WrapBlock(reply, 20)
	wide := WrapBlock(reply, 60)

	if len(narrow) <= len(wide) {
		t.Errorf("narrow folded to %d rows and wide to %d, want the narrow one to be the longer",
			len(narrow), len(wide))
	}
	// Only the words are compared, since a row of the narrow fold carries the
	// indent the wide one has no room for.
	words := func(rows []string) string {
		return strings.Join(strings.Fields(strings.Join(rows, "\n")), " ")
	}
	if words(narrow) != words(wide) {
		t.Errorf("folded words differ:\n%q\n%q", words(narrow), words(wide))
	}
}

// Nothing in a rendered reply may be an escape introducer. A selection is taken
// out of the pane as plain text, and an escape sequence drawn around a word
// would be copied along with it, so a reply carrying one is not copyable.
func TestRenderMarkdownEmitsNoEscapeSequences(t *testing.T) {
	reply := strings.Join([]string{
		"# Heading",
		"- a **bold** item with `code`",
		"1. an *emphasised* one",
		"  " + fence + "go",
		"  fmt.Println(\"hello\")",
		"  " + fence,
	}, "\n")

	for _, width := range []int{1, 2, 5, 12, 40, 200} {
		for _, r := range WrapBlock(reply, width) {
			if strings.ContainsRune(r, 0x1b) {
				t.Errorf("width %d: row carries an escape introducer: %q", width, r)
			}
		}
		f := Frame{Reply: []string{reply}, Input: "a question"}
		for i, r := range Render(f, 24, width) {
			if strings.ContainsRune(r, 0x1b) {
				t.Errorf("width %d: frame row %d carries an escape introducer: %q", width, i, r)
			}
		}
	}
}

// No folded row may be wider than the pane. A row past the edge wraps, and a
// wrapped row pushes everything below it down, so a construct nested inside
// another one must not cost the frame its shape.
func TestRenderMarkdownRowsFitThePane(t *testing.T) {
	reply := strings.Join([]string{
		"### A heading with enough words to need a second row",
		"- an item with a nested list and a fenced block",
		"  - a child item",
		"  " + fence + "sh",
		"  echo one two three",
		"  " + fence,
		"1. an **ordered** item that also has to fold over rows",
	}, "\n")

	for _, width := range []int{6, 8, 10, 12, 16, 20, 40} {
		inFence := false
		for i, r := range WrapBlock(reply, width) {
			if isFence(r) {
				inFence = !inFence
				continue
			}
			if inFence {
				// A block is not folded, so a line of code may run past the
				// edge. The renderer cuts it, and the frame test below covers
				// what the reader actually sees.
				continue
			}
			// A row holding a single word wider than the pane is left over
			// the edge, since cutting a word is what the fold exists to
			// avoid. The indent in front of it is not part of that word, so
			// it is read off before the row is judged.
			body := strings.TrimLeft(r, " ")
			if n := len([]rune(r)); n > width && strings.Contains(body, " ") {
				t.Errorf("width %d: row %d = %q is %d wide", width, i, r, n)
			}
		}
	}
}

// A reply full of constructs must leave the frame the right shape at every
// width and height. The pane is folded to the width it is drawn at, so a
// construct that pushed a row over the edge would push the prompt with it.
func TestRenderMarkdownFrameKeepsItsShape(t *testing.T) {
	reply := strings.Join([]string{
		"# Heading",
		"",
		"- one item",
		"- another item with **emphasis** and `code`",
		"",
		"1. an ordered item",
		"",
		fence + "go",
		"func main() {}",
		fence,
	}, "\n")

	for _, height := range []int{8, 12, 20, 40} {
		for _, width := range []int{1, 2, 3, 5, 8, 12, 20, 40, 80} {
			f := Frame{Reply: []string{reply}, Input: "a question"}
			lines := Render(f, height, width)
			if len(lines) > height {
				t.Errorf("height %d width %d: frame is %d rows", height, width, len(lines))
			}
			for i, l := range lines {
				if n := len([]rune(l)); n > width {
					t.Errorf("height %d width %d: row %d is %d wide: %q",
						height, width, i, n, l)
				}
			}
		}
	}
}

// Every word of the reply must survive the rendering, since a dropped
// character corrupts a copied answer. Only the markers are taken, never the
// words around them.
func TestRenderMarkdownKeepsEveryWord(t *testing.T) {
	reply := strings.Join([]string{
		"## Checking the **wrapping** of a _reply_",
		"",
		"- the OPENROUTER_API_KEY file",
		"- a span holding `go build ./...`",
		"1. the price is 2 * 3 = 6",
		"",
		"The last paragraph mentions snake_case_name and issue #42.",
	}, "\n")

	rows := strings.Join(WrapBlock(reply, 80), " ")
	for _, word := range []string{
		"Checking", "wrapping", "reply", "OPENROUTER_API_KEY",
		"go build ./...", "price", "2 * 3 = 6", "snake_case_name", "#42",
	} {
		if !strings.Contains(rows, word) {
			t.Errorf("the word %q is missing from the rendered reply:\n%s", word, rows)
		}
	}
}
