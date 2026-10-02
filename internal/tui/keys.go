package tui

import "strings"

// Special key sequences a terminal sends for keys that are not bytes.
//
// The reader is in raw mode with echo disabled, so every key arrives as bytes
// and a sequence must be consumed whole. A sequence handled one byte at a time
// leaves its opening escape to be read as a key, and a lone escape ends the
// line: pressing an arrow would close the session.

// keyFinals are the final bytes of the sequences this reader acts on.
//
// The sequences are the ones in common use by terminal emulators: the four
// arrows and the two ends of the line. A sequence ending in any other byte is
// consumed and discarded rather than being read as keys, so the home and end
// keys, which a terminal writes the same way as an arrow, need no constant of
// their own.
const (
	keyUp    = 'A'
	keyDown  = 'B'
	keyRight = 'C'
	keyLeft  = 'D'
)

// keyShiftEnter is the queued marker for a shift with enter.
//
// It is not a byte a terminal sends, since a terminal reports the key as a
// sequence rather than as a byte. It is the value that sequence is queued as,
// so that the read loop and the key handler agree on what the key is called.
//
// The value is chosen to be one no sequence final can take. Every final byte
// this reader recognises sits in the range from 0x40 up, and the queued keys are
// held as bytes, so a value below that range cannot collide with an arrow, a
// CSI u report, or anything else the reader queues.
const keyShiftEnter = 0x00

// csiParamLo and csiParamHi bound the parameter bytes of a control sequence,
// which are the digits and the separators between them.
const (
	csiParamLo = 0x30
	csiParamHi = 0x3F
)

// csiFinalLo and csiFinalHi bound the final byte of a control sequence, which is
// the byte that ends it and is what identifies the key.
const (
	csiFinalLo = 0x40
	csiFinalHi = 0x7E
)

// takeSequence extracts a complete key sequence from the front of buf.
//
// It reports the final byte of the sequence, which is what identifies it. A
// sequence that has not arrived whole is left in place, since the rest of it may
// be in the next read.
//
// The length is read from the shape of the sequence rather than from a table of
// the forms this reader acts on. A terminal writes a modified key with a
// parameter in front of the final byte, as in ESC [ 1 ; 5 A for an alt-held
// up arrow, and a table of fixed lengths does not cover those. A form that is
// not recognised is consumed just the same, since leaving its bytes to be read
// as keys would put a bare escape and a bracket into the message.
// keyPageUp and keyPageDown are the queued values for a shifted arrow acting as
// a page rather than as a step.
//
// They sit below the range of sequence final bytes, as keyShiftEnter does, so
// that a queued key cannot collide with an arrow or with a report the terminal
// writes in the same range.
const (
	keyPageUp   = 0x01
	keyPageDown = 0x02
)

// Only the shift is read out of the modifier. The others are passed through as
// the plain arrow they are, since an alt up arrow is a key this reader has no
// use for and treating it as a page would be a guess. The value itself is
// shiftModifier, which the shifted enter reader already declares.

// takeSequenceWithModifier extracts a key sequence and the modifier it carries.
//
// It reports the final byte as before, and whether a shift was held. The
// modifier is read from the parameter rather than from a table of the forms a
// terminal writes, since a terminal reports a modified arrow as ESC [ 1 ; 2 A
// and a table of fixed lengths does not cover them.
//
// A sequence with no parameter, such as a plain arrow, is not shifted, whatever
// its final byte happens to be. That matters for the final bytes which are also
// digits, where reading the final as a parameter would turn every arrow into a
// shifted one.
func takeSequenceWithModifier(buf []byte) (final byte, shifted bool, rest []byte, ok bool) {
	if len(buf) < 2 || buf[0] != keyEscape || buf[1] != '[' {
		return 0, false, buf, false
	}
	if len(buf) >= 3 && (buf[2] == '<' || buf[2] == 'M') {
		return 0, false, buf, false
	}

	for i := 2; i < len(buf); i++ {
		c := buf[i]
		if c >= csiFinalLo && c <= csiFinalHi {
			return c, sequenceShifted(buf[2:i]), buf[i+1:], true
		}
		if c < 0x20 || c > csiParamHi {
			return 0, false, buf, false
		}
	}
	return 0, false, buf, false
}

// sequenceShifted reports whether the parameters of a sequence name a shift.
//
// The parameters of a modified key are the digit, a separator, and the modifier
// as one plus itself, so a shift is the last parameter being 2. A sequence with
// no separator, such as a plain arrow, carries no modifier and is not shifted.
func sequenceShifted(params []byte) bool {
	for i := len(params) - 2; i >= 0; i-- {
		if params[i] == ';' {
			return params[i+1] == '0'+shiftModifier
		}
	}
	return false
}

