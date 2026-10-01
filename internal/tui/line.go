package tui

import (
	"bufio"
	"io"
	"strings"
	"unicode/utf8"
)

// Control keys the line editor acts on. The reader is in raw mode, so these
// arrive as ordinary bytes rather than as signals or as a completed line.
const (
	keyCtrlC     = 0x03
	keyCtrlD     = 0x04
	keyCtrlU     = 0x15
	keyCtrlW     = 0x17
	keyBackspace = 0x08
	keyDelete    = 0x7f
	keyEscape    = 0x1b
	keyEnter     = '\r'
	keyNewline   = '\n'
	keyTab       = '\t'
)

// ErrInterrupt reports that the user asked to abandon the current input.
//
// It is returned rather than handled by a panic so that the caller decides
// whether abandoning one line abandons the session.
var ErrInterrupt = errorString("interrupted")

// ErrEndOfInput reports that the input was closed.
var ErrEndOfInput = errorString("end of input")

// errorString is an error with a fixed message. The package avoids fmt.Errorf
// for these so that the two sentinels compare equal by identity.
type errorString string

func (e errorString) Error() string { return string(e) }

// LineEditor reads a single line of text.
type LineEditor struct {
	r *bufio.Reader
}

// NewLineEditor reads lines from r.
func NewLineEditor(r io.Reader) *LineEditor {
	return &LineEditor{r: bufio.NewReader(r)}
}

// ReadLine reads one line.
//
// Raw mode means no line discipline, so keys are assembled here. Backspace and
// Ctrl-U clear text, Ctrl-W clears the last word, and Ctrl-C abandons the line.
// A multi-byte rune arriving one byte at a time is held until the sequence is
// complete, so a character is not cut in half by a keystroke boundary.
func (le *LineEditor) ReadLine() (string, error) {
	var out strings.Builder
	var pending []byte

	for {
		b, err := le.r.ReadByte()
		if err != nil {
			if len(pending) == 0 && out.Len() == 0 {
				return "", ErrEndOfInput
			}
			return out.String(), err
		}

		switch b {
		case keyCtrlC:
			return "", ErrInterrupt
		case keyCtrlD:
			if out.Len() == 0 && len(pending) == 0 {
				return "", ErrEndOfInput
			}
		case keyCtrlU:
			out.Reset()
			pending = pending[:0]
		case keyCtrlW:
			// The buffer is not reset here: trimLastWord reads it in order to
			// keep everything before the final word.
			pending = pending[:0]
			trimLastWord(&out)
		case keyBackspace, keyDelete:
			if len(pending) == 0 {
				removeLastRune(&out)
			} else {
				pending = pending[:0]
			}
		case keyEnter, keyNewline:
			if len(pending) > 0 {
				out.Write(pending)
				pending = pending[:0]
			}
			return out.String(), nil
		case keyEscape:
			// A lone escape is a key rather than the start of a sequence,
			// since no escape sequence is read as input here.
			if out.Len() == 0 {
				return "", ErrInterrupt
			}
		default:
			if b < 0x20 && b != keyTab {
				// Another control key, ignored rather than inserted.
				continue
			}
			if b < utf8Start {
				out.WriteByte(b)
				continue
			}
			pending = append(pending, b)
			if !utf8Complete(pending) {
				continue
			}
			out.Write(pending)
			pending = pending[:0]
		}
	}
}

// utf8Start is the first byte of a multi-byte UTF-8 sequence. A byte below it
// is a complete ASCII character and needs no buffering.
const utf8Start = 0x80

// utf8Complete reports whether the held bytes form a whole rune.
//
// A truncated sequence is held rather than written, so that a rune arriving
// across two reads is not committed as a replacement character.
func utf8Complete(b []byte) bool {
	switch {
	case len(b) == 0:
		return false
	case b[0] < 0xC0:
		return true
	case b[0] < 0xE0:
		return len(b) >= 2
	case b[0] < 0xF0:
		return len(b) >= 3
	default:
		return len(b) >= 4
	}
}

// trimLastWord removes the trailing word and the space before it, which is
// what Ctrl-W is expected to do in a shell.
func trimLastWord(out *strings.Builder) {
	s := strings.TrimRight(out.String(), " \t")
	// A last index of -1 means there is no space at all, so the whole line is
	// one word and nothing is kept. Falling through with the string unchanged
	// would make the key do nothing in exactly the case where a shell clears
	// the line.
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		s = s[:i]
	} else {
		s = ""
	}
	out.Reset()
	out.WriteString(s)
}

// removeLastRune removes the final rune, so that a multibyte character is
// removed whole rather than leaving half of it behind.
func removeLastRune(out *strings.Builder) {
	s := out.String()
	if s == "" {
		return
	}
	size := lastRuneWidth(s)
	out.Reset()
	out.WriteString(s[:len(s)-size])
}

// lastRuneWidth reports the width in bytes of the final rune of s.
//
// The whole string is passed rather than only its last byte, because decoding a
// single trailing byte of a multibyte rune reports a width of one and would
// leave the remaining bytes behind as a broken sequence.
func lastRuneWidth(s string) int {
	_, size := utf8.DecodeLastRuneInString(s)
	if size < 1 {
		return 1
	}
	return size
}
