package tui

import "strings"

// This file holds the style spans that travel beside the rows of a frame.
//
// A row is plain text, and stays plain text: no escape character is ever
// written into it, so folding, width, scrolling, search, /copy, saved files and
// the model context all see the text and nothing else. What color a stretch of
// a row takes is kept apart, as integers, and only Screen.DrawFrame turns it
// into bytes at draw time, and only when color is on.

// span names a stretch of one row and the role it is drawn in.
//
// The offsets are bytes into the row as it is returned, not columns and not
// runes. Both lie on rune boundaries, since a span is built from whole strings
// and from the box-drawing runes, and the screen cuts the row at them.
type span struct {
	start, end int
	role       role
}

// noRole marks a row or a stretch that is drawn in no role at all.
const noRole role = -1

// wholeRow returns the one span that covers a row, or none where the row is
// empty or has no role. An empty row has no character to color, and writing a
// sequence around nothing is a sequence for nothing.
func wholeRow(row string, r role) []span {
	if row == "" || r < 0 || r >= roleCount {
		return nil
	}
	return []span{{start: 0, end: len(row), role: r}}
}

// boxSpans returns the spans of one row of the approval box.
//
// The top and bottom are borders all the way across. A side row is a border
// only at its two edges, so the question inside it keeps the color of the rest
// of the frame, and a border drawn in one color from corner to corner does not
// read as a line of text that happens to be long. A bare question, which is
// what a terminal too narrow for a box gets, is the approval itself and is
// drawn whole.
func boxSpans(row string) []span {
	switch {
	case strings.HasPrefix(row, boxTopLeft), strings.HasPrefix(row, boxBottomLeft):
		return wholeRow(row, roleApproval)
	case strings.HasPrefix(row, boxVertical) && strings.HasSuffix(row, boxVertical) &&
		len(row) >= 2*len(boxVertical):
		return []span{
			{start: 0, end: len(boxVertical), role: roleApproval},
			{start: len(row) - len(boxVertical), end: len(row), role: roleApproval},
		}
	default:
		return wholeRow(row, roleApproval)
	}
}

// remapSpans carries spans over from a row to the row as plainRow leaves it.
//
// Dropping a byte shortens the row and moves every offset after it. The new
// offset of a position is the length of what plainRow leaves of the text before
// it, so the spans stay on the same characters. A span that is left with nothing
// in it is dropped.
func remapSpans(row string, spans []span) []span {
	if len(spans) == 0 {
		return spans
	}
	out := make([]span, 0, len(spans))
	for _, sp := range spans {
		if sp.start < 0 || sp.end > len(row) || sp.start >= sp.end {
			continue
		}
		start := len(plainRow(row[:sp.start]))
		end := len(plainRow(row[:sp.end]))
		if start < end {
			out = append(out, span{start: start, end: end, role: sp.role})
		}
	}
	return out
}

// replyRun is a stretch of the rows of a frame that came from one piece of model
// text: where it starts among the rows, how many rows it folded to, and the text
// they were folded from.
type replyRun struct {
	start, n int
	text     string
}

// fillReplySpans works out the markdown spans of the runs that reach the rows
// from lo up to but not including hi, and stores them in spans beside the rows.
//
// A run is folded again, with spans, only when one of its rows is in sight. The
// fold is the same one that made the rows, so its rows are the same, and a run
// that did not come out the length it went in is left plain rather than risk
// putting a span on the wrong row.
func fillReplySpans(spans [][]span, runs []replyRun, width, lo, hi int) {
	for _, run := range runs {
		first, last := maxInt(lo, run.start), minInt(hi, run.start+run.n)
		if first >= last {
			continue
		}
		rows, sp := wrapBlock(run.text, width, first-run.start, last-run.start)
		if len(rows) != run.n || len(sp) != last-first {
			continue
		}
		for k, one := range sp {
			if at := first + k; at < len(spans) {
				spans[at] = one
			}
		}
	}
}
