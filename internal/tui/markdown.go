package tui

import (
	"strings"
	"unicode"
)

// A reply is rendered as plain text, and every construct below is rendered by
// what it leaves rather than by how it is drawn.
//
// The interface draws no styling of its own, since a selection taken out of
// the pane is copied as whatever is on the screen, and an escape sequence
// wrapped around a word would be copied along with it. The pane search settled
// the same point for the column it reports, so the rule is read across rather
// than stated twice.
//
// PROVISIONAL: how a construct should look in plain text is not settled. Each
// choice below is the most conservative one available and is marked where it
// is taken, since a maintainer may reasonably want a heading ruled off, a
// bullet normalised, or a code span quoted.

// maxHeadingLevel is the deepest ATX heading read as a heading.
//
// A seventh hash is prose, since a line of them is a comment rather than a
// heading, and a construct that swallowed every hash would swallow a sentence.
const maxHeadingLevel = 6

// maxLeadIndent is the indentation a block construct may carry and still be
// recognised. A heading written inside a list is indented, and three columns
// is as deep as the nesting goes before the nesting rather than the construct
// is what the line is about.
const maxLeadIndent = 3

// maxDelimRun is the longest run of asterisks or underscores read as a
// delimiter. A longer run is not emphasis in any dialect, and reading one as a
// delimiter would strip characters out of a row of asterisks.
const maxDelimRun = 3

// renderMarkdown renders one line of prose and folds it to width.
//
// It belongs to the folding path rather than to the receive path, since a
// reply is stored whole and folded when it is drawn. A reply rendered as it
// arrived would have been folded to a width it could not know, and a reply
// arriving in pieces is not yet known to hold a construct at all.
//
// A fenced block never reaches here, because the wrapper separates the block
// before this is called. A line inside one is left exactly as it was written.
func renderMarkdown(line string, width int) []string {
	rows, _ := renderMarkdownRows(line, width, false)
	return rows
}

// renderMarkdownRows renders one line of prose, folds it to width, and when
// track is set also returns the style spans of each row.
//
// The text is the same with track set or clear, since both are the one path:
// the spans are worked out beside the text and never decide it. A line that is
// quoted, a heading, or a list item takes the roles below, and the inline
// constructs inside it are laid over them.
func renderMarkdownRows(line string, width int, track bool) ([]string, [][]span) {
	if width < 1 {
		width = wrapWidth
	}
	if strings.TrimSpace(line) == "" {
		return []string{""}, blankSpans(1, track)
	}
	if prefix, text, ok := splitHeading(line); ok {
		return foldConstructRows(prefix, text, width, roleHeading, roleHeading, track)
	}
	if prefix, text, ok := splitListItem(line); ok {
		return foldConstructRows(prefix, text, width, roleList, noRole, track)
	}
	text, marks := scanInline(line, track)
	if !track {
		return foldWords(text, width), nil
	}
	base := noRole
	if isQuote(line) {
		base = roleQuote
	}
	return foldWordsStyled(text, paintRoles(len(text), base, marks), width)
}

// blankSpans returns a list of n empty span lists, or none when spans are not
// being tracked.
func blankSpans(n int, track bool) [][]span {
	if !track {
		return nil
	}
	return make([][]span, n)
}

// isQuote reports whether a line is a block quote.
//
// It is recognised for color only. The marker stays in the text, since
// dropping it would edit the reply rather than show it, and the rows of a quote
// that folds are not indented under it.
func isQuote(line string) bool {
	t := strings.TrimLeft(line, " \t")
	return t == ">" || strings.HasPrefix(t, "> ")
}

// foldConstruct folds the text of a block construct under its marker.
func foldConstruct(prefix, text string, width int) []string {
	rows, _ := foldConstructRows(prefix, text, width, noRole, noRole, false)
	return rows
}

