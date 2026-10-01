package tui

import (
	"fmt"
	"strings"
)

// Status holds the values shown in the status bar.
//
// Most fields are empty until the API client reports them. An empty field is
// rendered as a dash rather than being hidden, so that the layout stays stable
// as values arrive and a missing value is visible rather than ambiguous.
type Status struct {
	Provider string
	Model    string
	State    string
	// Credits is the remaining allowance reported by the key endpoint, as a
	// figure against a limit. It is not the conversation context, which is the
	// share of the model window a message occupies.
	Credits string
	// Context is the share of the model window the conversation occupies. It
	// is a percentage rather than a figure, since what matters is how close
	// the conversation is to the point where it must be compacted.
	Context string
	// TokensIn and TokensOut accumulate across the session rather than
	// describing one exchange, since a session is the unit a reader cares
	// about.
	TokensIn  string
	TokensOut string
	Host      string
}

// fields lists the status bar in the order the interface presents it.
//
// The order is fixed here rather than being configurable, because a status bar
// that reorders itself between runs cannot be read at a glance.
var fields = []struct {
	label string
	value func(Status) string
}{
	{"Provider", func(s Status) string { return s.Provider }},
	{"Model", func(s Status) string { return s.Model }},
	{"Status", func(s Status) string { return s.State }},
	{"Credits", func(s Status) string { return s.Credits }},
	{"Context", func(s Status) string { return s.Context }},
	{"In", func(s Status) string { return s.TokensIn }},
	{"Out", func(s Status) string { return s.TokensOut }},
}

// placeholder is shown for a value that is not yet known.
const placeholder = "-"

// StatusLine renders the status bar.
//
// A field is dropped when its label and placeholder together would not fit the
// width, so that the bar degrades by losing the least important fields rather
// than by wrapping onto a second line.
func StatusLine(s Status, width int) string {
	sep := " | "
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		value := f.value(s)
		if value == "" {
			value = placeholder
		}
		parts = append(parts, f.label+": "+value)
	}

	// The host is held back rather than appended. It is the field a reader
	// can most afford to lose, since it does not change while the session
	// runs, and appending it last meant a narrow bar dropped the token
	// counters before it instead.
	host := s.Host

	line := strings.Join(parts, sep)
	if width > 0 && len(line) > width {
		line = trimToWidth(parts, sep, runeWidth(line, width))
	}
	if host != "" && (width <= 0 || len(line)+len(host)+len(sep) <= width) {
		line += sep + host
	}
	return line
}

// dropOrder names the fields in the order they are sacrificed when the bar is
// too narrow.
//
// The order is by how much a reader loses, not by where the field sits. The
// token counters go before the allowance, since a running total is the figure
// most often watched, and the host goes before either, since it does not change
// while the session runs. Dropping by position instead would remove whichever
// field happened to be last, which was the token count.
var dropOrder = []string{"In", "Out", "Credits", "Context", "Status", "Model"}

// trimToWidth removes fields until the line fits, sacrificing dropOrder first.
func trimToWidth(parts []string, sep string, width int) string {
	kept := make([]string, len(parts))
	copy(kept, parts)

	for _, label := range dropOrder {
		if len(join(kept, sep)) <= width {
			break
		}
		for i, p := range kept {
			// The colon is part of the match, so dropping one label cannot
			// remove a different field whose label begins with the same
			// letters.
			if strings.HasPrefix(p, label+": ") {
				kept = append(kept[:i], kept[i+1:]...)
				break
			}
		}
	}

	line := join(kept, sep)
	if len(line) > width && len(kept) > 0 {
		// Everything droppable is gone and it still does not fit, so the
		// remainder is cut rather than left to wrap onto a second row.
		return truncate(kept[0], width)
	}
	return line
}

// join concatenates the parts, which is the empty string when there are none.
func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}

// runeWidth returns how many columns a line occupies on the terminal, which is
// not the same as its length in bytes.
//
// The rule is drawn from a box-drawing character, which is three bytes and one
// column, and a rule a byte count reports as three times too wide is a rule that
// runs off the terminal and wraps.
func runeWidth(s string, max int) int {
	n := 0
	for range s {
		n++
		if n >= max {
			return n
		}
	}
	return n
}

