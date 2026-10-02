package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// replyCorpus is the text the fold is held to. It reaches every construct the
// colour knows, every one it does not, and the shapes that have broken folds
// before: an open fence, a marker with nothing to close it, wide runes, tabs and
// nothing at all.
func replyCorpus() []string {
	f := fence
	return []string{
		"",
		" ",
		"\n",
		"\n\n",
		"plain words that need folding once they are long enough to fold",
		"# Heading",
		"## A **bold** heading with `code` and *emphasis*",
		"###### six",
		"####### seven is prose",
		"- item",
		"- a list item with `inline code` in it",
		"1. ordered with **bold** and a [link](http://x.example/a_b)",
		"  - nested item that is long enough to fold onto a second row at narrow widths",
		"    - too deep to be an item",
		"> a quote",
		"> a quote containing a [link](http://example.com/path) and more words after it",
		"> > nested quote with `code`",
		">",
		"  > an indented quote",
		"not > a quote",
		"**bold** and *emphasis* and ***both*** and __strong__ and _em_",
		"**bold with *nested emphasis* inside**",
		"*emphasis with **nested bold** inside*",
		"## **emphasis** inside a heading",
		"2 * 3 * 4 = 24 and snake_case_name and a ** lone marker",
		"an unmatched **marker and an unmatched `tick and an unmatched [bracket",
		"[text](http://example.com) and [two](b) and [not a link] (x) and [empty]()",
		"[a [b](c) d](e)",
		"[`code`](http://example.com/code) next",
		"`[not](a link)` but [yes](it_is)",
		"[x](" + strings.Repeat("y", 40) + ")",
		"```\nplain code\n```",
		"before\n" + f + "go\nfunc main() {\n\tfmt.Println(\"**not bold**\")\n}\n" + f + "\nafter with **bold**",
		"here is the code: " + f + "sh\nls -la\n" + f + " and then prose",
		"unclosed\n" + f + "\ncode that never ends\n  indented\n\nmore",
		"  " + f + "\n  indented fence\n  " + f,
		"~~~\ntilde\n~~~",
		f + "\r\ncrlf\r\n" + f,
		"- a list\n  with a\n```\ncode in a list\n```\n- again",
		"wide \u4e16\u754c runes \u4e16\u754c\u4e16\u754c\u4e16\u754c in prose and `\u4e16\u754c` in code",
		"**\u4e16\u754c** and *\u4e16*",
		"- \u4e16\u754c item with wide marker words that fold",
		"tab\tseparated\twords and\ttabs",
		"```\n\ttabbed code\t\n```",
		"trailing spaces   \n  and leading",
		"a\x00b control \x1b[31m escape\x1b[0m bytes",
		strings.Repeat("word ", 60),
		strings.Repeat("**a** ", 40),
		"a_very_long_identifier_that_does_not_fit_in_a_narrow_pane_at_all and `another_one_just_as_long`",
		"## " + strings.Repeat("long heading ", 8),
		"> " + strings.Repeat("long quote ", 8),
		"- " + strings.Repeat("long item ", 8),
		"\xff\xfe invalid bytes and **bold \xe4\xb8",
	}
}

// sampleReply is a reply with one of everything, cut at every byte to stand in
// for a stream.
const sampleReply = "# Title\n\nSome **bold** and *emphasis* text with `code` and a [link](http://x.example/p).\n\n" +
	"> quoted with [a link](http://y.example) inside\n\n" +
	"- first `item`\n- second **item**\n1. numbered\n\n" +
	"```go\nfunc main() {\n\tprintln(\"**x**\")\n}\n```\n\n" +
	"The end \u4e16\u754c."

var corpusWidths = []int{-1, 0, 1, 2, 3, 4, 5, 8, 13, 20, 40, 80}

