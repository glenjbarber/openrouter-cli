package tui

import (
	"fmt"
	"strings"
	"unicode"
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
	// Approval is what the client will run without asking. It is here
	// because the client now does run programs, and a reader watching a
	// model ask to build something wants to know at a glance whether it will
	// be stopped and asked first.
	Approval string
	Host     string
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
	{"Approval", func(s Status) string { return s.Approval }},
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
	if width > 0 && displayWidth(line) > width {
		line = trimToWidth(parts, sep, width)
	}
	if host != "" && (width <= 0 || displayWidth(line)+displayWidth(host)+displayWidth(sep) <= width) {
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
var dropOrder = []string{"In", "Out", "Approval", "Credits", "Context", "Status", "Model"}

// trimToWidth removes fields until the line fits, sacrificing dropOrder first.
func trimToWidth(parts []string, sep string, width int) string {
	kept := make([]string, len(parts))
	copy(kept, parts)

	for _, label := range dropOrder {
		if displayWidth(join(kept, sep)) <= width {
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
	if displayWidth(line) > width && len(kept) > 0 {
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

// runeWidth returns how many characters a line carries, up to a bound.
//
// It counts characters rather than columns, and it is used where the string is
// one the interface wrote and holds nothing but ASCII, so the two agree: the
// names on the hint row, and the marker and the prefix of a row, which are a
// marker and a space. Anything a terminal will show and a reader will measure
// is counted with displayWidth instead.
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

// displayWidth returns how many columns a line occupies on the terminal.
//
// A character count is not a column count. A character in the East Asian wide
// ranges takes two columns, and a combining mark takes none at all, since it
// is drawn over the character before it. A reply carrying either was measured
// too wide when it was counted in characters, so the row overflowed the pane,
// the terminal wrapped it, and everything below it moved down.
//
// A byte count is worse, since the rule is three bytes for one column and a
// byte count reports a rule three times too wide.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeColumns(r)
	}
	return n
}

// runeColumns returns how many columns one character occupies.
func runeColumns(r rune) int {
	switch {
	case r < 0x20 || (r >= 0x7f && r <= 0x9f):
		// A control character is drawn in no column of its own. The frame
		// drops these before it is drawn, so the case is here only so that a
		// width cannot be counted from a string that still carries one.
		return 0
	case isZeroWidth(r):
		return 0
	case isWide(r):
		return 2
	default:
		return 1
	}
}

// wideRanges are the ranges a terminal in a UTF-8 mode gives two columns to:
// the East Asian wide and fullwidth forms, the Hangul syllables, and the
// emoji.
//
// The whole table of every script is not reproduced. A character outside these
// ranges is measured as one column, which is the common case and the safe way
// to be wrong, since a row measured too wide is cut while a row measured too
// narrow wraps.
var wideRanges = [...][2]rune{
	{0x1100, 0x115f},   // Hangul jamo, initial.
	{0x2e80, 0x303e},   // CJK radicals and punctuation.
	{0x3041, 0x33ff},   // Kana and CJK compatibility.
	{0x3400, 0x4dbf},   // CJK extension A.
	{0x4e00, 0x9fff},   // CJK unified ideographs.
	{0xa000, 0xa4cf},   // Yi.
	{0xa960, 0xa97f},   // Hangul jamo, extended A.
	{0xac00, 0xd7a3},   // Hangul syllables.
	{0xf900, 0xfaff},   // CJK compatibility ideographs.
	{0xfe10, 0xfe19},   // Vertical forms.
	{0xfe30, 0xfe6f},   // CJK compatibility and small forms.
	{0xff00, 0xff60},   // Fullwidth forms.
	{0xffe0, 0xffe6},   // Fullwidth signs.
	{0x1f300, 0x1f64f}, // Emoji.
	{0x1f680, 0x1f6ff}, // Transport and map symbols.
	{0x1f900, 0x1f9ff}, // Supplemental symbols.
	{0x20000, 0x3fffd}, // CJK extensions B and beyond.
}

// isWide reports whether a character takes two columns.
func isWide(r rune) bool {
	for _, span := range wideRanges {
		if r >= span[0] && r <= span[1] {
			return true
		}
	}
	return false
}

// isZeroWidth reports whether a character is drawn over the one before it
// rather than in a column of its own.
//
// A combining mark is the case the interface meets most, since an accented
// character reaches it in the decomposed form with the accent as a mark of its
// own. The rest are the joiner and the variation selectors, which a reply
// carrying an emoji reaches through.
func isZeroWidth(r rune) bool {
	if unicode.In(r, unicode.Mn, unicode.Me) {
		return true
	}
	return r == 0x200b || r == 0x200c || r == 0x200d || r == 0xfeff ||
		(r >= 0x2060 && r <= 0x2064) || (r >= 0xfe00 && r <= 0xfe0f)
}

// ellipsis marks a row that was cut rather than begun there. It is counted
// once, as the three columns it occupies, and never as part of the text it
// stands in front of.
const ellipsis = "..."

// fitPrefix returns the leading part of s that fits in width columns.
//
// A character is taken whole, so a two-column character is not left half past
// the edge, and a mark is kept with the character it belongs to rather than
// left at the head of the row on its own.
func fitPrefix(s string, width int) string {
	if width <= 0 {
		return ""
	}
	n := 0
	for i, r := range s {
		w := runeColumns(r)
		if n+w > width {
			return s[:i]
		}
		n += w
	}
	return s
}

// fitSuffix returns the trailing part of s that fits in width columns, on the
// same terms as fitPrefix.
//
// The scan starts at the end and stops at a character that takes columns of
// its own, so that the marks following it are kept with it rather than cut
// off the front of what is left.
func fitSuffix(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	start, n := len(r), 0
	for i := len(r) - 1; i >= 0; i-- {
		w := runeColumns(r[i])
		if w == 0 {
			continue
		}
		if n+w > width {
			start = i + 1
			break
		}
		n += w
		start = i
	}
	return string(r[start:])
}

// tail returns the last columns of s, marked with an ellipsis so that a
// reader can tell the line was cut rather than begun there.
func tail(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if displayWidth(s) <= n {
		return s
	}
	if n <= len(ellipsis) {
		return fitSuffix(s, n)
	}
	return ellipsis + fitSuffix(s, n-len(ellipsis))
}

// truncate shortens a string to width columns, marking the cut with an
// ellipsis.
//
// The cut is taken in columns, since a reply carries multibyte text and a cut
// in bytes would leave half a character on the row. The ellipsis is counted
// once: the text is shortened by the columns the ellipsis occupies, so the
// result is the width asked for and no wider.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	if width <= len(ellipsis) {
		return fitPrefix(s, width)
	}
	return fitPrefix(s, width-len(ellipsis)) + ellipsis
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

// maxBlockRows is how many lines of a block above the prompt are shown before
// the rest are reported as a count instead. A paste of a thousand lines, or a
// queue that has grown while a slow model works, would otherwise fill the
// screen.
const maxBlockRows = 5

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
	// Tint is the sequence that colours the twiddle, empty when there is none.
	//
	// It is carried on the frame rather than written into Spinner, since a
	// frame is also rendered into plain text and a sequence inside the figure
	// would have to be stripped again before the row could be measured,
	// searched or copied. The screen applies it, having written the bytes.
	Tint string
	// Pasted holds the lines of a paste that has landed but not yet been
	// submitted. They occupy their own rows above the prompt, since a paste
	// cannot be shown on one row and a prompt that silently swallowed it
	// would read as a lost paste.
	Pasted []string
	// Queued holds the lines committed while a model was working, which are
	// waiting for that request to end. They take their own rows above the
	// prompt, since a line the reader has sent and cannot see is a line they
	// would send again, and since the decision to stop a model with them
	// cannot be made against a queue that is not shown.
	Queued []string
	// Hints are the keys that do something in the current state, shown on one
	// row above the prompt. Only keys that act are named, so the row never
	// promises a key that does nothing. It is a list rather than a finished
	// string so that entries can be dropped whole when the row is too narrow,
	// which a single string could not be without being cut.
	Hints []string
	// Confirm is a question put to the reader, drawn in the input block above
	// the prompt rather than written into the pane.
	//
	// The input block is where it belongs. A question is about what the reader
	// is about to do, and a question written into the pane scrolls back into
	// the history the moment a reply arrives, which is about the moment a
	// reader answering it would need to read it again. The pane is the
	// conversation; the input block is the reader's own side of the screen.
	//
	// It is one row, budgeted with the rest of the block, so that a question
	// cannot push the prompt off the bottom of the screen.
	Confirm string
	// ConfirmChoice is the option the reader has moved to with Tab, shown on
	// the input line in place of what is being composed, so that the choice
	// being given is in the same place as the question asking for it. It is
	// empty until Tab has chosen one, which is what makes the default no:
	// nothing is preselected, so a reader who answers without reaching for
	// Tab has approved nothing.
	ConfirmChoice string
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
// Render draws a frame at a size and returns the rows to write.
func Render(f Frame, height, width int) []string {
	rows, _, _ := render(f, height, width)
	return rows
}

// render draws a frame and reports the offset it drew it at. The offset is
// returned because it is clamped here, and the session keeps its own copy of
// it: an offset the renderer silently reduced would leave the session holding
// a position the reader cannot see, and coming back down from it would take a
// notch per line rather than per screen.
// render draws a frame, reports the offset it drew it at, and reports the row
// carrying the twiddle.
//
// The twiddle row is reported rather than coloured here. Every row leaves this
// function as plain text with the bytes a terminal would act on removed, and a
// sequence inserted before that would be stripped along with the ones a model
// sent. The screen applies it, having written the bytes.
func render(f Frame, height, width int) ([]string, int, int) {
	if width < 1 {
		width = 1
	}

	// The hint row is rendered once, before the budget is made, so that the
	// budget reserves a row only when a row will actually be drawn.
	hints := hintLine(f.Hints, width)

	// The question is rendered once, on the same terms as the hint row, so
	// that the budget reserves a row for it only when one will be drawn.
	confirm := confirmLine(f, width)

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
	// The rows a block above the prompt takes are held onto, rather than
	// recomputed at the point where it is drawn. The budget is what keeps the
	// block from pushing the prompt off the bottom of the screen, so the
	// drawing has to be held to the same figure the budget came to.
	pasteBudget := blockRows(len(f.Pasted), height-inputRows)
	inputRows += pasteBudget
	// The queue is budgeted after the paste. A paste has landed and is in
	// front of the reader, while the queue is what the reader is about to
	// decide about, and either order leaves the prompt where it belongs.
	queueBudget := blockRows(len(f.Queued), height-inputRows)
	inputRows += queueBudget
	// The hint row is budgeted after the paste, so that a paste which has
	// landed is still reported. A paste the reader cannot see reads as a lost
	// paste, while a missing hint costs nothing beyond the row it was on.
	inputRows += hintRows(hints, height-inputRows)
	// The question is budgeted after the hint row. A hint that gives way to a
	// question is a caption lost for a moment, while a question that gives way
	// to a hint is a decision the reader cannot see being asked of them,
	// which is the one failure here worth costing a row for.
	inputRows += confirmRows(confirm, height-inputRows)

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
	// twiddleLine is the row the twiddle is added as, empty where there is no
	// twiddle. It is the text rather than an index for the reason given below
	// where it is set.
	twiddleLine := ""
	if f.Spinner != "" {
		// The twiddle leads the line it belongs to. It is placed before the
		// text so that the text does not shift sideways as the twiddle turns,
		// which a trailing one would cause.
		//
		// The line is named rather than numbered. The pane is trimmed and
		// padded below, so the index it is appended at is not the one it is
		// finally drawn on, and an index carried through those two operations
		// is a second thing to keep correct. The text is found instead, once,
		// after the pane is settled.
		twiddleLine = f.Spinner + " thinking"
		reply = append(append([]string{}, reply...), twiddleLine)
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
	//
	// The block is drawn to the rows the budget allowed rather than to the
	// fixed maximum on its own. The maximum bounds a paste on a terminal with
	// room to spare, and on one without it the block took rows the prompt
	// needed, so a paste of any size could push the prompt off the bottom and
	// leave the reader with no way to write a next message.
	pasteShown, pasteNotice := blockLayout(len(f.Pasted), pasteBudget)
	for i := 0; i < pasteShown; i++ {
		// The lines are indented the same way as the notice that counts the
		// rest, so that the block reads as one thing rather than as a message.
		body = append(body, indent("", "  ", f.Pasted[i], width))
	}
	if pasteNotice {
		body = append(body, indent("", "  ", fmt.Sprintf(
			"... %d more pasted lines", len(f.Pasted)-pasteShown), width))
	}

	// A queued message takes its own rows above the prompt, below the paste it
	// arrived with. Each is drawn as it was committed rather than folded into one
	// line, since the decision to stop a model with them is made against what
	// they say.
	queueShown, queueNotice := blockLayout(len(f.Queued), queueBudget)
	for i := 0; i < queueShown; i++ {
		body = append(body, indent(queuedMarker, "  ", f.Queued[i], width))
	}
	if queueNotice {
		body = append(body, indent("", "  ", fmt.Sprintf(
			"... %d more queued messages", len(f.Queued)-queueShown), width))
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
	body = append(body, indent("> ", "", confirmInput(f), width))

	// The row below the prompt is added only when the frame has not already
	// filled the height, so a short terminal is not pushed one row over.
	if len(body) < height {
		body = append(body, "")
	}
	// The frame is cut to the height as a last resort. Every row above is
	// placed with a budget, but a terminal shorter than the prompt block
	// leaves nothing to cut, and a frame past the bottom pushes the prompt
	// off the screen and leaves the reader unable to continue.
	rows := body[:minInt(len(body), height)]
	// The rows leave here as plain text. A reply is written by a model, and a
	// model passes on whatever it was given, so a row can carry a byte the
	// terminal would act on rather than show. Render is the last place the
	// frame is whole, and it is the boundary between text from elsewhere and
	// the terminal, so it is where such a byte is dropped.
	for i, row := range rows {
		rows[i] = plainRow(row)
	}

	// The twiddle row is found in the finished frame rather than tracked
	// through the trims above. The pane drops lines from the front and adds
	// blanks at the back, and an index moved by hand through both is a second
	// thing to keep right; matching the line here is one comparison against a
	// frame that has already stopped moving.
	//
	// The pane occupies the rows after the header, so the offset is where the
	// pane begins. A frame too short to hold it reports no row, since a tint
	// naming a row that does not exist would colour whatever took its place.
	tinted := -1
	if twiddleLine != "" && headerRows+paneHeight <= len(rows) {
		for i := headerRows; i < headerRows+paneHeight; i++ {
			if rows[i] == twiddleLine {
				tinted = i
				break
			}
		}
	}
	return rows, f.Scroll, tinted
}

// confirmLine renders the question onto one row above the prompt.
//
// The caller spells the question with its options, since what the options are
// and what they mean differs between the questions being asked, and a helper
// that hardcoded them would be wrong for at least one of them.
func confirmLine(f Frame, width int) string {
	if f.Confirm == "" {
		return ""
	}
	return truncate(f.Confirm, width)
}

// confirmRows is the single row a question takes, or none when there is no room.
//
// The row is budgeted with the rest of the input block rather than taken from
// the pane afterwards, on the same terms as the hint row. The prompt is what a
// reader needs in order to answer, so a row that only names a question yields
// to it rather than the other way round.
func confirmRows(line string, room int) int {
	if line == "" || room < 1 {
		return 0
	}
	return 1
}

// confirmInput returns what the input line shows while a question is open.
//
// The line carries the option the reader has moved to with Tab, in place of
// the line being composed, since nothing is being composed while a question is
// open. The choice is shown where the reader is already looking rather than
// on the question row alone, so that the answer being given is beside the
// question asking for it.
//
// Nothing is shown until Tab has chosen one, which is what makes the default
// no: a reader who answers without reaching for Tab has chosen nothing, and a
// reader who answered with the choice showing would know what they were
// giving.
func confirmInput(f Frame) string {
	if f.Confirm == "" {
		return f.Input
	}
	if f.ConfirmChoice == "" {
		return ""
	}
	return f.ConfirmChoice
}

// plainRow removes the bytes a terminal would act on from a row.
//
// The frame is plain text, and a selection taken out of the pane is copied as
// whatever is on the screen. An escape written into a row would move the
// cursor, restyle the rest of the frame, or retitle the window, and would be
// copied out along with the prose around it. A carriage return left at the end
// of a line, which is what a reply written with Windows line endings carries,
// is the same kind of byte: the terminal acts on it and the selection keeps
// it.
//
// A tab is kept. It is a column of space rather than a sequence, nothing acts
// on it, and dropping it would fold a line that was laid out with one.
func plainRow(s string) string {
	if !strings.ContainsFunc(s, isDroppedByte) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isDroppedByte(r) {
			return -1
		}
		return r
	}, s)
}

// isDroppedByte reports whether a rune is one the frame drops.
//
// The range is the control characters and the two blocks that are treated as
// controls: the C0 set at the start of the byte range, and the C1 set at the
// end of Latin-1. Everything else is text, including the box-drawing and
// braille figures the interface draws with.
//
// The name says what it answers rather than what it selects, since plainRow
// drops a rune this reports and keeps one it does not, and a name reading the
// other way round would have a reader keep exactly the bytes meant to go.
func isDroppedByte(r rune) bool {
	if r == '\t' {
		return false
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
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
	if displayWidth(body) > room {
		fit = tail(body, room)
	}
	return truncate(marker+prefix+fit, width)
}

// queuedMarker leads a row holding a message waiting for the model to finish.
//
// The marker is a word rather than a decoration, since a row is plain text a
// terminal selection copies out. A reader who selects the block above the
// prompt should get the message back marked as queued rather than as
// something the model was asked.
const queuedMarker = "queued "

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

// blockRows returns how many rows a block above the prompt will occupy, which
// is the lines shown plus the row reporting anything cut.
//
// The block is a paste that has landed or a queue of messages waiting on a
// model. Both are held to the same budget, since both would otherwise take
// the rows the prompt needs.
//
// The count is bounded by the room left after the prompt, since the prompt is
// what a reader needs in order to type the next message. A block that cannot
// fit is cut and reported rather than allowed to push the prompt off the
// bottom of the screen, which would leave no way to continue the session.
func blockRows(n, room int) int {
	if n == 0 || room < 1 {
		return 0
	}
	rows := minInt(n, maxBlockRows)
	if n > rows {
		rows++
	}
	return minInt(rows, maxInt(0, room))
}

// blockLayout decides how many lines of a block above the prompt are drawn and
// whether the rest of it are reported.
//
// The lines shown are bounded by the rows the budget allowed as well as by the
// fixed maximum, since the maximum says what a terminal with room to spare
// shows while the budget says what this one has. Where the budget is the
// smaller of the two it is the one that decides, because the rows past it
// would come out of the prompt.
//
// The overflow is reported whenever a row is left to report it in, since a
// block cut without saying so reads as one that arrived short. A row spent on
// the notice is a line of the block given up for it, which is the right trade:
// a reader told a block was cut can ask for the rest, while a reader told
// nothing cannot know anything was lost.
func blockLayout(n, budget int) (shown int, notice bool) {
	if n <= 0 || budget < 1 {
		return 0, false
	}
	shown = minInt(minInt(n, maxBlockRows), budget)
	if n <= shown {
		return shown, false
	}
	if shown+1 > budget {
		shown--
	}
	return shown, true
}

// Draw writes the frame to w, one line per row, home first.
//
// Each line is truncated to the width so that a line cannot wrap onto the next
// row and push the layout out of alignment.
func (s *Screen) Draw(lines []string) {
	s.DrawTinted(lines, tint{})
}

// tint names the row carrying colour, the sequence that colours it, and the
// figure the colour covers.
//
// It is handed to the screen rather than applied by the renderer, since the
// renderer builds rows as text and every row leaves it with the bytes a
// terminal would act on removed. The screen is the one place that writes bytes
// rather than text, so it is the one place a sequence belongs.
type tint struct {
	// row is the index of the row the sequence applies to, or negative where
	// there is none to apply it to.
	row int
	// sequence is written before the figure and reset immediately after it,
	// so the colour reaches the twiddle alone.
	sequence string
	// figure is the leading text of the row in colour, which is the twiddle.
	//
	// It is the text rather than a count of columns, because the twiddle is
	// drawn from braille figures and those are several bytes each. A count of
	// bytes would cut one in half, and the terminal would draw half a glyph
	// and the rest of it as text.
	figure string
}

// DrawTinted draws the rows, colouring one figure in part.
//
// A tint naming no row, no sequence, or no figure draws exactly what Draw
// would. That is the ordinary case: most frames carry no twiddle, and every
// frame outside a turn carries none.
func (s *Screen) DrawTinted(lines []string, t tint) {
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
		if t.sequence != "" && t.figure != "" && i == t.row &&
			strings.HasPrefix(line, t.figure) {
			// The colour is written around the figure rather than around the
			// row. The row holds the twiddle and the word beside it, and the
			// word is prose a reader copies out, so colouring it would put a
			// sequence into a selection.
			s.write(t.sequence)
			s.write(t.figure)
			s.write(seqResetAttr)
			s.write(line[len(t.figure):])
			continue
		}
		s.write(line)
	}
	// The remainder of the screen below the frame is cleared, since a shorter
	// frame would otherwise leave the bottom of a taller one behind.
	for i := len(lines); i < s.height; i++ {
		s.write("\r\n")
		s.write(seqClearLine)
	}
	s.write(seqHome)

	// The cursor is placed after the prompt on the last row of the input
	// block, so that the caret sits where the next character will appear.
	// The column is taken from the input row rather than from the first row,
	// which is the title.
	//
	// A frame with no rows leaves the cursor where it is. There is no last row
	// to place it against, and a terminal too short to hold the frame is the
	// one case where guessing would put the caret somewhere meaningless.
	if len(lines) == 0 {
		return
	}
	// The row holding the prompt is found rather than assumed, because the
	// row below the prompt is drawn whenever the frame has not filled the
	// height. Placing the caret on the last row would then put it one row
	// under the prompt, on the blank the layout leaves there, and a reader
	// typing would watch the character appear away from what they are typing
	// into.
	//
	// The row is the last one with anything on it, because everything drawn
	// after the prompt is that blank: the hint row and the pasted lines are
	// drawn above it, and the record puts both there so that the prompt stays
	// the last row of the input block.
	row := len(lines)
	for row > 0 && lines[row-1] == "" {
		row--
	}
	if row == 0 {
		return
	}
	last := lines[row-1]
	s.write(fmt.Sprintf("\x1b[%d;%dH", row, minInt(displayWidth(last)+1, s.width)))
}

// minInt returns the smaller of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
