package tui

import (
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

// readChunk is how many bytes are read from the terminal at once.
//
// A whole mouse report fits in one read, and a terminal writes it in one piece,
// so reading in blocks is what lets a report be recognised before any part of
// it is mistaken for a key. The block is larger than the longest report so
// that a report typed alongside a keystroke is still read whole.
const readChunk = 64

// LineEditor reads a single line of text.
type LineEditor struct {
	r io.Reader
	// OnChange is called with the line as it stands after each keystroke. It
	// is what makes the composed line visible, since the terminal is in raw
	// mode with echo disabled and so does not draw it. A nil callback draws
	// nothing, which is the correct behaviour for a non-interactive reader.
	OnChange func(string)
	// OnMouse is called with the wheel direction of each mouse report read
	// alongside the keys. It is what lets the view be scrolled without
	// ending the line, since a report shares the input stream with the keys
	// and has to be consumed rather than read as one.
	//
	// The callback runs on the reading goroutine and must not block. A nil
	// callback discards the report, which is what a reader with no view to
	// scroll should do.
	OnMouse func(direction int)
	// buf holds input read from the terminal but not yet acted on. The bytes
	// behind a mouse report are kept here, so that a report arriving in the
	// same read as a keystroke does not swallow the keystroke.
	buf []byte
	// chunk is the buffer a block is read into. It is held separately from
	// buf, since buf is consumed a byte at a time and a read needs the whole
	// block every time.
	chunk []byte
}

// NewLineEditor reads lines from r.
func NewLineEditor(r io.Reader) *LineEditor {
	return &LineEditor{r: r}
}

// readKey returns the next key, consuming any mouse report that comes first.
//
// Input is read in blocks and a report is taken from the block before the block
// is treated as keys. A terminal writes a whole report in one piece and the
// read returns all of it, so the report is recognised here and passed to
// OnMouse rather than being read as a key. That is what keeps a wheel notch
// from ending the session, since every report opens with a bare escape and a
// lone escape is an interrupt. Reading a byte at a time would see that opening
// escape on its own, which is exactly the failure this avoids.
func (le *LineEditor) readKey() (byte, error) {
	for {
		if len(le.buf) == 0 {
			if err := le.fill(); err != nil {
				return 0, err
			}
		}
		for len(le.buf) > 0 {
			if seq, rest, ok := takeMouseSequence(le.buf); ok {
				le.buf = rest
				le.mouse(seq)
				continue
			}
			// Bytes at the front that could still become a report are held
			// rather than returned as keys. A reader is free to return a
			// short block, and a report split across two blocks would
			// otherwise be handed over as the escape that opens one, which
			// ends the line. A lone escape does not look like a report
			// prefix, so it is still returned at once.
			if mousePrefix(le.buf) {
				break
			}
			b := le.buf[0]
			le.buf = le.buf[1:]
			return b, nil
		}
		// The block held no key to return, which is what a block of mouse
		// reports looks like. The next block is read rather than returning
		// nothing, since an empty key would end the line.
		if err := le.fill(); err != nil {
			return 0, err
		}
		if len(le.buf) == 0 {
			return 0, io.EOF
		}
	}
}

// fill reads one block of input and appends it to what is already held.
//
// The bytes are appended rather than replacing the buffer, since a report split
// across two reads leaves a prefix that the next block completes. Dropping it
// would leave the tail of the report to be read as keys, and the escape at its
// head would end the line.
func (le *LineEditor) fill() error {
	if le.chunk == nil {
		le.chunk = make([]byte, readChunk)
	}
	n, err := le.r.Read(le.chunk)
	le.buf = append(le.buf, le.chunk[:n]...)
	return err
}

// mouse reports a wheel direction to the caller.
func (le *LineEditor) mouse(seq []byte) {
	if le.OnMouse == nil {
		return
	}
	if d := parseMouse(seq); d != mouseNone {
		le.OnMouse(d)
	}
}

// notify reports the current line.
func (le *LineEditor) notify(out *strings.Builder) {
	if le.OnChange != nil {
		le.OnChange(out.String())
	}
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
		b, err := le.readKey()
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
			le.notify(&out)
		case keyCtrlW:
			// The buffer is not reset here: trimLastWord reads it in order to
			// keep everything before the final word.
			pending = pending[:0]
			trimLastWord(&out)
			le.notify(&out)
		case keyBackspace, keyDelete:
			if len(pending) == 0 {
				removeLastRune(&out)
			} else {
				pending = pending[:0]
			}
			le.notify(&out)
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
				le.notify(&out)
				continue
			}
			pending = append(pending, b)
			if !utf8Complete(pending) {
				continue
			}
			out.Write(pending)
			pending = pending[:0]
			le.notify(&out)
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