// The styled fold returns the text WrapBlock returns, and WrapBlock returns the
// text it returned before the twin existed. The second half is held against a
// copy of the old functions, so that neither can drift without the other.
func TestStyledTextIsTheTextOfWrapBlock(t *testing.T) {
	for _, in := range replyCorpus() {
		for _, w := range corpusWidths {
			old := legacyWrapBlock(in, w)
			got := WrapBlock(in, w)
			rows, spans := WrapBlockStyled(in, w)
			if !sameRows(old, got) {
				t.Errorf("WrapBlock(%q, %d) = %q, was %q", in, w, got, old)
			}
			if !sameRows(old, rows) {
				t.Errorf("WrapBlockStyled(%q, %d) rows = %q, want %q", in, w, rows, old)
			}
			if len(spans) != len(rows) {
				t.Errorf("WrapBlockStyled(%q, %d): %d span lists for %d rows", in, w, len(spans), len(rows))
			}
		}
	}
}

// A stream is cut at every byte of a reply, so a reply is folded and coloured in
// every state it passes through, and the text is the same as ever in each.
func TestStyledTextHoldsForAStreamCutAtEveryByte(t *testing.T) {
	for _, w := range []int{1, 7, 24, 80} {
		for n := 0; n <= len(sampleReply); n++ {
			cut := sampleReply[:n]
			rows, spans := WrapBlockStyled(cut, w)
			if want := legacyWrapBlock(cut, w); !sameRows(rows, want) {
				t.Fatalf("width %d, cut at %d: rows = %q, want %q", w, n, rows, want)
			}
			checkSpans(t, fmt.Sprintf("width %d cut %d", w, n), rows, spans)
		}
	}
}

// checkSpans holds spans to what a frame may draw: each is inside its row, on
// character boundaries, ordered, disjoint, has a role, and ends before any
// spaces at the end of the row.
func checkSpans(t *testing.T, what string, rows []string, spans [][]span) {
	t.Helper()
	if len(spans) != len(rows) {
		t.Errorf("%s: %d span lists for %d rows", what, len(spans), len(rows))
		return
	}
	for i, row := range rows {
		limit := len(strings.TrimRight(row, " \t"))
		pos := 0
		for _, sp := range spans[i] {
			switch {
			case sp.start >= sp.end:
				t.Errorf("%s row %d %q: empty span %+v", what, i, row, sp)
			case sp.start < pos:
				t.Errorf("%s row %d %q: span %+v overlaps the one before it", what, i, row, sp)
			case sp.end > len(row):
				t.Errorf("%s row %d %q: span %+v runs past the row", what, i, row, sp)
			case sp.end > limit:
				t.Errorf("%s row %d %q: span %+v covers the padding", what, i, row, sp)
			case sp.role < 0 || sp.role >= roleCount:
				t.Errorf("%s row %d %q: span %+v has no role", what, i, row, sp)
			case !utf8.RuneStart(row[sp.start]) || (sp.end < len(row) && !utf8.RuneStart(row[sp.end])):
				t.Errorf("%s row %d %q: span %+v is off a character boundary", what, i, row, sp)
			}
			pos = sp.end
		}
	}
}

func TestSpansStayInsideTheirRows(t *testing.T) {
	for _, in := range replyCorpus() {
		for _, w := range corpusWidths {
			rows, spans := WrapBlockStyled(in, w)
			checkSpans(t, fmt.Sprintf("%q at %d", in, w), rows, spans)
		}
	}
}

// styledAs renders a reply and returns every row as "role=text|role=text", with
// the text outside a span left out, so a test can say what was coloured and in
// what. A row with no span is shown as "-".
func styledAs(in string, width int) []string {
	rows, spans := WrapBlockStyled(in, width)
	out := make([]string, len(rows))
	for i, row := range rows {
		if len(spans[i]) == 0 {
			out[i] = "-"
			continue
		}
		var parts []string
		for _, sp := range spans[i] {
			parts = append(parts, spanRoleName(sp.role)+"="+row[sp.start:sp.end])
		}
		out[i] = strings.Join(parts, "|")
	}
	return out
}

func spanRoleName(r role) string {
	switch r {
	case roleCode:
		return "code"
	case roleHeading:
		return "heading"
	case roleEmphasis:
		return "emphasis"
	case roleQuote:
		return "quote"
	case roleList:
		return "list"
	case roleLink:
		return "link"
	}
	return fmt.Sprintf("role%d", int(r))
}

