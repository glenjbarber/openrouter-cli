package tui

import (
	"math"
	"strings"
	"unicode/utf8"
)

// wrapWidth is the column a line is wrapped at, when the caller has no better
// figure.
const wrapWidth = 80

// fenceMarkers are the characters that open and close a fenced code block.
//
// A fenced block is recognised on its opening fence line alone, since the
// closing fence is only known once the block has been read. The indent is
// tolerated because a list item containing a block indents it.
var fenceMarkers = []string{"```", "~~~"}

// wrapText folds prose to width, without breaking a word.
//
// A word longer than the width is left on a line of its own rather than being
// cut, since a URL or a path broken across two lines is neither readable nor
// copyable. A fenced code block is passed through unwrapped, because wrapping
// code changes what it means and a line that looks wrong is better than one
// that is.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = wrapWidth
	}
	if s == "" {
		return []string{""}
	}
	if strings.TrimSpace(s) == "" {
		return []string{s}
	}
	return wrapProse(s, width)
}

// wrapProse folds a paragraph of prose, without rendering markdown in it.
//
// It is the fold behind wrapText, which is a helper rather than the path the
// pane draws from. What the reader sees goes through WrapBlock, so that the
// markdown in a reply is rendered as it is folded rather than kept as it was
// written.
func wrapProse(s string, width int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, foldWords(para, width)...)
	}
	return out
}

// foldWords breaks one paragraph into lines no wider than width.
func foldWords(para string, width int) []string {
	words := strings.Fields(para)
	if len(words) == 0 {
		return []string{""}
	}

	var out []string
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		// A word that cannot fit is moved to the next line whole. Cutting it
		// would split an identifier across two rows, which is neither
		// readable nor copyable. The width is counted in columns, since a
		// character two columns wide counted as one folds a row twice as
		// wide as the pane.
		if displayWidth(candidate) > width {
			out = append(out, line)
			line = w
			continue
		}
		line = candidate
	}
	return append(out, line)
}

// WrapBlock folds a reply for display.
//
// Prose is rendered as markdown and then folded to width, in that order, since
// a marker that was left in place would be folded as though it were a word. A
// fenced code block is kept line for line, and a line inside one that is wider
// than the pane is left overlong rather than folded, since a code line that has
// been reflowed is no longer the code that was written. It is marked so that
// the reader can see it continues past the edge instead of believing it ended
// there. A block is never rendered: a marker inside one is the code rather than
// a construct, and stripping an asterisk out of a shell glob would change what
// the reader was shown.
func WrapBlock(s string, width int) []string {
	rows, _ := wrapBlock(s, width, 0, 0)
	return rows
}

// WrapBlockStyled folds a reply exactly as WrapBlock does and returns the style
// spans of every row beside it.
//
// The rows are the rows WrapBlock returns, byte for byte, since both are the one
// function. The spans are the colour a reply takes when colour is on: code,
// headings, emphasis, quotes, list markers and links. They are offsets into the
// row and never text in it, so nothing here puts an escape character anywhere.
func WrapBlockStyled(s string, width int) ([]string, [][]span) {
	return wrapBlock(s, width, 0, math.MaxInt)
}