// tail returns the last n columns of s, marked with an ellipsis so that a
// reader can tell the line was cut rather than begun there.
func tail(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[len(r)-n:])
	}
	return "..." + string(r[len(r)-(n-3):])
}

// truncate shortens a string to width, marking the cut with an ellipsis.
//
// The cut is taken in characters rather than in bytes, since a reply carries
// multibyte text and a cut at a byte boundary would leave half a character on
// the row and count as wider than it is.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-3]) + "..."
}

// minHeightForDivision is the shortest terminal that still has room for the
// rows around the rule, once the pane and the prompt have taken theirs.
const minHeightForDivision = 8

// The rows below the pane are the status bar, the pasted rows, and the prompt.
// The division adds a blank, a rule, and another blank when it is drawn.
const (
	inputRowsBare    = 2
	inputRowsDivided = 5
)

// maxPasteRows is how many lines of a pasted block are shown before the rest
// are reported as a count instead. A paste of a thousand lines would otherwise
// fill the screen.
const maxPasteRows = 5

// headerRowCount is the height of the header above the reply pane: the title, a
// rule, and the status bar. It is a constant rather than a local so that the
// renderer and the search agree on how tall the pane is, since a jump that
// placed a match under the prompt would be worse than not jumping at all.
const headerRowCount = 3

// Frame is the whole interface at one moment.
type Frame struct {
	// Title is the heading shown at the top of the reply pane.
	Title string
	// Status is the bar shown at the bottom.
	Status Status
	// Reply holds the messages exchanged so far.
	Reply []string
	// Input is the line being composed, without the prompt.
	Input string
	// Hint is an optional message shown in place of the reply when the pane is
	// otherwise empty.
	Hint string
	// Partial is a reply that is still arriving. It is shown in place of the
	// last pane line, so that a slow model does not look idle.
	Partial string
	// Busy reports that a request is in flight, which the status bar shows.
	Busy bool
	// Delegate is the partial answer of a background question, shown below the
	// conversation while it arrives. It is kept apart from the reply because it
	// belongs to neither conversation and would be misleading among them.
	Delegate string
	// Spinner is the twiddle shown while work is in progress, empty when
	// idle. It is drawn beside the partial reply rather than in the status
	// bar, so that it moves where the eye already is.
	Spinner string
	// Pasted holds the lines of a paste that has landed but not yet been
	// submitted. They occupy their own rows above the prompt, since a paste
	// cannot be shown on one row and a prompt that silently swallowed it
	// would read as a lost paste.
	Pasted []string
	// Hints are the keys that do something in the current state, shown on one
	// row above the prompt. Only keys that act are named, so the row never
	// promises a key that does nothing. It is a list rather than a finished
	// string so that entries can be dropped whole when the row is too narrow,
	// which a single string could not be without being cut.
	Hints []string
	// Scroll is how many lines the pane is scrolled up from the newest
	// output. Zero means the view is following the bottom, which is the only
	// behaviour the pane had before scrolling existed.
	Scroll int
}

// scrolled reports whether the pane is scrolled back from the newest output.
func (f Frame) scrolled() bool { return f.Scroll > 0 }

// scrollMarker is shown beside the title while the view is scrolled back.
//
// Without it a reader who has scrolled cannot tell whether the pane is holding
// still or has simply run out of new output, and the only way to find out is to
// scroll down and watch whether anything moves.
const scrollMarker = "[scrolled back]"

// titleLine renders the heading, carrying the scroll marker when the view is
// not at the bottom.
//
// The marker is placed on the title row rather than on a row of its own, since
// a row taken for it would change the height of the pane the moment the user
// scrolled, and a pane that resizes under the reader is worse than no marker.
func titleLine(title string, width int, scrolled bool) string {
	if title == "" {
		title = "openrouter-cli"
	}
	if !scrolled {
		return truncate(title, width)
	}
	// The marker is cut from the right of the title before the title itself
	// is, since the title is the part that names the session.
	if width <= len(scrollMarker) {
		return truncate(scrollMarker, width)
	}
	keep := width - len(scrollMarker) - 1
	return truncate(title, keep) + " " + scrollMarker
}

