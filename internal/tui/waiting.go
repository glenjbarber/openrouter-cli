package tui

import "strings"

// The waiting marker sits beside the input bar and says that the client is
// alive and waiting for a message.
//
// It answers the question the twiddle answers, and the two are kept apart. The
// twiddle says a request is in flight; the marker says the client is ready for
// the next one. An idle interface and a wedged one look identical from outside,
// so a resting state a reader can see at a glance is needed even when nothing
// is happening.
//
// The marker does not move. A twiddle beside it would read as work in
// progress, which is the opposite of what it is for, so the marker is drawn in
// characters that hold still and is told apart from the in-progress twiddle by
// its glyphs rather than by its motion.

// waitingMarker is the text drawn beside the input bar when nothing is in
// flight.
//
// Parenthetical rather than a twiddle or a bullet, and in plain characters
// rather than bold, so that it is not mistaken for the indicator beside the
// partial reply. The twiddle uses braille figures and turns; this uses
// characters that do not move, and a reader who has learned both tells them
// apart from the glyphs alone.
//
// Bold was authorized for the banner and is not used. The frame settles that
// rendered output is plain text and that a selection yields it with no escape
// sequence and no padding, and two tests enforce it by failing on any escape
// in any row. The marker reads as a banner by its position, by holding still,
// and by being on the prompt row at all times, rather than by a sequence drawn
// around it.
const waitingMarker = "(waiting)"

// waitingPromptMarker leads the input bar, and is the columns it takes before
// any of the composed text.
//
// It is named apart from the promptMarker below rather than sharing it, since
// the bar is built from two constants and a name that is used for both would
// not say which is which.
const waitingPromptMarker = "> "

// waitingAir is the column of air held between the composed text and the
// marker.
//
// One column, so that the marker reads as beside the bar rather than as a
// suffix of the text.
const waitingAir = 1

// waitingMinWidth is the narrowest terminal the marker is drawn on.
//
// The prompt marker and the air are counted as well as the marker itself, since
// a marker drawn against the prompt reads as part of the prompt rather than as
// beside it.
const waitingMinWidth = len(waitingPromptMarker) + len(waitingMarker) + waitingAir

// waitingReserved returns the columns the marker takes beside the input bar,
// or zero where the terminal is too narrow to carry it.
//
// The marker is on a column reserved for it rather than placed after the text
// being composed, since a marker that moved with the length of the line would
// not be beside anything. Reserving the column means the text is fitted to
// what is left, so a line as long as the terminal is wide still leaves the
// marker where the reader learned to look for it.
//
// The marker is dropped whole rather than cut where there is no room for it. A
// cut marker would read as "(wait" or as "(waitin", and a reader seeing that
// would take it for a message rather than for the marker they learned. The text
// keeps the columns instead, since a reader composing a message needs the room
// to type it more than they need to see the marker.
func waitingReserved(width int) int {
	if width < waitingMinWidth {
		return 0
	}
	return len(waitingMarker)
}

// waitingRow draws the input bar with the waiting marker beside it.
//
// The name is spelled out rather than shortened, since promptRow is the name a
// test in this package already uses for the helper that finds the prompt on a
// drawn frame, and two things in one package cannot both answer to it.
func waitingRow(input string, width int) string {
	held := waitingReserved(width)
	if held == 0 {
		// The marker is dropped, and the row is the one the bar has always
		// been, so a terminal too narrow to carry the marker behaves exactly
		// as it did before the marker existed.
		return indent(waitingPromptMarker, "", input, width)
	}

	room := width - held - waitingAir
	bar := indent(waitingPromptMarker, "", input, room)
	gap := width - displayWidth(bar) - held
	if gap < waitingAir {
		gap = waitingAir
	}
	return bar + strings.Repeat(" ", gap) + waitingMarker
}

// waitingCaret returns the column the caret belongs at within a drawn input row,
// counting from one.
//
// The column is measured back from the marker on the right rather than forward
// from the left, since the columns between the text and the marker are air and
// a reader may leave air at the end of what they are typing. Measuring from the
// left finds nothing, because nothing in the row distinguishes a space the
// reader typed from a space the layout put there.
//
// The figure this returns is where the reserved region begins rather than
// where the text ended, so it is right only where the text filled the row it
// was given. That is the known defect in this function, and it is why it is not
// called from anywhere: a caret placed from it sits on the padding rather than
// after the last character. Fixing it needs the column worked out where the
// text is in hand and carried out of the renderer, which is a change to Frame
// rather than to this file.
func waitingCaret(row string, width int) int {
	held := waitingReserved(width)
	if held == 0 {
		return minInt(displayWidth(row)+1, width)
	}
	at := displayWidth(row) - held - waitingAir
	return minInt(maxInt(at, 1), width)
}
