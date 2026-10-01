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
		line = trimToWidth(parts, sep, width)
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
var dropOrder = []string{"In", "Out", "Credits", "Status", "Model"}

// trimToWidth removes fields until the line fits, sacrificing dropOrder first.
func trimToWidth(parts []string, sep string, width int) string {
	kept := make([]string, len(parts))
	copy(kept, parts)

	for _, label := range dropOrder {
		if len(join(kept, sep)) <= width {
			break
		}
		for i, p := range kept {
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

// truncate shortens a string to width, marking the cut with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}

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
	// Spinner is the twiddle shown while work is in progress, empty when
	// idle. It is drawn beside the partial reply rather than in the status
	// bar, so that it moves where the eye already is.
	Spinner string
}

// Render draws the frame and returns the lines to write.
//
// The reply pane is filled from the top and the newest lines are kept when the
// pane is too short, since the newest exchange is the one being read.
func Render(f Frame, height, width int) []string {
	if height < 3 {
		height = 3
	}
	if width < 1 {
		width = 1
	}

	// One line for the title, one for the bar, one for the input, and one
	// blank between the title and the reply.
	paneHeight := height - 3
	if paneHeight < 1 {
		paneHeight = 1
	}

	body := make([]string, 0, height)
	title := f.Title
	if title == "" {
		title = "openrouter-cli"
	}
	body = append(body, truncate(title, width))

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
	if f.Spinner != "" {
		// The twiddle leads the line it belongs to. It is placed before the
		// text so that the text does not shift sideways as the twiddle turns,
		// which a trailing one would cause.
		reply = append(append([]string{}, reply...), f.Spinner+" thinking")
	}
	if len(reply) == 0 && f.Hint != "" {
		reply = []string{f.Hint}
	}
	if len(reply) > paneHeight {
		reply = reply[len(reply)-paneHeight:]
	}
	for len(body) < height-2 {
		var line string
		if i := len(body) - 1; i < len(reply) {
			line = reply[i]
		}
		// A folded line already fits. One that came from a code block may
		// not, and is cut rather than allowed to wrap, since a wrapped code
		// line would push the rest of the frame down.
		body = append(body, truncate(line, width))
	}

	body = append(body, StatusLine(f.Status, width))
	body = append(body, "> "+truncate(f.Input, maxInt(0, width-2)))
	return body
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