// Render draws the frame and returns the lines to write.
//
// The reply pane is filled from the top and the newest lines are kept when the
// pane is too short, since the newest exchange is the one being read.
func Render(f Frame, height, width int) []string {
	if width < 1 {
		width = 1
	}

	// The hint row is rendered once, before the budget is made, so that the
	// budget reserves a row only when a row will actually be drawn.
	hints := hintLine(f.Hints, width)

	// The rows below the pane are the status bar, a blank, the rule, the
	// pasted rows, the prompt, and a blank. The pane is given what remains, so
	// that adding the division does not make the frame taller than the
	// terminal and push the status bar off the screen.
	divided := height >= minHeightForDivision

	// The rows the input block takes depend on whether the division is drawn
	// and on how much was pasted. A frame on a short terminal therefore gives
	// more of itself to the conversation rather than to decoration, since a
	// pane with no rows is not a pane, and a pasted block takes only what is
	// left once the prompt has been accounted for.
	inputRows := inputRowsBare
	if divided {
		inputRows = inputRowsDivided
	}
	inputRows += pasteRows(f.Pasted, height-inputRows)
	// The hint row is budgeted after the paste, so that a paste which has
	// landed is still reported. A paste the reader cannot see reads as a lost
	// paste, while a missing hint costs nothing beyond the row it was on.
	inputRows += hintRows(hints, height-inputRows)

	// The header is the model name, a rule, and the status bar. It is drawn
	// above the conversation and outside the scrolled slice, so it stays put
	// while the reader scrolls back through earlier output. At the foot it
	// scrolled away at exactly the moment the figures in it were wanted.
	//
	// The title is the first thing dropped on a terminal too short to hold
	// the whole header, since the pane is what a reader is reading and the
	// status bar carries the figures worth keeping. A frame that runs past
	// the bottom pushes the prompt off the screen, which leaves no way to type
	// a next message, so something in the header has to yield.
	headerRows := headerRowCount
	if height < headerRows+inputRows+1 {
		// The header yields before the prompt does, and the title is the first
		// row it gives up. A terminal too short to hold the header and the
		// prompt together keeps the prompt, since a reader with no prompt has
		// no way to write a next message.
		headerRows = maxInt(0, height-inputRows)
	}

	// The pane takes a row out of the row the budget holds below the prompt,
	// since that row is only drawn when the frame has not already filled the
	// height. Where the input block alone is taller than the terminal there is
	// nothing left to give, and the pane gives way to the prompt instead,
	// since a frame showing a pane and no way to type is one the reader cannot
	// continue.
	paneHeight := height - headerRows - inputRows
	if paneHeight < 1 {
		paneHeight = maxInt(0, height-headerRows-inputRows+1)
	}

	body := make([]string, 0, height)

	// Every reply entry is folded before the pane is filled, since folding
	// changes how many rows a reply occupies. Truncating instead would lose
	// whatever fell past the edge, which for prose is most of a paragraph.
	//
	// An entry may itself span several lines, since a reply is stored whole so
	// that a fenced code block stays recognisable.
	reply := make([]string, 0, len(f.Reply)*2)
	for _, entry := range f.Reply {
		reply = append(reply, WrapBlock(entry, width)...)
	}

	if f.Partial != "" {
		// A reply that is still arriving occupies the rows below the pane, so
		// it is folded the same way a finished reply is. Folding it here as
		// well matters for a code block: an unfinished reply is not yet known
		// to contain a fence, so folding it separately would reflow code that
		// must keep its own lines.
		reply = append(reply, WrapBlock(f.Partial, width)...)
	}
	if f.Delegate != "" {
		// A delegate that is still answering is labelled, so a line in the
		// pane is not mistaken for the answer to what was just asked.
		reply = append(reply, strings.TrimRight(f.Delegate, "\n"))
	}
	if f.Spinner != "" {
		// The twiddle leads the line it belongs to. It is placed before the
		// text so that the text does not shift sideways as the twiddle turns,
		// which a trailing one would cause.
		reply = append(append([]string{}, reply...), f.Spinner+" thinking")
	}
	if len(reply) == 0 && f.Hint != "" {
		// The hint is drawn as written rather than folded. Folding pads a
		// short line to the full width, which leaves a multi-line hint ragged
		// along its second row.
		hint := strings.Split(strings.TrimRight(f.Hint, "\n"), "\n")
		reply = make([]string, len(hint))
		copy(reply, hint)
	}
	// The offset is applied before the newest lines are kept, so that
	// scrolling reveals lines that were previously off the pane rather than
	// blank rows above the ones already shown.
	//
	// The offset is clamped so that at least a full pane of history remains.
	// Letting it run to the very end would leave a single line at the top of
	// an otherwise empty pane, whereas scrolling to the top should settle on
	// the oldest lines there are and fill the pane with them. The oldest line
	// is the stop rather than the end, so it is always kept.
	//
	// A conversation shorter than the pane clamps to zero rather than to its
	// own length. There is nothing above the first line to scroll to, so an
	// offset past the top is not a position that exists, and taking it literally
	// leaves the pane blank while the title still claims the view is scrolled
	// back from a view that is not scrolled at all.
	if maxScroll := len(reply) - paneHeight; f.Scroll > maxScroll {
		f.Scroll = maxInt(0, maxScroll)
	}

	// The header is drawn after the offset has been clamped, so that the
	// marker on the title row describes the view that is on screen.
	header := []string{
		titleLine(f.Title, width, f.scrolled()),
		rule(width),
		StatusLine(f.Status, width),
	}
	if headerRows < len(header) {
		header = header[len(header)-headerRows:]
	}
	body = append(body, header...)

	if f.Scroll > 0 {
		reply = reply[:len(reply)-f.Scroll]
	}
	if len(reply) > paneHeight {
		reply = reply[len(reply)-paneHeight:]
	}
	for len(body) < headerRows+paneHeight {
		var line string
		if i := len(body) - headerRows; i < len(reply) {
			line = reply[i]
		}
		// A folded line already fits. One that came from a code block may
		// not, and is cut rather than allowed to wrap, since a wrapped code
		// line would push the rest of the frame down.
		body = append(body, truncate(line, width))
	}

	// The input box is separated from the conversation by a blank row and a
	// rule. Without them the prompt sits directly under the last line of a
	// reply, and the two are read as one block: a reply ending mid-sentence
	// above a prompt reads as a single run of text rather than as an exchange.
	//
	// On a terminal too short to hold the division it is dropped rather than
	// drawn, since a frame taller than the screen pushes the status bar off the
	// top of it, and a status bar that cannot be seen is worse than a missing
	// rule.
	if divided {
		// The blank row above the rule gives it air, and the blank row below
		// it separates the rule from the prompt it belongs to. A rule touching
		// the prompt reads as a border of the prompt rather than as a
		// division of the screen.
		body = append(body, "")
		body = append(body, rule(width))
		body = append(body, "")
	}

	// A paste occupies the rows above the prompt. They are appended after the
	// pane is filled, so the pane gives up the rows rather than the frame
	// overflowing and pushing the status bar off the screen.
	for i, line := range f.Pasted {
		if i >= maxPasteRows {
			// The notice is indented the same way the lines it counts, so it
			// is read as part of the block rather than as another message.
			body = append(body, indent("", "  ", fmt.Sprintf(
				"... %d more pasted lines", len(f.Pasted)-maxPasteRows), width))
			break
		}
		body = append(body, indent("", "  ", line, width))
	}

	// The hint row sits above the prompt, so the prompt stays the last row of
	// the input block and the caret, which is placed on the last row, stays
	// on the prompt.
	//
	// The row is dropped whole rather than cut when the frame cannot hold it
	// and the prompt both. The budget above has already taken a row for it,
	// but the pane and the header are each floored at one row, so on a short
	// terminal the budget can still leave the frame too tall. Dropping the
	// hint here costs the reader a row of names, where letting the trim below
	// take the last row would cost them the prompt and with it any way to
	// type a next message.
	if hints != "" && len(body)+2 <= height {
		body = append(body, hints)
	}
	body = append(body, indent("> ", "", f.Input, width))

	// The row below the prompt is added only when the frame has not already
	// filled the height, so a short terminal is not pushed one row over.
	if len(body) < height {
		body = append(body, "")
	}
	// The frame is cut to the height as a last resort. Every row above is
	// placed with a budget, but a terminal shorter than the prompt block
	// leaves nothing to cut, and a frame past the bottom pushes the prompt
	// off the screen and leaves the reader unable to continue.
	return body[:minInt(len(body), height)]
}

