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

// Both a carriage return and a newline submit, since a terminal sends
// whichever its line discipline was set to.
func TestAuditBothBreaksSubmit(t *testing.T) {
	for _, in := range []string{"cr\r", "lf\n"} {
		le := NewLineEditor(strings.NewReader(in))
		got, err := le.ReadLine()
		if err != nil {
			t.Errorf("%q: err = %v", in, err)
			continue
		}
		if got != strings.TrimRight(in, "\r\n") {
			t.Errorf("%q: got %q, want the line submitted", in, got)
		}
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
