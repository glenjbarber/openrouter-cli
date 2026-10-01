package tui

import "strings"

// Special key sequences a terminal sends for keys that are not bytes.
//
// The reader is in raw mode with echo disabled, so every key arrives as bytes
// and a sequence must be consumed whole. A sequence handled one byte at a time
// leaves its opening escape to be read as a key, and a lone escape ends the
// line: pressing an arrow would close the session.

// sequenceKinds are the final bytes of the sequences the reader understands.
//
// The sequences are the ones in common use by terminal emulators. A sequence
// ending in any other byte is not one this reader acts on, so it is consumed
// and discarded rather than being read as keys.
const (
	keyUp    = 'A'
	keyDown  = 'B'
	keyRight = 'C'
	keyLeft  = 'D'
	keyHome  = 'H'
	keyEnd   = 'F'
	keyDel   = '~'
)

// sequenceLengths maps the byte that ends a sequence to how long that sequence
// is, for the forms whose length is fixed.
//
// The cursor and home keys are three bytes: an escape, a bracket, and a final
// byte. Delete may be written either as the three-byte bracket form or as a
// four-byte form with a number, which is why it carries its own length.
var sequenceLengths = map[byte]int{
	keyUp:    3,
	keyDown:  3,
	keyRight: 3,
	keyLeft:  3,
	keyHome:  3,
	keyEnd:   3,
	keyDel:   3,
}

// takeSequence extracts a complete key sequence from the front of buf.
//
// It reports the final byte of the sequence, which is what identifies it. A
// sequence that has not arrived whole is left in place, since the rest of it may
// be in the next read.
func takeSequence(buf []byte) (final byte, rest []byte, ok bool) {
	if len(buf) < 2 || buf[0] != keyEscape || buf[1] != '[' {
		return 0, buf, false
	}
	if len(buf) < 3 {
		return 0, buf, false
	}

	final = buf[2]

	// The four-byte forms carry a number before the final byte, such as the
	// delete key. They are recognised by a digit or a semicolon after the
	// bracket.
	// The two introducers that belong to a mouse report are left alone. A
	// report arriving in pieces would otherwise be taken as an unknown key
	// sequence and the rest of it read as keys.
	if buf[2] == '<' || buf[2] == 'M' {
		return 0, buf, false
	}

	// A digit where a key letter would be marks one of the longer forms, whose
	// number precedes the final byte.
	if buf[2] >= '0' && buf[2] <= '9' {
		for i := 2; i < len(buf); i++ {
			if buf[i] == '~' {
				// Everything up to and including the tilde is consumed, so
				// the tilde is not left to be read as a key of its own.
				return buf[2], buf[i+1:], true
			}
			if buf[i] < '0' || buf[i] > '9' {
				return 0, buf, false
			}
		}
		// The number has not arrived whole, so the sequence is left for the
		// next read rather than being cut in half.
		return 0, buf, false
	}

	n, known := sequenceLengths[final]
	if !known {
		// An unknown final byte is still consumed. It is a sequence this
		// reader does not act on, and leaving its bytes to be read as keys
		// would put a bare escape and a bracket into the line.
		return final, buf[3:], true
	}
	if len(buf) < n {
		return 0, buf, false
	}
	return final, buf[n:], true
}

// key reports a special key to the caller.
//
// The line editor keeps no history and does not move the cursor within the
// line, so there is nothing for most of these to do. They are consumed rather
// than passed on for that reason: a sequence reaching the line editor as bytes
// would put an escape and a bracket into the message being composed.
//
// Left and right are recorded so that the composed line can be moved through// key acts on a special key.
//
// The up and down arrows walk the input history. The left and right arrows are
// consumed and reserved for input toggles, so that they do not end the line and
// so that the keys they will take are already spoken for.
func (le *LineEditor) key(final byte, out *strings.Builder) {
	switch final {
	case keyUp:
		le.applyRecall(true, out)
	case keyDown:
		le.applyRecall(false, out)
	case keyLeft, keyRight:
		return
	default:
		if le.OnKey != nil {
			le.OnKey(final)
		}
	}
}

// recall replaces the composed line with a remembered one.
//
// The builder is the line being composed, which belongs to the read loop
// maxHistory is how many submitted lines are remembered.

// recall walks the history and returns the line to compose.
//
// The up arrow walks back and the down arrow walks forward. At the ends it does
// nothing rather than wrapping, since wrapping makes it impossible to tell
// which end one is at. Walking past the newest restores the line that was being
// applyRecall replaces the composed line with a remembered one, when there is
// one to recall.
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
	// The composition follows the recall, so that a later recall knows where
	// in the walk the user is rather than treating every key as a new start.
	le.composing = line
	if le.OnChange != nil {
		le.OnChange(line)
	}
}

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
// and ignored, since the filter takes characters only.
func (le *LineEditor) ReadByte() (byte, error) {
	for {
		b, err := le.readKey()
		if err != nil {
			return 0, err
		}
		if len(le.pendingKeys) > 0 {
			le.pendingKeys = le.pendingKeys[:0]
		}
		return b, nil
	}
}
