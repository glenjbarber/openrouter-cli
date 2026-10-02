package tui

import (
	"strings"
	"unicode"
)

// This file works out the style spans of a model reply: which stretch of which
// row is code, a heading, emphasis, a quote, a list marker or a link.
//
// The spans are integers beside the rows and never text inside them. The rows
// are produced by the same code with or without them, so the text of a row does
// not depend on whether anything is going to be coloured, and a construct is
// recognised for colour without being rewritten. The one place a span is turned
// into bytes is Screen.DrawFrame, and only when colour is on.
//
// Spans are built in three steps. The inline scan reports where emphasis and
// code lie in the text it leaves, and the link scan reports where links lie in
// the same text. Those are painted, one byte at a time, into a role for each
// byte of the line, in a fixed order so that a later one wins where two overlap.
// The fold then maps the roles of each word onto the row the word lands on.
// Painting is what makes the spans of a row come out ordered and disjoint
// however the constructs nested.

// inlineMark is a stretch of a rendered line that takes a role. The offsets are
// bytes into the text the inline scan returned.
type inlineMark struct {
	start, end int
	role       role
}

// codeSkipper steps over the code spans of a line while the line is scanned in
// order. A bracket inside a code span is code rather than the start of a link.
type codeSkipper struct {
	marks []inlineMark
	at    int
}

// skip returns pos, or the end of the code span that holds it. Positions must
// be given in ascending order.
func (c *codeSkipper) skip(pos int) int {
	for c.at < len(c.marks) && c.marks[c.at].end <= pos {
		c.at++
	}
	if c.at < len(c.marks) && c.marks[c.at].start <= pos {
		return c.marks[c.at].end
	}
	return pos
}

// maxLinkDest is the longest destination a link is read with. A destination is
// one word, and a longer run is a string of characters that happens to follow a
// bracket.
const maxLinkDest = 2048

// findLinks returns the links in a rendered line as they appear in it.
//
// A link is "[text](destination)". The whole of it is marked, brackets and
// destination included, since all of it is on the screen and a link coloured
// only in its text would leave the address looking like prose. The text is not
// rewritten: the brackets and the address stay where they are.
//
// The destination is one word. A bracketed phrase followed by a space and a
// parenthesis is prose, and a destination with a space in it is a title the
// scan does not read. A bracket inside a code span is code.
//
// The scan is linear in the line. A bracket that finds no closer ends the scan,
// since every later bracket would find none either. A bracket whose closer is
// not followed by a destination hands the scan on past that closer, since any
// bracket before it would meet the same closer and fail the same way. A
// destination scan that ran out of word without finding its closing parenthesis
// remembers where, since a later one would run out in the same place.
func findLinks(text string, code []inlineMark) []inlineMark {
	if !strings.Contains(text, "](") {
		return nil
	}
	var out []inlineMark
	skipper := codeSkipper{marks: code}
	// noParenBefore is a position before which no closing parenthesis was
	// found, for a destination scan that began at or after noParenFrom.
	noParenFrom, noParenBefore := -1, -1

	for i := 0; i < len(text); {
		i = skipper.skip(i)
		if i >= len(text) {
			break
		}
		if text[i] != '[' {
			i++
			continue
		}
		// The closer is the first bracket after the opener that is not code. A
		// second opener met first is where the link starts instead.
		j := i + 1
		for j < len(text) {
			j = skipper.skip(j)
			if j >= len(text) || text[j] == ']' || text[j] == '[' {
				break
			}
			j++
		}
		if j >= len(text) {
			break
		}
		if text[j] == '[' {
			i = j
			continue
		}
		if j+1 >= len(text) || text[j+1] != '(' {
			i = j
			continue
		}
		dest := j + 2
		if dest >= noParenFrom && noParenFrom >= 0 && dest < noParenBefore {
			i = j
			continue
		}
		k := dest
		for k < len(text) && k-dest < maxLinkDest && text[k] != ')' && !isSpaceByte(text[k]) {
			k++
		}
		if k >= len(text) || text[k] != ')' || k == dest {
			// Running out of word is what a later scan would do too. Stopping
			// at the length limit is not, since a later scan starts further on.
			if k >= len(text) || isSpaceByte(text[k]) {
				noParenFrom, noParenBefore = dest, k
			}
			i = j
			continue
		}
		out = append(out, inlineMark{start: i, end: k + 1, role: roleLink})
		i = k + 1
	}
	return out
}

// isSpaceByte reports whether a byte is ASCII whitespace.
func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r'
}

// paintRoles returns one role for each byte of a line of n bytes, or none when
// nothing is painted. The base role is laid first and each mark over it in
// order, so a later mark wins. A byte with no role holds noRole.
func paintRoles(n int, base role, marks []inlineMark) []int8 {
	if n == 0 || (base < 0 && len(marks) == 0) {
		return nil
	}
	roles := make([]int8, n)
	fill := int8(noRole)
	if base >= 0 {
		fill = int8(base)
	}
	for i := range roles {
		roles[i] = fill
	}
	for _, m := range marks {
		if m.role < 0 || m.role >= roleCount {
			continue
		}
		start, end := m.start, m.end
		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}
		for p := start; p < end; p++ {
			roles[p] = int8(m.role)
		}
	}
	return roles
}