// indent returns a row carrying a marker, a body, and a prefix, fitted to the
// width.
//
// The marker and the prefix are dropped before the body is cut, in that order,
// since the body is the part a reader typed. What is left is the tail of what
// was typed rather than a marker alone, which on a terminal of one or two
// columns would otherwise be all that could be shown.
func indent(marker, prefix, body string, width int) string {
	room := width - runeWidth(marker, width)
	if room < 0 {
		room = 0
	}
	room -= runeWidth(prefix, room)
	if room < 0 {
		room = 0
	}
	fit := truncate(body, room)
	// A body that did not fit keeps its tail. The marker leads the row, so the
	// part dropped from the front is the one already spoken for, and the part
	// at the end is the line being composed.
	if runeWidth(body, room+1) > room {
		fit = tail(body, room)
	}
	return truncate(marker+prefix+fit, width)
}

// ruleRune is the character a rule is drawn with. It is a box-drawing
// character rather than an ASCII dash, since a run of dashes reads as text and
// a rule reads as a rule.
const ruleRune = "─"

// rule draws a horizontal rule across the width.
//
// A light shade is used rather than a heavy one, since the rule divides the
// screen and a heavy line reads as an object in its own right rather than as
// a division.
func rule(width int) string {
	if width < 1 {
		return ""
	}
	return strings.Repeat(ruleRune, width)
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// pasteRows returns how many rows a pasted block will occupy, which is the
// lines shown plus the row reporting anything cut.
//
// The count is bounded by the room left after the prompt, since the prompt is
// what a reader needs in order to type the next message. A paste that cannot
// fit is cut and reported rather than allowed to push the prompt off the
// bottom of the screen, which would leave no way to continue the session.
func pasteRows(pasted []string, room int) int {
	if len(pasted) == 0 || room < 1 {
		return 0
	}
	rows := minInt(len(pasted), maxPasteRows)
	if len(pasted) > rows {
		rows++
	}
	return minInt(rows, maxInt(0, room))
}

// Draw writes the frame to w, one line per row, home first.
//
// Each line is truncated to the width so that a line cannot wrap onto the next
// row and push the layout out of alignment.
func (s *Screen) Draw(lines []string) {
	s.write(seqHome)
	// Each row is cleared before it is written. Without that, a repaint that
	// is shorter than the frame before it leaves the tail of the longer one
	// on screen, so the pane appears to hold two copies of a reply.
	for i, line := range lines {
		if i > 0 {
			s.write("\r\n")
		}
		s.write(seqResetAttr)
		s.write(seqClearLine)
		s.write(line)
	}
	// The remainder of the screen below the frame is cleared, since a shorter
	// frame would otherwise leave the bottom of a taller one behind.
	for i := len(lines); i < s.height; i++ {
		s.write("\r\n")
		s.write(seqClearLine)
	}
	s.write(seqHome)

	// The cursor is placed after the prompt on the last row, so that the
	// caret sits where the next character will appear. The column is taken
	// from the input row rather than from the first row, which is the title.
	//
	// A frame with no rows leaves the cursor where it is. There is no last row
	// to place it against, and a terminal too short to hold the frame is the
	// one case where guessing would put the caret somewhere meaningless.
	if len(lines) == 0 {
		return
	}
	last := lines[len(lines)-1]
	s.write(fmt.Sprintf("\x1b[%d;%dH", len(lines), minInt(len(last)+1, s.width)))
}

// minInt returns the smaller of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