// foldConstructRows folds the text of a block construct under its marker, and
// when track is set returns the spans of each row.
//
// The rows after the first are indented to line up under the first character
// of the text, since a construct whose rows ran back to the edge would read as
// several constructs rather than one. The indent is taken off the width the
// text is folded to, so that every row stays within the pane rather than the
// last row running over the edge.
//
// A marker wider than the pane leaves nothing beside it for the indent to line
// up with, so the text is folded to the full width and the marker takes a row
// of its own. That row is left over the edge rather than cut, which is what a
// row holding a word wider than the pane does as well.
//
// The marker is drawn in prefixRole and the text starts from textRole. The
// spans of the text are shifted by the width of what stands in front of them on
// each row, which is the marker on the first and the indent on the others.
func foldConstructRows(prefix, text string, width int, prefixRole, textRole role, track bool) ([]string, [][]span) {
	// The text is rendered here rather than by the caller, so that a heading
	// and a list item cannot each forget to do it.
	text, marks := scanInline(text, track)
	var roles []int8
	if track {
		roles = paintRoles(len(text), textRole, marks)
	}
	fold := func(w int) ([]string, [][]span) {
		if !track {
			return foldWords(text, w), nil
		}
		return foldWordsStyled(text, roles, w)
	}

	// The hang is measured in columns, since a marker carrying a wide
	// character takes two of them and indenting the rows under a count that
	// says one would leave them short of the text they belong to.
	hang := displayWidth(prefix)
	if hang >= width {
		rows, sp := fold(width)
		head := truncate(prefix, width)
		out := append([]string{head}, rows...)
		if !track {
			return out, nil
		}
		return out, append([][]span{markerSpans(head, prefix, prefixRole)}, sp...)
	}

	rows, sp := fold(width - hang)
	indent := strings.Repeat(" ", hang)
	out := make([]string, 0, len(rows))
	var spans [][]span
	if track {
		spans = make([][]span, 0, len(rows))
	}
	for i, row := range rows {
		lead := indent
		if i == 0 {
			lead = prefix
		}
		out = append(out, lead+row)
		if !track {
			continue
		}
		shifted := shiftSpans(sp[i], len(lead))
		if i == 0 {
			shifted = append(markerSpans(prefix, prefix, prefixRole), shifted...)
		}
		spans = append(spans, shifted)
	}
	return out, spans
}

// splitHeading separates the marker of an ATX heading from its text.
//
// A heading needs a space after the hashes, since a line beginning with a hash
// and none is a tag or a reference to an issue rather than a heading. The
// indentation of a nested heading is kept in front of the marker, so that the
// nesting survives the fold.
//
// PROVISIONAL: the marker is kept as it was written rather than replaced by an
// underline, a run of capitals, or a blank row above it. Keeping it changes no
// character of the reply, adds no row, and leaves what is copied out of the
// pane still a heading. A maintainer who would rather see the text ruled off
// is looking at a different decision, and it is made in one place here. The
// closing run of hashes on a heading is left in the text for the same reason:
// dropping it edits the reply rather than showing it.
func splitHeading(line string) (prefix, text string, ok bool) {
	r := []rune(line)
	at := leadingSpaces(r)

	level := 0
	for at+level < len(r) && r[at+level] == '#' {
		level++
	}
	if level == 0 || level > maxHeadingLevel {
		return "", "", false
	}
	if at > maxLeadIndent {
		return "", "", false
	}
	after := at + level
	if after >= len(r) || !isSpace(r[after]) {
		return "", "", false
	}
	return string(r[:after+1]), string(r[after+1:]), true
}

