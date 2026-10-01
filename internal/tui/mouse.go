package tui

import (
	"os"
	"strconv"
)

// Mouse event kinds the interface acts on.
const (
	// mouseNone reports a sequence that carries no action the interface
	// handles, such as a drag or a button press.
	mouseNone = iota
	// mouseUp is a wheel notch scrolling towards older output.
	mouseUp
	// mouseDown is a wheel notch scrolling towards newer output.
	mouseDown
)

// mouseStateMask is the wheel bit the terminal adds to the button number in an
// X10 report. A modern terminal sends the SGR form instead, but the offset is
// recorded because an X10 report is still the form a terminal falls back to
// when it cannot use SGR.
const mouseStateMask = 32

// wheel buttons, as the button number arrives with the motion bit applied.
const (
	wheelUp   = 64
	wheelDown = 65
)

// scrollStep is how many lines one wheel notch moves the view.
//
// Three is small enough that a single notch does not throw the reader past the
// paragraph they are on, and large enough that reaching the top of a long
// conversation does not take a hundred notches.
const scrollStep = 3

// parseMouse reports what an input sequence asks the mouse to do.
//
// The sequence is examined whole rather than byte by byte, so that a report
// arriving in one read is never mistaken for a keypress. Anything that is not
// a wheel report yields mouseNone, which leaves the sequence to be handled as
// ordinary input.
func parseMouse(seq []byte) int {
	switch {
	case len(seq) >= 3 && seq[0] == keyEscape && seq[1] == '[' && seq[2] == '<':
		button, ok := sgrButton(seq)
		if !ok {
			return mouseNone
		}
		return wheelDirection(button)
	case len(seq) == 6 && seq[0] == keyEscape && seq[1] == '[' && seq[2] == 'M':
		// The X10 form carries a single byte of button, so the wheel is
		// offset by 32 rather than by the two digits SGR uses. The value is
		// 32 more than 64 at most, so it is folded back with a mask rather
		// than a subtraction.
		return wheelDirection(int(seq[3]) &^ mouseStateMask)
	}
	return mouseNone
}

// sgrButton returns the button number carried by an SGR report.
//
// The SGR form is the one modern terminals send, since it is not limited to
// the coordinate range of the older three-byte form. Only the button is
// needed, so the coordinates are not read.
func sgrButton(seq []byte) (int, bool) {
	end := -1
	for i := 3; i < len(seq); i++ {
		if seq[i] == ';' {
			end = i
			break
		}
		if seq[i] < '0' || seq[i] > '9' {
			return 0, false
		}
	}
	if end < 0 {
		return 0, false
	}
	button, err := strconv.Atoi(string(seq[3:end]))
	if err != nil {
		return 0, false
	}
	return button, true
}

// wheelDirection reports which way a button number scrolls, if it is a wheel.
func wheelDirection(button int) int {
	switch button {
	case wheelUp:
		return mouseUp
	case wheelDown:
		return mouseDown
	}
	return mouseNone
}

// takeMouseSequence consumes a complete mouse report at the front of buf.
//
// It reports the sequence and the bytes left over, so that a reader holding a
// buffer which begins with a report can take the report and keep the text that
// followed it. A leading byte that is not a report is left alone, since
// ordinary input shares the same stream.
func takeMouseSequence(buf []byte) ([]byte, []byte, bool) {
	if len(buf) == 0 || buf[0] != keyEscape {
		return nil, buf, false
	}
	if len(buf) < 3 || buf[1] != '[' {
		return nil, buf, false
	}

	// The SGR form ends at M or m, whichever comes first, and carries its
	// fields as digits and semicolons between them.
	if buf[2] == '<' {
		for i := 3; i < len(buf); i++ {
			if buf[i] == 'M' || buf[i] == 'm' {
				seq := buf[:i+1]
				return seq, buf[i+1:], true
			}
			if buf[i] != ';' && (buf[i] < '0' || buf[i] > '9') {
				return nil, buf, false
			}
		}
		// The report has not arrived whole, so nothing is taken. Returning
		// false leaves the bytes for a later read, which is what keeps a
		// report split across two reads from being read as keys.
		return nil, buf, false
	}

	// The X10 form is a fixed width, so it is taken once its six bytes are
	// present and never a byte earlier.
	if buf[2] == 'M' {
		if len(buf) < 6 {
			return nil, buf, false
		}
		return buf[:6], buf[6:], true
	}
	return nil, buf, false
}

// InTmux reports whether the session is running inside tmux.
//
// The environment is consulted rather than the terminal type, since tmux sets
// it for every pane it opens and a terminal cannot otherwise be asked.
func InTmux() bool { return os.Getenv("TMUX") != "" }

// scrolledBy returns the offset after one wheel notch.
//
// It is a function rather than an assignment so that the arithmetic can be
// tested without a terminal, and so that the clamping is stated once. The
// offset is never negative: a negative value would show blank rows above the
// newest output rather than stop at the bottom.
func scrolledBy(offset, direction int) int {
	switch direction {
	case mouseUp:
		return offset + scrollStep
	case mouseDown:
		if offset <= scrollStep {
			return 0
		}
		return offset - scrollStep
	}
	return offset
}

// mousePrefix reports whether the bytes so far could still grow into a mouse
// report, and so must be held rather than returned as keys.
//
// A lone escape is deliberately not one. It is a key the user pressed, and
// treating it as a report would swallow the interrupt it stands for. Only the
// SGR form can be held, since it is variable in length and is therefore the one
// a short read can split.
func mousePrefix(buf []byte) bool {
	if len(buf) == 0 || buf[0] != keyEscape {
		return false
	}

	// A prefix too short to tell a report from a lone escape is still a
	// candidate. Over a slow link a report arrives one byte at a time, and a
	// two-byte buffer cannot yet be recognised, so requiring three bytes here
	// is what returned the opening escape as a key and ended the session.
	if len(buf) < 3 {
		return len(buf) == 1 || buf[1] == '['
	}

	// Beyond the bracket the form decides. A report is the angle form. The
	// six-byte form is not held, since it is a fixed width and is taken whole
	// or not at all. Anything else after the bracket is an arrow or another
	// escape sequence, and holding it would swallow the key rather than a
	// report.
	if buf[2] != '<' {
		return false
	}

	// The fields after the angle marker are digits and semicolons, and the
	// report ends at M or m. Any other byte means these are not a report at
	// all. A report that has ended is not held either: it is taken whole by
	// the caller rather than being waited for.
	for i := 3; i < len(buf); i++ {
		switch c := buf[i]; {
		case c >= '0' && c <= '9', c == ';':
			continue
		default:
			return false
		}
	}
	return true
}
