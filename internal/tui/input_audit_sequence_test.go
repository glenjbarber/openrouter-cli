package tui

import (
	"strings"
	"testing"
)

// A sequence with a modifier, which a terminal sends for a shifted or
// alt-held arrow, is consumed whole rather than leaving its opening escape to
// be read as a key.
func TestAuditModifiedSequenceIsConsumedWhole(t *testing.T) {
	for _, seq := range []string{"\x1b[1;5A", "\x1b[1;2B", "\x1b[3;5~"} {
		le := NewLineEditor(strings.NewReader(seq + "text\r"))
		got, err := le.ReadLine()
		if err != nil {
			t.Errorf("%q: err = %v, want the sequence consumed", seq, err)
			continue
		}
		if got != "text" {
			t.Errorf("%q: got %q, want %q", seq, got, "text")
		}
	}
}

// Keys read in one block are queued rather than held in a single slot, since
// a block can hold several and a second would overwrite the first.
func TestAuditSeveralSequencesInOneBlockAreAllActedOn(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\rsecond\r\x1b[A\x1b[A\x1b[A\r"))
	got := readLines(t, le, 3)
	if got[2] != "first" {
		t.Errorf("got %q, want the oldest line, so a queued key overwrote another", got[2])
	}
}

// A carriage return submits the line, since on an idle prompt it is the key
// that sends and Ctrl-J opens the multi-line mode rather than ending a line.
func TestAuditCarriageReturnSubmits(t *testing.T) {
	le := NewLineEditor(strings.NewReader("cr\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if got != "cr" {
		t.Errorf("got %q, want the line submitted", got)
	}
}

// Ctrl-J opens the multi-line mode rather than sending, since a break held in
// the single prompt row is invisible to the reader writing it. Inside the mode
// Enter ends a line rather than sending, and a second Ctrl-J sends the block.
//
// The mode is reported once on the change rather than on every read, so the
// interface draws the notice without drawing it again per keystroke.
func TestAuditCtrlJOpensTheMultiLineMode(t *testing.T) {
	var states []bool
	// one, then Ctrl-J opens the mode, two, then Enter ends a line, three,
	// then Ctrl-J sends the block.
	le := NewLineEditor(strings.NewReader("one\ntwo\rthree\n"))
	le.OnMode = func(on bool) { states = append(states, on) }

	if le.multilineMode() {
		t.Fatal("the mode is on before a key was pressed")
	}
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(states) != 2 || !states[0] || states[1] {
		t.Errorf("the mode was reported as %v, want it reported twice, on then off", states)
	}
	if le.multilineMode() {
		t.Error("the mode is still on after the block was sent")
	}
	if got != "one\ntwo\nthree" {
		t.Errorf("got %q, want the three lines joined into one message", got)
	}
}

// Enter ends a line inside the mode rather than sending, so the key that sends
// on an idle prompt does not send a block the reader is still typing. The block
// goes when Ctrl-J is pressed again.
func TestAuditEnterEndsALineInsideTheMode(t *testing.T) {
	le := NewLineEditor(strings.NewReader("one\ntwo\r"))
	if _, err := le.ReadLine(); err == nil {
		t.Error("Enter sent the block, want it to end a line")
	}
	if got := le.pasted; len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Errorf("the held lines are %q, want both lines the breaks ended", got)
	}
}

// Escape abandons the block rather than the session, since a reader pressing it
// inside the mode means to discard what they have written rather than to leave.
func TestAuditEscapeAbandonsTheBlock(t *testing.T) {
	le := NewLineEditor(strings.NewReader("one\ntwo\x1b"))
	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Errorf("err = %v, want the interrupt", err)
	}
	if le.multilineMode() {
		t.Error("the mode survived the abandon")
	}
	if len(le.pasted) != 0 {
		t.Errorf("the block is %q, want it discarded", le.pasted)
	}
}

// Shift with enter ends a line in either state, since it means a break and a
// break is what it means on an idle prompt too. It is not the send key, so a
// reader holding it can still reach Ctrl-J or Enter to do that.
func TestAuditShiftEnterInsertsANewline(t *testing.T) {
	le := NewLineEditor(strings.NewReader("first\x1b[13;2usecond\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if got != "first\nsecond" {
		t.Errorf("got %q, want one newline from the shifted report, not two", got)
	}
}

// A shifted report is taken whole, so no escape or bracket reaches the message
// being composed.
func TestAuditShiftEnterLeavesNoBytesInTheMessage(t *testing.T) {
	le := NewLineEditor(strings.NewReader("text\x1b[13;2u\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("got %q, want no escape left in the message", got)
	}
}

// A key sequence read at the end of a read is discarded before the end of the
// input is reported, since the filter takes characters only. Leaving it queued
// would hand it to the line editor afterwards, where it would act on a key
// pressed while the filter was open.
func TestAuditReadByteDrainsASequenceBeforeTheEnd(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[A"))
	if _, err := le.ReadByte(); err == nil {
		t.Fatal("err = nil, want the end of the input reported")
	}
	if len(le.pendingKeys) != 0 {
		t.Errorf("pendingKeys = %v, want the sequence discarded", le.pendingKeys)
	}
}

// A sequence read by the filter is discarded rather than acted on, since the
// filter walks the catalogue rather than composing a message.
func TestAuditReadByteDiscardsASequence(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[Ax"))
	b, err := le.ReadByte()
	if err != nil {
		t.Fatalf("ReadByte: %v", err)
	}
	if b != 'x' {
		t.Errorf("byte = %q, want the sequence consumed and the character returned", b)
	}
	if len(le.pendingKeys) != 0 {
		t.Errorf("pendingKeys = %v, want the sequence discarded", le.pendingKeys)
	}
}

// A sequence at the end of a read is acted on before the end of the input is
// handled. A block whose last bytes are a sequence is followed by the end of
// the input rather than by a further byte, so handling the error first would
// leave the key with no effect at all.
func TestAuditSequenceAtTheEndOfAReadIsActedOn(t *testing.T) {
	le := NewLineEditor(strings.NewReader("one\r\x1b[A"))

	got := readLines(t, le, 1)
	if got[0] != "one" {
		t.Errorf("got %q, want the line submitted", got[0])
	}

	// The arrow is still acted on even though the input ended with it, so the
	// remembered line is what the read returns.
	line, err := le.ReadLine()
	if err == nil {
		t.Fatal("err = nil, want the end of the input reported")
	}
	if line != "one" {
		t.Errorf("line = %q, want the sequence acted on before the end of the input", line)
	}
}

// A sequence that begins like a mouse report is left alone, since taking it as
// an unknown sequence would leave the rest of the report to be read as keys.
func TestAuditSequenceBeginningLikeAMouseReportIsLeftAlone(t *testing.T) {
	for _, buf := range [][]byte{
		[]byte("\x1b[<64;1;1M"),
		[]byte("\x1b[<64"),
		[]byte("\x1b[M"),
		[]byte("\x1b[M\x60\x20\x20"),
	} {
		if _, _, ok := takeSequence(buf); ok {
			t.Errorf("%q was taken as a key sequence", buf)
		}
	}
}