// splitListItem separates the marker of a list item from its text.
//
// The marker is read only where a space follows it, so a minus in front of a
// figure and an asterisk in front of a word are arithmetic and prose rather
// than a list. The spaces after the marker are collapsed to one, since they are
// layout rather than content, and the indentation of a nested item is kept in
// front of the marker.
//
// A line indented further than a nested list is reached is not an item, since
// what it holds is a block rather than an item in a list.
//
// PROVISIONAL: the marker is kept as the model wrote it rather than normalised
// to one bullet and one delimiter. Normalising would be tidier on screen and
// would change what a reader copies, and a reply carrying mixed bullets is a
// thing a model does rather than a thing to correct.
func splitListItem(line string) (prefix, text string, ok bool) {
	r := []rune(line)
	at := leadingSpaces(r)
	if at >= len(r) || at > maxLeadIndent {
		return "", "", false
	}

	end := at
	if isDigit(r[at]) {
		// An ordered item begins with a figure and a delimiter, and the
		// delimiter is what separates the number from what follows it.
		for end < len(r) && isDigit(r[end]) {
			end++
		}
		if end >= len(r) || (r[end] != '.' && r[end] != ')') {
			return "", "", false
		}
		end++
	} else {
		if !isBullet(r[at]) {
			return "", "", false
		}
		end++
	}

	if end >= len(r) || !isSpace(r[end]) {
		return "", "", false
	}
	return string(r[:end+1]), string(r[end+1:]), true
}

// renderInline strips the markers that carry no meaning in plain text.
//
// A strong span and an emphasised span are shown as the words they hold, since
// there is no styling left to carry the distinction and the asterisks copied
// out of the pane would be punctuation the reader never wrote. A code span
// keeps its backticks, since those are the plain text way of saying that what
// is between them is code, and the alternative is a span that reads as prose.
//
// A run of markers is a delimiter only where it could open or close a span,
// which is what leaves arithmetic alone: the asterisks in "2 * 3" have a space
// on either side and are neither an opener nor a closer, and a row of figures
// is common in an answer about sizes. An underscore inside a word is never a
// delimiter, since the identifiers a model writes outnumber the words it
// emphasises, and a stripped underscore turns a name into something else.
//
// A backslash is copied along with the character it precedes, so that an
// escaped marker cannot open a span. The backslash is kept rather than
// dropped: PROVISIONAL, since removing it is what a renderer would normally
// do, but it changes the text and an escape is rare enough that the mark is
// cheaper than the edit.
func renderInline(s string) string {
	out, _ := scanInline(s, false)
	return out
}

// scanInline is renderInline, and when track is set it also reports where the
// emphasised spans and the code spans lie in the text it returns.
//
// The offsets are bytes into the returned text, since that is the text the rows
// are cut from: the markers that were stripped are not in it, so every offset
// after one has moved against the source. A link is recognised in the returned
// text rather than in the source, for the same reason, and never rewrites it.
// The text returned is the same with track set or clear.
func scanInline(s string, track bool) (string, []inlineMark) {
	r := []rune(s)
	var b strings.Builder

	// A closing run found while scanning ahead is remembered rather than
	// dropped where it was found, since the scan reaches it a second time on
	// its way past. They are kept on a stack because emphasis nests, and one
	// remembered run would be overwritten by the run inside it.
	var closing []int
	// opens is where each remembered closing run's span begins in the output.
	// It is pushed and popped with closing, so the two never differ in depth.
	var opens []int
	var emph, code []inlineMark

	for i := 0; i < len(r); {
		if n := len(closing); n > 0 && i == closing[n-1] {
			i += runLen(r, i, r[i])
			closing = closing[:n-1]
			if track {
				emph = append(emph, inlineMark{start: opens[n-1], end: b.Len(), role: roleEmphasis})
				opens = opens[:n-1]
			}
			continue
		}

		c := r[i]
		switch {
		case c == '\\' && i+1 < len(r) && isDelimiter(r[i+1]):
			b.WriteString(string(r[i : i+2]))
			i += 2
		case c == '`':
			n := runLen(r, i, '`')
			if end := findRun(r, i+n, '`', n); end >= 0 {
				// A code span is taken whole, so that a marker inside one
				// survives as the character it is rather than being read as
				// emphasis laid over code.
				from := b.Len()
				b.WriteString(string(r[i : end+n]))
				if track {
					code = append(code, inlineMark{start: from, end: b.Len(), role: roleCode})
				}
				i = end + n
				continue
			}
			// An unterminated span is a backtick in prose rather than a
			// delimiter, and is left where it stands.
			b.WriteRune(c)
			i++
		case isDelimiter(c):
			n := runLen(r, i, c)
			if n > maxDelimRun || !opensSpan(r, i, n, c) {
				b.WriteString(string(r[i : i+n]))
				i += n
				continue
			}
			end := findRun(r, i+n, c, n)
			if end < 0 || !closesSpan(r, end, n, c) {
				b.WriteString(string(r[i : i+n]))
				i += n
				continue
			}
			closing = append(closing, end)
			if track {
				opens = append(opens, b.Len())
			}
			i += n
		default:
			b.WriteRune(c)
			i++
		}
	}
	out := b.String()
	if !track {
		return out, nil
	}
	// The marks are returned in the order they are painted in, so a later one
	// wins where two cover the same byte: emphasis, then code, then a link.
	marks := append(emph, code...)
	return out, append(marks, findLinks(out, code)...)
}