func TestEachConstructTakesItsRole(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain prose", "just some words", []string{"-"}},
		{"heading", "## Title", []string{"heading=##|heading=Title"}},
		{"strong", "a **bold** word", []string{"emphasis=bold"}},
		{"emphasis", "an *italic* word", []string{"emphasis=italic"}},
		{"underscored", "an _italic_ and __bold__ word", []string{"emphasis=italic|emphasis=bold"}},
		{"a phrase is one span", "a *two words* here", []string{"emphasis=two words"}},
		{"nested emphasis is one span", "**bold with *em* inside**", []string{"emphasis=bold with em inside"}},
		{"inline code keeps its ticks", "run `go test` now", []string{"code=`go test`"}},
		{"code beats emphasis", "*a `b` c*", []string{"emphasis=a|code=`b`|emphasis=c"}},
		{"a marker inside code is code", "the `*` marker and `**`", []string{"code=`*`|code=`**`"}},
		{"link", "see [the docs](http://x.example/a) now", []string{"link=[the docs](http://x.example/a)"}},
		{"link with emphasis in its text", "[**bold** text](u)", []string{"link=[bold text](u)"}},
		{"link with code in its text", "[`code`](u)", []string{"link=[`code`](u)"}},
		{"a bracket in code is not a link", "`[a](b)` after", []string{"code=`[a](b)`"}},
		{"not a link without a destination", "[text] (x) and [y]", []string{"-"}},
		{"not a link with a spaced destination", "[a](b c)", []string{"-"}},
		{"an empty destination is not a link", "[a]()", []string{"-"}},
		{"the inner of two openers is the link", "[a [b](c) d](e)", []string{"link=[b](c)"}},
		{"quote", "> quoted text", []string{"quote=> quoted text"}},
		{"quote with a link", "> see [it](u) now", []string{"quote=> see|link=[it](u)|quote=now"}},
		{"quote with code", "> a `b` c", []string{"quote=> a|code=`b`|quote=c"}},
		{"indented quote", "  > quoted", []string{"quote=> quoted"}},
		{"a lone marker is a quote", ">", []string{"quote=>"}},
		{"a greater-than inside prose is not", "a > b", []string{"-"}},
		{"list marker", "- item", []string{"list=-"}},
		{"ordered marker", "12. item", []string{"list=12."}},
		{"nested marker leaves the indent", "  - item", []string{"list=-"}},
		{"list item with code", "- run `go` now", []string{"list=-|code=`go`"}},
		{"list item with emphasis", "1. a **b** c", []string{"list=1.|emphasis=b"}},
		{"emphasis inside a heading", "## A **bold** one", []string{"heading=##|heading=A|emphasis=bold|heading=one"}},
		{"code inside a heading", "# run `x`", []string{"heading=#|heading=run|code=`x`"}},
		{"an unmatched marker is text", "an **unmatched marker and a `tick", []string{"-"}},
		{"arithmetic is text", "2 * 3 * 4", []string{"-"}},
		{"identifier is text", "the snake_case_name here", []string{"-"}},
		{"fenced block", "```go\nx := 1\n```", []string{"code=```go", "code=x := 1", "code=```"}},
		{"code keeps its markers", "```\n**x** [a](b)\n```", []string{"code=```", "code=**x** [a](b)", "code=```"}},
		{"blank row in a block", "```\na\n\nb\n```", []string{"code=```", "code=a", "-", "code=b", "code=```"}},
		{"indentation of code is not coloured", "```\n    indented\n```", []string{"code=```", "code=indented", "code=```"}},
		{"trailing spaces of code are not coloured", "```\nx   \n```", []string{"code=```", "code=x", "code=```"}},
		{"prose before a fence", "see: ```sh\nls\n```", []string{"-", "code=```sh", "code=ls", "code=```"}},
		{"an open fence is code to the end", "a\n```\nb\nc", []string{"-", "code=```", "code=b", "code=c"}},
		{"markdown after a fence is coloured again", "```\nx\n```\n**y**", []string{"code=```", "code=x", "code=```", "emphasis=y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := styledAs(c.in, 80)
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("styled %q:\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

// A span that crosses a fold is split, one piece to each row, and each piece is
// where its words landed.
func TestASpanThatCrossesAFoldIsSplitPerRow(t *testing.T) {
	in := "one *two three four five* six"
	rows := WrapBlock(in, 12)
	if len(rows) != 3 {
		t.Fatalf("rows = %q", rows)
	}
	got := styledAs(in, 12)
	want := []string{"emphasis=two", "emphasis=three four", "emphasis=five"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q (rows %q)", got, want, rows)
	}
}

func TestAQuoteTakesItsRoleOnEveryRow(t *testing.T) {
	in := "> " + strings.Repeat("quoted words ", 6)
	rows, spans := WrapBlockStyled(in, 24)
	if len(rows) < 3 {
		t.Fatalf("the quote folded to %d rows", len(rows))
	}
	for i, row := range rows {
		if len(spans[i]) != 1 || spans[i][0].role != roleQuote ||
			spans[i][0].start != 0 || spans[i][0].end != len(strings.TrimRight(row, " ")) {
			t.Errorf("row %d %q carries %+v", i, row, spans[i])
		}
	}
}

func TestAHeadingAndAListFoldUnderTheirMarker(t *testing.T) {
	rows, spans := WrapBlockStyled("- a list item long enough to fold with `code` late in it", 20)
	if len(rows) < 3 {
		t.Fatalf("rows = %q", rows)
	}
	if len(spans[0]) == 0 || spans[0][0].role != roleList || rows[0][spans[0][0].start:spans[0][0].end] != "-" {
		t.Errorf("first row %q carries %+v", rows[0], spans[0])
	}
	for i := 1; i < len(rows); i++ {
		for _, sp := range spans[i] {
			if sp.role == roleList {
				t.Errorf("row %d %q carries a list marker span", i, rows[i])
			}
			if sp.start < 2 {
				t.Errorf("row %d %q has a span in its indent: %+v", i, rows[i], sp)
			}
		}
	}
	found := false
	for i, row := range rows {
		for _, sp := range spans[i] {
			if sp.role == roleCode && row[sp.start:sp.end] == "`code`" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the code span was lost: %q %+v", rows, spans)
	}
}

// A marker wider than the pane takes a row of its own, cut with an ellipsis, and
// the ellipsis is not coloured.
func TestAMarkerWiderThanThePaneKeepsItsEllipsisPlain(t *testing.T) {
	in := "   1234567890. a b"
	rows, spans := WrapBlockStyled(in, 6)
	checkSpans(t, "wide marker", rows, spans)
	if !sameRows(rows, legacyWrapBlock(in, 6)) {
		t.Fatalf("rows differ: %q", rows)
	}
	for i, row := range rows {
		for _, sp := range spans[i] {
			if strings.Contains(row[sp.start:sp.end], ellipsis) {
				t.Errorf("row %q has the ellipsis inside span %+v", row, sp)
			}
		}
	}
}

// Wide runes are counted in columns for the fold and in bytes for the spans.
func TestSpansAreBytesAndSurviveWideRunes(t *testing.T) {
	got := styledAs("a *\u4e16\u754c* b `\u4e16`", 80)
	want := []string{"emphasis=\u4e16\u754c|code=`\u4e16`"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNoEscapeEntersARowOrASpan(t *testing.T) {
	for _, in := range replyCorpus() {
		rows, _ := WrapBlockStyled(in, 40)
		for _, row := range rows {
			if strings.Contains(row, "\x1b") && !strings.Contains(in, "\x1b") {
				t.Errorf("a row of %q gained an escape: %q", in, row)
			}
		}
	}
}

// The window decides which rows are worked out, and a row in it gets the spans
// it would have had if every row had been.
func TestAWindowGivesTheSpansOfTheFullFold(t *testing.T) {
	in := strings.Join(replyCorpus(), "\n")
	for _, w := range []int{9, 30, 80} {
		fullRows, full := WrapBlockStyled(in, w)
		n := len(fullRows)
		for _, win := range [][2]int{{0, 0}, {0, 1}, {0, n}, {3, 9}, {n - 5, n}, {n - 1, n + 10}, {n, n + 1}, {5, 5}} {
			rows, got := wrapBlock(in, w, win[0], win[1])
			if !sameRows(rows, fullRows) {
				t.Fatalf("window %v changed the rows", win)
			}
			lo, hi := win[0], minInt(win[1], n)
			want := 0
			if hi > lo {
				want = hi - lo
			}
			if len(got) != want {
				t.Fatalf("width %d window %v: %d span lists, want %d", w, win, len(got), want)
			}
			for k := range got {
				if fmt.Sprint(got[k]) != fmt.Sprint(full[lo+k]) {
					t.Errorf("width %d window %v row %d: %+v, want %+v", w, win, lo+k, got[k], full[lo+k])
				}
			}
		}
	}
}

func TestNoSpansAreWorkedOutWithoutAWindow(t *testing.T) {
	_, spans := wrapBlock(sampleReply, 40, 0, 0)
	if len(spans) != 0 {
		t.Errorf("%d span lists were made for no window", len(spans))
	}
}

// notMarkdown reports whether a role is one the frame gives its chrome and its
// notices, which colour on and colour off alike draw, rather than one of the
// roles of the markdown in a reply.
func notMarkdown(r role) bool {
	return r == roleChrome || r == roleTitle || r == roleNotice
}

// replyFrame is a frame holding one model reply, with the reply recorded as one.
func replyFrame(text string) Frame {
	return Frame{
		Reply:        []string{"> question", text, "a note"},
		Kinds:        []entryKind{{}, {kind: kindReply}, {kind: kindNotice}},
		styleReplies: true,
	}
}

// The row a reply entry folds to carries its spans through the frame: the
// pane's rows are the rows WrapBlock gave, and the spans are the twin's.
func TestTheFrameColoursAReplyEntryOnly(t *testing.T) {
	f := replyFrame("a **bold** word\n```\ncode\n```")
	rows, spans, _, _ := renderStyled(f, 30, 60)
	got := map[string]string{}
	for i, row := range rows {
		for _, sp := range spans[i] {
			if notMarkdown(sp.role) {
				continue
			}
			got[strings.TrimRight(row, " ")] += spanRoleName(sp.role) + "=" + row[sp.start:sp.end] + ";"
		}
	}
	if got["a bold word"] != "emphasis=bold;" {
		t.Errorf("the reply was not coloured: %v", got)
	}
	if got["code"] != "code=code;" {
		t.Errorf("the code was not coloured: %v", got)
	}
	for text, spans := range got {
		if strings.Contains(text, "question") || text == "a note" {
			t.Errorf("a line that is not a reply was coloured: %q %s", text, spans)
		}
	}
}

// A line the user typed and a note the client wrote are plain even when they
// look like markdown.
func TestTypedLinesAndNotesStayPlain(t *testing.T) {
	f := Frame{
		Reply:        []string{"> **typed** `x` [a](b)", "# a note with `code`", "**model**"},
		Kinds:        []entryKind{{}, {kind: kindNotice}, {kind: kindReply}},
		styleReplies: true,
	}
	rows, spans, _, _ := renderStyled(f, 30, 60)
	for i, row := range rows {
		for _, sp := range spans[i] {
			if notMarkdown(sp.role) {
				continue
			}
			text := row[sp.start:sp.end]
			if text != "model" || sp.role != roleEmphasis {
				t.Errorf("unexpected span %s=%q on row %q", spanRoleName(sp.role), text, row)
			}
		}
	}
}

// Colour off draws the bytes it always did and works out no markdown span.
func TestColourOffWorksOutNoMarkdownSpans(t *testing.T) {
	on := replyFrame(sampleReply)
	off := on
	off.styleReplies = false
	rowsOn, spansOn, _, _ := renderStyled(on, 40, 60)
	rowsOff, spansOff, _, _ := renderStyled(off, 40, 60)
	if strings.Join(rowsOn, "\n") != strings.Join(rowsOff, "\n") {
		t.Fatal("colour changed the rows")
	}
	if len(spansOn) != len(spansOff) {
		t.Fatalf("%d span lists on, %d off", len(spansOn), len(spansOff))
	}
	coloured := 0
	for i := range rowsOn {
		for _, sp := range spansOff[i] {
			if !notMarkdown(sp.role) {
				t.Errorf("colour off made a reply span: %+v on %q", sp, rowsOff[i])
			}
		}
		for _, sp := range spansOn[i] {
			if !notMarkdown(sp.role) {
				coloured++
			}
		}
	}
	if coloured == 0 {
		t.Error("colour on coloured nothing in the reply")
	}

	sc, read := screenCapture(t)
	sc.height, sc.width = 40, 60
	sc.DrawFrame(rowsOff, framePaint{spans: spansOff, twiddle: -1})
	if got, want := read(), oldFrame(rowsOff, 40); !strings.HasPrefix(got, want) {
		t.Errorf("colour off is not the old bytes\n got %q\nwant %q", got, want)
	}
}

// With colour on, the row text is the same and only sequences are added.
func TestColourOnOnlyAddsSequences(t *testing.T) {
	f := replyFrame(sampleReply)
	rows, spans, _, _ := renderStyled(f, 40, 60)
	height, width := 40, 60

	sc, read := screenCapture(t)
	sc.height, sc.width = height, width
	sc.DrawFrame(rows, framePaint{spans: spans, twiddle: -1})
	off := read()

	pal := newPalette(config.Theme{})
	sc2, read2 := screenCapture(t)
	sc2.height, sc2.width = height, width
	sc2.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
	on := read2()
	if csi.ReplaceAllString(on, "") != csi.ReplaceAllString(off, "") {
		t.Error("the text with colour stripped is not the colour-off text")
	}
	for _, want := range []string{pal.role(roleHeading), pal.role(roleEmphasis), pal.role(roleCode),
		pal.role(roleQuote), pal.role(roleList), pal.role(roleLink)} {
		if !strings.Contains(on, want) {
			t.Errorf("the frame never wrote the sequence %q", want)
		}
	}
	for _, row := range rows {
		if strings.ContainsRune(row, 0x1b) {
			t.Errorf("a row holds an escape: %q", row)
		}
	}
	if strings.Contains(on, "38;2") || strings.Contains(on, "48;2") {
		t.Error("a 24 bit sequence was written")
	}
}

// The spans of every row of a frame, with colour on, line up with the rows and
// stay inside them, over every reply and every size.
func TestFrameSpansHoldAtEverySize(t *testing.T) {
	for _, in := range replyCorpus() {
		for _, size := range frameSizes {
			f := replyFrame(in)
			f.Partial = in
			rows, spans, _, _ := renderStyled(f, size[0], size[1])
			if len(spans) != len(rows) {
				t.Fatalf("%d span lists for %d rows", len(spans), len(rows))
			}
			for i, row := range rows {
				pos := 0
				for _, sp := range spans[i] {
					if sp.start < pos || sp.start >= sp.end || sp.end > len(row) {
						t.Errorf("%q at %v: row %q has span %+v", in, size, row, sp)
					}
					if sp.end > len(strings.TrimRight(row, " \t")) {
						t.Errorf("%q at %v: row %q has a span over its padding: %+v", in, size, row, sp)
					}
					pos = sp.end
				}
			}
		}
	}
}

// A partial and the entry it becomes are coloured the same, row for row, so the
// colour does not change when the stream ends.
func TestAPartialAndItsEntryAreColouredAlike(t *testing.T) {
	for n := 0; n <= len(sampleReply); n += 3 {
		text := strings.TrimRight(sampleReply[:n], "\n")
		streaming := Frame{Reply: []string{"> q"}, Kinds: []entryKind{{}}, Partial: text, styleReplies: true}
		done := Frame{Reply: []string{"> q", text}, Kinds: []entryKind{{}, {kind: kindReply}}, styleReplies: true}
		ra, a, _, _ := renderStyled(streaming, 60, 40)
		rb, b, _, _ := renderStyled(done, 60, 40)
		if strings.Join(ra, "\n") != strings.Join(rb, "\n") {
			t.Fatalf("cut at %d: the rows changed when the stream ended", n)
		}
		for i := range a {
			if fmt.Sprint(a[i]) != fmt.Sprint(b[i]) {
				t.Fatalf("cut at %d: row %d %q changed colour when the stream ended: %+v then %+v",
					n, i, ra[i], a[i], b[i])
			}
		}
	}
}

// What is open when a stream is cut is coloured as the renderer already treats
// it: a fence with no end is code, and a marker with no partner is text.
func TestWhatIsOpenMidStreamIsColouredAsTheRendererTreatsIt(t *testing.T) {
	got := styledAs("text **bold\n```\ncode", 80)
	want := []string{"-", "code=```", "code=code"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
	got = styledAs("an *open marker and `an open tick and [a link](", 80)
	if strings.Join(got, "\n") != "-" {
		t.Errorf("got %q, want nothing coloured", got)
	}
}

// bigReply is a reply of n lines with every construct in it.
func bigReply(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&b, "## Section %d with **bold** words\n", i)
		case 1:
			b.WriteString("Some prose with `inline code`, *emphasis* and a [link](http://example.com/x) that is long enough to fold at eighty columns easily.\n")
		case 2:
			b.WriteString("> a quoted line with [a link](http://example.com/q) in it\n")
		case 3, 4:
			b.WriteString("- a list item with `code` and **bold** text\n")
		case 5:
			b.WriteString("```go\n")
		case 6, 7:
			b.WriteString("\tfmt.Println(\"**not bold**\", 1*2*3)\n")
		case 8:
			b.WriteString("```\n")
		default:
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// A repaint is the cost of the fold and a screenful of spans: growing the reply
// by a factor of ten grows the colour-on repaint by about that factor and no
// more. The bound is loose, since a timing is only roughly proportional, and it
// is there to catch a repaint that goes with the square.
func TestARepaintIsNotQuadraticInTheReply(t *testing.T) {
	if testing.Short() {
		t.Skip("timing")
	}
	measure := func(n int) time.Duration {
		f := Frame{Reply: []string{bigReply(n)}, Kinds: []entryKind{{kind: kindReply}}, styleReplies: true}
		best := time.Hour
		for i := 0; i < 5; i++ {
			start := time.Now()
			renderStyled(f, 40, 80)
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}
	small, large := measure(2000), measure(20000)
	if large > 30*small {
		t.Errorf("a repaint of 20000 lines took %v and of 2000 took %v", large, small)
	}
}

// A line full of brackets and parentheses is read in time that goes with its
// length.
func TestLinkScanIsLinear(t *testing.T) {
	inputs := []string{
		strings.Repeat("[a](", 30000),
		strings.Repeat("[", 60000),
		strings.Repeat("[a]", 30000),
		strings.Repeat("[a](b", 20000),
		strings.Repeat("]", 60000),
		strings.Repeat("[a](bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 2000),
	}
	for _, in := range inputs {
		start := time.Now()
		WrapBlockStyled(in, 80)
		if d := time.Since(start); d > 3*time.Second {
			t.Errorf("a line of %d bytes took %v", len(in), d)
		}
	}
}

func TestADelegateAnswerIsColouredAndItsQuestionIsNot(t *testing.T) {
	s := &Session{}
	s.addDelegateLines("/delegate what is **this**", "a note with `code`")
	s.mu.Lock()
	s.dpane.addAnswer("the **answer** with `code`")
	s.dpane.lines = append(s.dpane.lines, "(error) later **line**")
	s.registerPanes()
	s.panes.Select(delegatePaneIndex)
	f := s.frame
	s.applyDelegatePane(&f)
	s.mu.Unlock()
	f.styleReplies = true
	if len(f.Reply) != 4 || len(f.Kinds) != 3 {
		t.Fatalf("the pane holds %d lines and %d records", len(f.Reply), len(f.Kinds))
	}
	got := map[string][]string{}
	rows, spans, _, _ := renderStyled(f, 30, 80)
	for i, row := range rows {
		for _, sp := range spans[i] {
			if notMarkdown(sp.role) {
				continue
			}
			key := strings.TrimRight(row, " ")
			got[key] = append(got[key], spanRoleName(sp.role)+"="+row[sp.start:sp.end])
		}
	}
	if len(got) != 1 || strings.Join(got["the answer with `code`"], "|") != "emphasis=answer|code=`code`" {
		t.Errorf("only the answer should be coloured: %v", got)
	}
}