// wrapBlock is the one fold behind WrapBlock and WrapBlockStyled.
//
// Spans are worked out only for the rows from lo up to but not including hi,
// counted from the first row of the reply, and the list returned holds one entry
// for each of those rows that exists, so entry k belongs to row lo+k. A frame is
// repainted on every tick of the spinner and shows a screenful of a reply that
// may be thousands of rows long, so the rows out of sight cost no more than they
// did before colour existed. With lo and hi both zero no span is worked out at
// all, and the return is the fold WrapBlock always was.
func wrapBlock(s string, width, lo, hi int) ([]string, [][]span) {
	if width < 1 {
		width = wrapWidth
	}

	var out []string
	var spans [][]span
	var block []string
	inFence := false

	// add appends one row, and its spans when the row is one the caller asked
	// for. The spans are finished against the row they belong to, so that none
	// reaches into the spaces at its end or past its length.
	add := func(row string, sp []span) {
		if at := len(out); at >= lo && at < hi {
			spans = append(spans, finishSpans(row, sp))
		}
		out = append(out, row)
	}
	// addCode appends a row drawn as code. The spans of a row that is out of
	// sight are not worked out.
	addCode := func(row string) {
		if at := len(out); at >= lo && at < hi {
			add(row, codeRowSpans(row))
			return
		}
		out = append(out, row)
	}
	// addProse appends the rows one line of prose folds to. The line is folded
	// without spans first, which is all a row out of sight needs, and folded
	// again with them only when one of its rows is in sight. The text of the two
	// is the same, and the first is the one kept, so that a fault in the second
	// could cost a line its colour and nothing else.
	addProse := func(line string) {
		rows, _ := renderMarkdownRows(line, width, false)
		at := len(out)
		if at < hi && at+len(rows) > lo {
			if again, sp := renderMarkdownRows(line, width, true); sameRows(rows, again) {
				for i, row := range rows {
					add(row, sp[i])
				}
				return
			}
		}
		for _, row := range rows {
			add(row, nil)
		}
	}

	flush := func() {
		for _, l := range block {
			// A line holding only indentation is blank, since keeping it
			// leaves a column of spaces that reads as content.
			if strings.TrimSpace(l) == "" {
				add("", nil)
				continue
			}
			addCode(l)
		}
		block = nil
	}

	for _, line := range strings.Split(s, "\n") {
		// A marker partway along a line is split out, so the prose before it
		// is still folded as prose and only the remainder is a fence. A model
		// writes "here is the code:" and the marker on one line often enough
		// that treating the whole line as a fence would fold the lead-in.
		before, marker, after := splitFence(line)
		if marker != "" {
			// The indent in front of a marker is layout, not content. Kept
			// as prose it becomes a blank row of its own, and kept inside a
			// block it becomes a row of spaces.
			before = strings.TrimRight(before, " \t")
			if before != "" {
				if inFence {
					block = append(block, before)
				} else {
					addProse(before)
				}
			}
			flush()
			inFence = !inFence
			addCode(marker)
			if after != "" {
				if inFence {
					block = append(block, strings.TrimRight(after, " \t"))
				} else {
					addProse(after)
				}
			}
			continue
		}

		if inFence {
			block = append(block, line)
			continue
		}
		addProse(line)
	}

	// A block left open at the end is still emitted, since a reply cut short
	// by a disconnect should not lose its tail.
	flush()
	return out, spans
}

// sameRows reports whether two lists of rows are the same text.
func sameRows(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// splitFence divides a line at the first fence marker.
//
// The text before the marker is prose, the marker itself opens or closes a
// block, and the text after it belongs inside that block. A line with no marker
// returns an empty marker.
func splitFence(line string) (before, marker, after string) {
	at := -1
	found := ""
	for _, m := range fenceMarkers {
		if i := strings.Index(line, m); i >= 0 && (at < 0 || i < at) {
			at, found = i, m
		}
	}
	if at < 0 {
		return "", "", ""
	}

	rest := line[at+len(found):]

	// An opening fence may carry an info string naming the language. It
	// belongs to the fence, so it is taken with it rather than being left as
	// the first line of the block, which would put "sh" inside the code.
	end := len(rest)
	if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
		end = i
	}
	return line[:at], found + rest[:end], rest[end:]
}

// isFence reports whether a line carries a fence marker anywhere.
func isFence(line string) bool {
	_, m, _ := splitFence(line)
	return m != ""
}

// runeLen counts display characters rather than bytes, since a line is cut or
// folded on characters and a byte count would split a multibyte one.
func runeLen(s string) int { return utf8.RuneCountInString(s) }