// fieldRanges returns where the words of a line lie, as strings.Fields splits
// it, but as byte ranges so that the roles under each word can be found.
func fieldRanges(s string) [][2]int {
	var out [][2]int
	start := -1
	for i, r := range s {
		if unicode.IsSpace(r) {
			if start >= 0 {
				out = append(out, [2]int{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, [2]int{start, len(s)})
	}
	return out
}

// foldWordsStyled folds a line the way foldWords does and returns the spans of
// each row beside it.
//
// The rows are the same as foldWords returns, since the words and the width
// arithmetic are the same: the width of a candidate row is the width of the row
// so far, one column for the space, and the width of the word, which is what
// measuring the joined row gives. What is added is the roles. Every word that
// lands on a row carries its roles to the offset it lands at, and the single
// space the fold puts between two words takes the role of the gap in the source
// only where the words on both sides of it have that role too, so that an
// emphasised phrase is one span rather than a span for each word.
func foldWordsStyled(text string, roles []int8, width int) ([]string, [][]span) {
	words := fieldRanges(text)
	if len(words) == 0 {
		return []string{""}, make([][]span, 1)
	}

	var rows []string
	var spans [][]span
	emit := func(from, to int) {
		var b strings.Builder
		var sp []span
		for k := from; k < to; k++ {
			ws, we := words[k][0], words[k][1]
			if k > from {
				if roles != nil && gapTakesRole(roles, words[k-1][1], ws) {
					sp = addSpan(sp, b.Len(), b.Len()+1, role(roles[ws]))
				}
				b.WriteByte(' ')
			}
			if roles != nil {
				sp = copyWordRoles(sp, roles, ws, we, b.Len())
			}
			b.WriteString(text[ws:we])
		}
		rows = append(rows, b.String())
		spans = append(spans, sp)
	}

	first := 0
	cur := displayWidth(text[words[0][0]:words[0][1]])
	for k := 1; k < len(words); k++ {
		w := displayWidth(text[words[k][0]:words[k][1]])
		if cur+1+w > width {
			emit(first, k)
			first, cur = k, w
			continue
		}
		cur += 1 + w
	}
	emit(first, len(words))
	return rows, spans
}

// gapTakesRole reports whether the gap between two words is drawn in a role.
// It is, when the last byte of the word before the gap, the first byte of the
// gap and the first byte of the word after it all carry the same role.
func gapTakesRole(roles []int8, prevEnd, nextStart int) bool {
	if prevEnd < 1 || prevEnd >= len(roles) || nextStart >= len(roles) {
		return false
	}
	r := roles[prevEnd-1]
	return r >= 0 && roles[prevEnd] == r && roles[nextStart] == r
}

// copyWordRoles appends the spans of the word at [ws, we) of the text, placed
// at offset at of its row.
func copyWordRoles(sp []span, roles []int8, ws, we, at int) []span {
	if we > len(roles) {
		we = len(roles)
	}
	for p := ws; p < we; {
		r := roles[p]
		q := p + 1
		for q < we && roles[q] == r {
			q++
		}
		if r >= 0 {
			sp = addSpan(sp, at+(p-ws), at+(q-ws), role(r))
		}
		p = q
	}
	return sp
}

// addSpan appends a span, joining it to the one before when the two touch and
// carry the same role.
func addSpan(sp []span, start, end int, r role) []span {
	if start >= end {
		return sp
	}
	if n := len(sp); n > 0 && sp[n-1].end == start && sp[n-1].role == r {
		sp[n-1].end = end
		return sp
	}
	return append(sp, span{start: start, end: end, role: r})
}

// shiftSpans returns spans moved right by n bytes.
func shiftSpans(sp []span, n int) []span {
	if n == 0 || len(sp) == 0 {
		return sp
	}
	out := make([]span, len(sp))
	for i, s := range sp {
		out[i] = span{start: s.start + n, end: s.end + n, role: s.role}
	}
	return out
}

// markerSpans returns the span of a marker in a role. The marker is the text of
// prefix without its indentation and without the space that follows it. A
// prefix cut to fit the pane is given as shown, and the ellipsis the cut added
// is left unstyled.
func markerSpans(shown, prefix string, r role) []span {
	if r < 0 || r >= roleCount {
		return nil
	}
	end := len(strings.TrimRight(shown, " \t"))
	if shown != prefix && strings.HasSuffix(shown, ellipsis) {
		end = len(shown) - len(ellipsis)
	}
	start := len(shown) - len(strings.TrimLeft(shown, " \t"))
	if start >= end {
		return nil
	}
	return []span{{start: start, end: end, role: r}}
}

// codeRowSpans returns the span of a row of code: the code itself, without the
// indentation in front of it or the spaces behind it.
func codeRowSpans(row string) []span {
	end := len(strings.TrimRight(row, " \t"))
	start := len(row) - len(strings.TrimLeft(row, " \t"))
	if start >= end {
		return nil
	}
	return []span{{start: start, end: end, role: roleCode}}
}

// finishSpans makes the spans of a row safe to draw: ordered, disjoint, inside
// the row, not reaching into the spaces at its end, and with touching spans of
// one role joined. A span left with nothing in it is dropped.
func finishSpans(row string, sp []span) []span {
	if len(sp) == 0 {
		return nil
	}
	limit := len(strings.TrimRight(row, " \t"))
	out := make([]span, 0, len(sp))
	pos := 0
	for _, s := range sp {
		if s.start < pos {
			s.start = pos
		}
		if s.end > limit {
			s.end = limit
		}
		if s.start >= s.end || s.role < 0 || s.role >= roleCount {
			continue
		}
		out = addSpan(out, s.start, s.end, s.role)
		pos = s.end
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