// opensSpan reports whether a run of markers can begin an emphasised span.
//
// A run followed by a space cannot open one, which is what leaves arithmetic
// alone. An underscore cannot open one inside a word, since the underscores in
// a name are part of the name.
func opensSpan(r []rune, at, n int, c rune) bool {
	next := at + n
	if next >= len(r) || isSpace(r[next]) {
		return false
	}
	if c == '_' && at > 0 && isWordRune(r[at-1]) {
		return false
	}
	return true
}

// closesSpan reports whether a run of markers can end an emphasised span, under
// the rules read the other way round.
func closesSpan(r []rune, at, n int, c rune) bool {
	if at == 0 || isSpace(r[at-1]) {
		return false
	}
	if c == '_' && at+n < len(r) && isWordRune(r[at+n]) {
		return false
	}
	return true
}

// isDelimiter reports whether a character can open or close emphasis.
func isDelimiter(r rune) bool { return r == '*' || r == '_' }

// isBullet reports whether a character is one of the unordered list markers.
func isBullet(r rune) bool { return r == '-' || r == '*' || r == '+' }

// isDigit reports whether a character is a figure.
//
// The range is taken rather than the Unicode class, since the two digits a
// model writes in front of a full stop are ASCII, and a figure elsewhere on the
// line is content rather than a marker.
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// isSpace reports whether a character is whitespace.
//
// A line is split on its newlines before it is read here, so the only
// whitespace that matters is what can sit between two markers on one line.
func isSpace(r rune) bool { return unicode.IsSpace(r) }

// isWordRune reports whether a character is a letter or a figure, which is what
// makes it part of a name rather than part of a span.
func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// leadingSpaces counts the columns a line is indented by.
//
// A tab counts as one column, since the arithmetic here is in characters
// rather than in the columns a terminal would give it, and the renderer cuts
// its rows in characters as well.
func leadingSpaces(r []rune) int {
	n := 0
	for n < len(r) && isSpace(r[n]) {
		n++
	}
	return n
}

// runLen counts how many of a character stand together at a position.
func runLen(r []rune, at int, c rune) int {
	n := 0
	for at+n < len(r) && r[at+n] == c {
		n++
	}
	return n
}

// findRun returns where a run of exactly n of a character begins at or after
// from, or -1 when there is none.
//
// A run of a different length is skipped whole rather than tested at each of
// its characters, so that the single asterisk inside a strong span is not read
// as that span being closed.
func findRun(r []rune, from int, c rune, n int) int {
	for i := from; i < len(r); {
		if r[i] != c {
			i++
			continue
		}
		run := runLen(r, i, c)
		if run == n {
			return i
		}
		i += run
	}
	return -1
}