func takeSequence(buf []byte) (final byte, rest []byte, ok bool) {
	if len(buf) < 2 || buf[0] != keyEscape || buf[1] != '[' {
		return 0, buf, false
	}
	// The two introducers that belong to a mouse report are left alone. A
	// report arriving in pieces would otherwise be taken as an unknown key
	// sequence and the rest of it read as keys.
	if len(buf) >= 3 && (buf[2] == '<' || buf[2] == 'M') {
		return 0, buf, false
	}

	// A control sequence runs to its first byte in the final range. Everything
	// before it is a parameter, which is why the digit forms and the modified
	// forms are taken by the same walk rather than by two rules.
	for i := 2; i < len(buf); i++ {
		c := buf[i]
		if c >= csiFinalLo && c <= csiFinalHi {
			// Everything up to and including the final byte is consumed, so
			// the final byte is not left to be read as a key of its own.
			return c, buf[i+1:], true
		}
		if c < 0x20 || c > csiParamHi {
			// Neither a parameter nor a final byte, so these bytes are not a
			// control sequence and are left alone.
			return 0, buf, false
		}
	}
	// The sequence has not arrived whole, so it is left for the next read
	// rather than being cut in half.
	return 0, buf, false
}

// key acts on a special key.
//
// The up and down arrows walk the input history. The left and right arrows are
// consumed and reserved for input toggles, so that they do not end the line and
// so that the keys they will take are already spoken for.
//
// A shifted enter does nothing here. The read loop has already ended the line
// on the key, since ending a line needs the builder to append the text being
// composed and this function is given the builder to write into but not the
// ability to hold a line. A case here as well would break the line twice, which
// put a blank row between every line of a block.
//
// Anything else the reader queues is passed on, so that a caller may act on a
// key the editor has no use for.
func (le *LineEditor) key(final byte, out *strings.Builder) {
	switch final {
	case keyUp:
		le.applyRecall(true, out)
	case keyDown:
		le.applyRecall(false, out)
	case keyPageUp, keyPageDown:
		// A shifted arrow pages rather than recalling, and is handed on rather
		// than acted on here: what it does is the session decision, since only
		// the session knows how tall the pane is.
		if le.OnKey != nil {
			le.OnKey(final)
		}
	case keyShiftEnter:
		return
	case keyLeft, keyRight:
		return
	default:
		if le.OnKey != nil {
			le.OnKey(final)
		}
	}
}

// applyRecall replaces the composed line with a remembered one, when there is
// one to recall.
//
// The composition is not overwritten here, since the walk holds it aside and
// walking back out restores it. Only the remembered line is reported, so that
// the frame shows what the reader asked for.
//
// With nothing to recall the key has no action at all, and the line being
// composed is left exactly as it is. The builder belongs to the read loop
// rather than to the editor, so it is passed in.
func (le *LineEditor) applyRecall(back bool, out *strings.Builder) {
	line, ok := le.walkHistory(back)
	if !ok {
		// With nothing to recall the key has no action at all, and the line
		// being composed is left exactly as it is.
		return
	}

	out.Reset()
	out.WriteString(line)
	if le.OnChange != nil {
		le.OnChange(line)
	}
}

// walkHistory moves through the history and returns the line to compose.
//
// The up arrow walks back and the down arrow walks forward. At the ends it does
// nothing rather than wrapping, since wrapping makes it impossible to tell
// which end one is at. Walking past the newest restores the line that was being
// composed rather than losing it.
func (le *LineEditor) walkHistory(back bool) (string, bool) {
	if len(le.history) == 0 {
		// With no history there is nothing to recall, and the key has no
		// action at all.
		return "", false
	}

	if back {
		if le.historyAt == 0 {
			return "", false
		}
		if le.historyAt == len(le.history) {
			// The walk is leaving the line being composed, so it is kept
			// aside for the walk back out. Taking the composition from here
			// rather than from the recalled line is what makes walking
			// forward past the newest resume it.
			le.recalled = le.composing
		}
		le.historyAt--
		return le.history[le.historyAt], true
	}

	if le.historyAt >= len(le.history) {
		return "", false
	}
	le.historyAt++
	if le.historyAt == len(le.history) {
		// Past the newest, so the line that was being composed is returned
		// and the composition resumes.
		line := le.recalled
		le.recalled = ""
		le.composing = line
		return line, true
	}
	return le.history[le.historyAt], true
}

// remember records a submitted line.
//
// The history is bounded, since a long session would otherwise grow it without
// limit for no benefit: the lines a user reaches for are recent ones.
func (le *LineEditor) remember(line string) {
	le.history = append(le.history, line)
	if len(le.history) > maxHistory {
		le.history = le.history[len(le.history)-maxHistory:]
	}
	le.historyAt = len(le.history)
	le.recalled = ""
}

// maxHistory is how many submitted lines are remembered.
const maxHistory = 100

// ReadByte returns the next byte of input without treating it as a line.
//
// The model filter is typed into rather than submitted, so it reads keys
// directly instead of going through the line editor. A key sequence is consumed
// and discarded, since the filter takes characters only.
//
// The discard happens before the error is reported, as the line editor acts on
// its own sequences before it handles the end of the input. Leaving one queued
// would hand it to the line editor afterwards, where it would act on a key
// pressed while the filter was open.
//
// A pasted block that lands here is discarded as well. The filter takes
// characters only and its enter chooses from a listing rather than sending a
// message, so a block left in the editor could never be delivered by the
// filter, and would instead be prepended to whatever was composed next.
func (le *LineEditor) ReadByte() (byte, error) {
	b, err := le.readKey()
	if len(le.pendingKeys) > 0 {
		le.pendingKeys = le.pendingKeys[:0]
	}
	le.pasted = nil
	return b, err
}
