package tui

import (
	"io"
	"os"
	"strings"
	"time"
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

// prefixTimeout is how long a possible report prefix is waited for before it is
// assumed not to be one.
//
// The figure is short enough that a lone escape feels immediate and long enough
// that a report split across a slow link arrives whole. A terminal writes a
// report in one piece, so the delay only applies to a prefix that was never
// going to complete.
const prefixTimeout = 60 * time.Millisecond

// errPrefixUnfinished reports that a held prefix was not completed in the time
// the wait allowed.
//
// The caller hands the bytes back as keys rather than treating this as the end of
// the input, since the prefix was never shown not to be a report.
var errPrefixUnfinished = errorString("the held prefix never arrived")

// LineEditor reads a single line of text.
type LineEditor struct {
	r io.Reader
	// src is the underlying file when there is one. A read deadline can only be
	// set on a file, and the deadline is what keeps a held report prefix
	// from waiting on input that never arrives.
	src *os.File
	// held is true while the buffer holds a possible report prefix that is
	// waiting for the rest of itself.
	held bool
	// OnPaste is called with the lines of a paste as it lands, so that the
	// interface can show them. Without it a paste would appear to do nothing
	// until it was submitted, which for a large one reads as a frozen
	// interface.
	OnPaste func(lines []string)
	// OnChange is called with the line as it stands after each keystroke. It
	// is what makes the composed line visible, since the terminal is in raw
	// mode with echo disabled and so does not draw it. A nil callback draws
	// nothing, which is the correct behaviour for a non-interactive reader.
	OnChange func(string)
	// OnTab is called when the user presses Tab, with the line as it stands.
	// It returns the line to compose in its place, or an empty string to
	// leave the line as it is. The editor holds no pane, so whatever the
	// callback wishes to say about the candidates it says itself rather than
	// the editor reporting them into a pane it does not own.
	//
	// A nil callback inserts a literal tab, which is the only thing a reader
	// without completion can expect the key to do.
	OnTab func(line string) string
	// OnMode is called when the multi-line mode is turned on or off, so that
	// the interface can say so on the hint row. The row names keys that act,
	// and Enter means something different inside the mode, so it has to be
	// told rather than left naming the key the reader has to stop using.
	//
	// A nil callback draws nothing, which is the correct behaviour for a
	// non-interactive reader.
	OnMode func(on bool)
	// pendingKeys holds the final bytes of the special key sequences read since
	// the last keypress, oldest first. They are acted on by the read loop, which
	// holds the line being composed, rather than by the reader, which does not.
	//
	// A queue rather than a single slot, since one read block can hold several
	// sequences and a second would overwrite the first.
	pendingKeys []byte
	// composing is the line being composed, kept so that walking into the
	// history can restore it when the user walks back out.
	composing string
	// history holds the submitted lines, newest last, so that the up and down
	// arrows walk through what has already been sent. It is not written
	// anywhere, so it lasts only as long as the session.
	history []string
	// historyAt is where in the history the cursor is, len(history) meaning
	// the line being composed rather than a remembered one.
	historyAt int
	// recalled is the line being composed when the user first walked back into
	// the history, so that walking forward past the newest restores it rather
	// than losing it. Zero once the walk has finished, since a later walk
	// captures a fresh composition.
	recalled string
	// pasted holds the lines of a paste that has landed, kept out of the
	// typed line so that a multi-line paste is one input rather than one
	// message per line.
	pasted []string
	// multiline reports that the reader has asked for a multi-line message, in
	// which Enter ends a line rather than sending and Ctrl-J sends the block
	// rather than opening the mode.
	//
	// The lines are held in the same list as a landed paste, since a block
	// typed over several lines is the same thing as a block pasted in one go
	// as far as the message that goes out and the rows the interface shows.
	// Reusing it is what keeps the frame from having to learn a second kind of
	// block above the prompt.
	multiline bool
	// OnAsk is called with each key before it is acted on, and reports whether
	// it took the key.
	//
	// It exists because the editor owns the terminal while a line is being
	// composed. An approval question is asked from another goroutine while the
	// editor is blocked reading, so the loop above never regains control and a
	// key pressed in answer to the question is taken as part of the message
	// being composed. A hook is what lets the editor hand the key over rather
	// than read it, and it is a hook rather than a poll because the editor only
	// regains control when a key arrives, which is too late to ask who wants it.
	OnAsk func(b byte) bool

	// OnKey is called with the final byte of each special key sequence, such as
	// an arrow. It is nil when the caller does not act on them.
	OnKey func(final byte)
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
	le := &LineEditor{r: r}
	if f, ok := r.(*os.File); ok {
		le.src = f
	}
	return le
}

// multilineMode returns whether the multi-line mode is on.
func (le *LineEditor) multilineMode() bool { return le.multiline }

// setMultiline turns the multi-line mode on or off and reports it.
//
// The callback is called on the change rather than on every read, so that the
// interface draws the notice once rather than on each keystroke.
func (le *LineEditor) setMultiline(on bool) {
	if le.multiline == on {
		return
	}
	le.multiline = on
	if le.OnMode != nil {
		le.OnMode(on)
	}
}

// holdable reports whether a report prefix may be held across a read.
//
// Every reader may be held for. A reader that is a file is given a deadline so
// that a prefix at the end of a stream does not wait forever, and a reader that
// is not a file reports the end of its input rather than blocking, so its read
// always returns.
func (le *LineEditor) holdable() bool { return true }

// fillHeld reads one block while a report prefix is held.
//
// The read waits for the rest of a report rather than being made on the spot,
// and the wait is bounded, so that a prefix which is never completed is handed
// back as keys instead of sitting in the buffer for ever. A lone escape is a
// key the reader pressed, so it has to come back rather than be waited on.
//
// The bound is a read that cannot block rather than a read deadline on the
// file. Go hands the standard streams to a program through os.NewFile, which
// leaves the terminal blocking and outside the runtime poller, and a deadline
// cannot be set on a descriptor the runtime is not watching: SetReadDeadline
// fails on every terminal, and the failure was taken for the end of the wait. A
// report arriving in pieces was therefore handed back as keys the moment it was
// split, and its opening escape ended the line. One wheel notch out of every six
// was split, since a read of sixty-four bytes does not divide the twelve-byte
// report, and scrolling fast is what filled the queue that splits one.
//
// The wait is per read rather than for the whole hold, so that a report arriving
// over several reads is assembled rather than cut short after the first gap.
func (le *LineEditor) fillHeld() error {
	if !le.held {
		le.held = true
	}
	if le.src == nil {
		// Without a file there is no descriptor to read, and the read reports
		// the end of a stream rather than blocking, so it returns.
		return le.fill()
	}
	n, err := readWithin(int(le.src.Fd()), prefixTimeout, le.chunkFor())
	if err != nil {
		return err
	}
	le.buf = append(le.buf, le.chunk[:n]...)
	return nil
}

// chunkFor returns the read buffer, allocating it on first use.
func (le *LineEditor) chunkFor() []byte {
	if le.chunk == nil {
		le.chunk = make([]byte, readChunk)
	}
	return le.chunk
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
		// A key already queued from a sequence is returned before anything
		// else is read.
		//
		// The queue is drained in the line loop, after this returns, so
		// returning a byte here while a sequence is queued would leave the
		// sequence waiting for the next byte to arrive. A reader pressing a
		// shifted arrow and getting nothing until they also pressed enter
		// had exactly that: the arrow was read, queued, and left there.
		if len(le.pendingKeys) > 0 {
			key := le.pendingKeys[0]
			le.pendingKeys = le.pendingKeys[1:]
			return key, nil
		}
		if len(le.buf) == 0 {
			if err := le.fill(); err != nil {
				return 0, err
			}
		}
		for len(le.buf) > 0 {
			// A pasted block is taken whole. Treating its bytes as keys would
			// submit on the first newline inside it and send half a paste as
			// a message.
			if text, rest, ok := takePaste(le.buf); ok {
				le.buf = rest
				le.pasted = append(le.pasted, normalisePaste(text)...)
				if le.OnPaste != nil {
					le.OnPaste(le.pasted)
				}
				continue
			}
			// Bytes at the front that could still become a report or a pasted
			// block are held rather than returned as keys. A reader is free
			// to return a short block, and over a slow link a report arrives
			// one byte at a time. Returning the leading escape as a key would
			// end the line and leave the session, which is a crash rather
			// than a misread.
			//
			// The hold is taken before the sequence is, since the opening
			// marker of a paste has the shape of a key sequence itself. Taken
			// first it would leave the body of the paste to be read as typing,
			// which submits the line at its first newline.
			//
			// A prefix too short to tell from anything else is still held,
			// since the rest of it may be in the next read. A lone escape
			// is held on the same terms and is handed back as a key once the
			// hold times out, which is what lets it interrupt.
			if le.holdable() && (mousePrefix(le.buf) || pastePrefix(le.buf)) {
				if err := le.fillHeld(); err != nil {
					// Nothing more arrived before the deadline, so the
					// prefix was never a report. The bytes are handed back
					// as keys below, which makes a lone escape interrupt
					// and an arrow reach the key handler, rather than either
					// being reported as the end of the input.
					break
				}
				continue
			}
			if seq, rest, ok := takeMouseSequence(le.buf); ok {
				le.buf = rest
				le.mouse(seq)
				continue
			}
			// A shifted enter is queued rather than acted on here, since the
			// line being composed is held by the read loop and not by this
			// function. It is taken before the ordinary sequence so that the
			// report is not read as an unknown key and its bytes put into the
			// message.
			if rest, ok := takeShiftedEnter(le.buf); ok {
				le.buf = rest
				le.pendingKeys = append(le.pendingKeys, keyShiftEnter)
				continue
			}
			// A key sequence is consumed whole. Left to be read as keys, its
			// opening escape would end the line and leave the session, which
			// is what pressing an arrow used to do.
			if final, shifted, rest, ok := takeSequenceWithModifier(le.buf); ok {
				le.buf = rest
				// A shifted arrow pages rather than stepping, so it is queued
				// as its own key. The plain arrow is queued as the arrow,
				// since that is what every terminal sends for it and a
				// sequence carrying no modifier is not a shifted one.
				if shifted {
					switch final {
					case keyUp:
						final = keyPageUp
					case keyDown:
						final = keyPageDown
					}
				}
				le.pendingKeys = append(le.pendingKeys, final)
				continue
			}
			b := le.buf[0]
			le.buf = le.buf[1:]
			return b, nil
		}
		// The buffer holds a prefix that was not completed. The bytes are
		// handed back as keys, which is what a lone escape and an arrow are.
		if mousePrefix(le.buf) || pastePrefix(le.buf) {
			b := le.buf[0]
			le.buf = le.buf[1:]
			le.held = false
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
	n, err := le.r.Read(le.chunkFor())
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
	le.composing = out.String()
	if le.OnChange != nil {
		le.OnChange(le.composing)
	}
}

// reportBlock tells the interface what the block above the prompt holds.
//
// It is called whenever the block changes, so that the rows the reader can see
// are the lines that are actually there. The block is reported whether it came
// from a paste or from the reader typing into the multi-line mode, since the
// interface draws one above the other.
func (le *LineEditor) reportBlock() {
	if le.OnPaste == nil || len(le.pasted) == 0 {
		return
	}
	le.OnPaste(le.pasted)
}

// ReadLine reads one line.
//
// Raw mode means no line discipline, so keys are assembled here. Backspace and
// Ctrl-U clear text, Ctrl-W clears the last word, and Ctrl-C abandons the line.
// A multi-byte rune arriving one byte at a time is held until the sequence is
// complete, so a character is not cut in half by a keystroke boundary.
//
// Enter submits the message. Ctrl-J opens the multi-line mode instead, in which
// Enter ends a line, Ctrl-J sends the block, and Escape abandons it. The mode
// exists because a newline typed into a line is held but not shown: the prompt
// draws one row, so a message being composed over several lines would be
// invisible to the reader writing it. The mode holds the lines where the
// interface already draws a landed paste, so a block typed is shown the way a
// block pasted is.
//
// Shift with Enter ends a line in either state, since it means a break and a
// break is what it means on an idle prompt too.
func (le *LineEditor) ReadLine() (string, error) {
	var out strings.Builder
	var pending []byte

	// A block that has landed belongs to the line it landed in. Every path out
	// of this loop other than a submit abandons it, so that text the reader
	// discarded with a control character is not prepended to whatever they type
	// next. The submit path takes the block out of the editor itself and sets
	// the flag, so the two cannot disagree.
	submitted := false
	defer func() {
		if !submitted && !le.multiline {
			le.pasted = nil
		}
	}()

	for {
		b, err := le.readKey()

		// Every special key read on the way to this byte is acted on before
		// the byte is handled, so that an arrow sharing a read block with a
		// submit is seen before the submit is processed. All are drained,
		// since one read block may hold several sequences.
		//
		// The drain comes before the error check, because a sequence at the
		// end of a block is followed by the end of the input rather than by a
		// further byte, and acting on it after the error was handled would
		// leave the key with no effect at all.
		for len(le.pendingKeys) > 0 {
			key := le.pendingKeys[0]
			le.pendingKeys = le.pendingKeys[1:]
			// A shifted enter breaks the line where it was pressed, so a rune
			// that has not arrived whole is abandoned rather than committed
			// after the break. The decision is taken here rather than in the
			// key handler, since the held bytes belong to this loop.
			if key == keyShiftEnter {
				pending = pending[:0]
				le.breakLine(&out)
			}
			le.key(key, &out)
		}

		if err != nil {
			if len(pending) == 0 && out.Len() == 0 {
				return "", ErrEndOfInput
			}
			return out.String(), err
		}

		// A key is offered to whatever is waiting for one before it is acted
		// on here. An approval question is open while the reader is composing
		// nothing in particular, and its answer has to reach the question
		// rather than the line. The key is not put back: a question answers
		// one key and is closed by it, and a key handed to it has been used.
		if le.OnAsk != nil && le.OnAsk(b) {
			continue
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
		case keyEnter:
			if len(pending) > 0 {
				out.Write(pending)
				pending = pending[:0]
			}
			// Inside the multi-line mode Enter ends a line rather than sending
			// the message. The line is held where a landed paste is held, so
			// that it is drawn above the prompt, and the line being composed
			// starts again empty.
			if le.multiline {
				le.breakLine(&out)
				continue
			}
			// A block that has landed is returned as part of the message, so
			// that a multi-line paste reaches the model whole. It is joined
			// with newlines rather than sent as several messages, since the
			// user pasted one thing.
			// A submitted line joins the history, and the composition is
			// cleared, since that line is no longer being composed and the
			// next recall would otherwise save it joined to the next.
			if line := out.String(); line != "" {
				le.remember(line)
			}
			le.composing = ""

			if len(le.pasted) > 0 {
				lines := le.pasted
				le.pasted = nil
				submitted = true
				if typed := out.String(); typed != "" {
					lines = append(lines, typed)
				}
				return strings.Join(lines, "\n"), nil
			}
			submitted = true
			return out.String(), nil
		case keyNewline:
			// Ctrl-J opens the multi-line mode, and sends the block where the
			// mode is on. A terminal in raw mode sends it as the same byte as a
			// newline, so a line carrying one arrived as text as well, and the
			// two are not told apart.
			//
			// A rune that has not arrived whole is abandoned rather than
			// committed, so that a break typed part way through one does not
			// leave a broken sequence behind to be written later.
			pending = pending[:0]
			if le.multiline {
				le.breakLine(&out)
				le.setMultiline(false)
				le.settleSubmitted(&out)
				return le.sendBlock(), nil
			}
			// The key ends the line being composed as well as opening the
			// mode. A block is a block however it was started, and a break
			// held only once the mode was on would join the first line to
			// the second.
			le.breakLine(&out)
			le.setMultiline(true)
		case keyTab:
			// A control key abandons a rune that has not arrived whole, so
			// that a Tab pressed part way through one does not leave a broken
			// sequence behind to be committed later.
			pending = pending[:0]
			if le.OnTab == nil {
				// Without a completer the key is a tab, which is what a
				// reader indenting a message may want.
				out.WriteByte(keyTab)
				le.notify(&out)
				continue
			}
			// The line is replaced only when a single candidate was found.
			// An ambiguous prefix and one that matches nothing both return
			// nothing, so Tab never guesses at a word. The line is reported
			// either way, so that a Tab which only wrote a report is
			// repainted by the same path that repaints any other keystroke.
			if line := le.OnTab(out.String()); line != "" {
				out.Reset()
				out.WriteString(line)
			}
			le.notify(&out)
		case keyEscape:
			// A lone escape is a key rather than the start of a sequence,
			// since no escape sequence is read as input here.
			//
			// It interrupts whether or not there is a line in hand. It was
			// answered only on an empty line, which left the key doing
			// nothing at all while a message was being composed, and a reader
			// pressing it to abandon what they had typed had no way to say
			// so. What the session does with the interrupt is decided there,
			// since escape abandons a line on an idle prompt and stops a
			// model with it while one is working.
			//
			// Inside the multi-line mode it abandons the block rather than the
			// line, since a reader pressing it there means to discard what they
			// have written rather than to leave the session.
			if le.multiline {
				le.setMultiline(false)
				le.pasted = nil
			}
			return "", ErrInterrupt
		default:
			// Another control key, ignored rather than inserted. Tab is not
			// among them, since it has a case of its own above.
			if b < 0x20 {
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

// breakLine ends the line being composed and holds it, so that the next line
// starts empty and the one before it is drawn above the prompt.
//
// The line is held in the same list as a landed paste, which is what keeps the
// interface from having to learn a second kind of block above the prompt. An
// empty line is not held, since a reader pressing Enter twice has typed
// nothing rather than a blank line, and a block carrying an empty row reads as
// one carrying a gap in it.
func (le *LineEditor) breakLine(out *strings.Builder) {
	if out.Len() > 0 {
		le.pasted = append(le.pasted, out.String())
	}
	out.Reset()
	le.composing = ""
	le.reportBlock()
}

// settleSubmitted marks the block as being sent, so that the deferred clearing
// of an abandoned block does not take it on the way out.
//
// The submit path takes the block out of the editor itself and sets the flag, so
// the two cannot disagree. Without the flag here, the deferred clearing above
// would empty a block that had already been handed to the caller.
func (le *LineEditor) settleSubmitted(out *strings.Builder) {
	if out.Len() > 0 {
		le.pasted = append(le.pasted, out.String())
		out.Reset()
	}
	le.composing = ""
}

// sendBlock returns the block as one message and clears it.
//
// The block is joined with newlines, since a message typed over several lines
// is one message and sending it as several would ask them as several questions.
func (le *LineEditor) sendBlock() string {
	lines := le.pasted
	le.pasted = nil
	if len(lines) == 0 {
		return ""
	}
	line := strings.Join(lines, "\n")
	le.remember(line)
	le.composing = ""
	return line
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
