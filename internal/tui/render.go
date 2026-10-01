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
	Provider  string
	Model     string
	Reasoning string
	Branch    string
	State     string
	Approval  string
	Context   string
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
	{"Reasoning", func(s Status) string { return s.Reasoning }},
	{"Branch", func(s Status) string { return s.Branch }},
	{"Status", func(s Status) string { return s.State }},
	{"Approval", func(s Status) string { return s.Approval }},
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

	line := strings.Join(parts, sep)
	if width > 0 && len(line) > width {
		line = trimToWidth(parts, sep, width)
	}
	if s.Host != "" && (width <= 0 || len(line)+len(s.Host)+3 <= width) {
		line += sep + s.Host
	}
	return line
}

// trimToWidth drops trailing fields until the line fits.
func trimToWidth(parts []string, sep string, width int) string {
	for len(parts) > 1 {
		candidate := strings.Join(parts, sep)
		if len(candidate) <= width {
			return candidate
		}
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts[0]) > width {
		return truncate(parts[0], width)
	}
	return parts[0]
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

	reply := f.Reply
	if f.Partial != "" {
		// A reply that is still arriving occupies the last line, so the
		// partial text is drawn there rather than appended as a new line for
		// every token.
		reply = append(append([]string{}, reply...), f.Partial)
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
