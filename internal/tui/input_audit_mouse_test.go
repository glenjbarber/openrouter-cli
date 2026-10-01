package tui

import (
	"io"
	"strings"
	"testing"
	"time"
)

// A report split at every possible byte boundary must be assembled. The
// failure this guards against was reported over ssh: the leading escape read
// as a key ends the line and closes the session. Every boundary is tried
// because a split at any one of them is a case a hand-picked example misses.
func TestAuditReportSplitAtEveryBoundaryIsAssembled(t *testing.T) {
	full := "\x1b[<64;10;20Mok\r"

	for cut := 1; cut < len(full)-1; cut++ {
		var notches int
		le := NewLineEditor(&splitReader{chunks: []string{full[:cut], full[cut:]}})
		le.OnMouse = func(int) { notches++ }

		got, err := le.ReadLine()
		if err != nil {
			t.Errorf("cut=%d: ReadLine: %v, so a split report ended the line", cut, err)
			continue
		}
		if got != "ok" {
			t.Errorf("cut=%d: got %q, want %q", cut, got, "ok")
		}
		if notches != 1 {
			t.Errorf("cut=%d: notches = %d, want 1", cut, notches)
		}
	}
}

// The six-byte form is a fixed width and is not held, so it is taken whole or
// not at all. A split after its third byte therefore hands back its opening
// escape as a key, which is the deliberate consequence of not holding it rather
// than an oversight. A split before that is still held, since a buffer of one or
// two bytes cannot yet be told from anything else.
func TestAuditX10ReportIsNotHeldOnceItsFormIsKnown(t *testing.T) {
	full := "\x1b[M\x60\x20\x20ok\r"

	for cut := 3; cut <= 5; cut++ {
		le := NewLineEditor(&splitReader{chunks: []string{full[:cut], full[cut:]}})
		if _, err := le.ReadLine(); err != ErrInterrupt {
			t.Errorf("cut=%d: err = %v, want the six-byte form handed back as keys", cut, err)
		}
	}

	// Arriving whole it is taken as a report, since nothing is left to hold.
	var notches int
	le := NewLineEditor(&splitReader{chunks: []string{full}})
	le.OnMouse = func(int) { notches++ }

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
	if notches != 1 {
		t.Errorf("notches = %d, want 1", notches)
	}
}

// A report arriving one byte per read, which is the ssh case in its worst
// form, must still be assembled rather than handed back as keys.
func TestAuditReportOneBytePerReadIsAssembled(t *testing.T) {
	var chunks []string
	for _, b := range []byte("\x1b[<64;10;20Mok\r") {
		chunks = append(chunks, string([]byte{b}))
	}

	var notches int
	le := NewLineEditor(&splitReader{chunks: chunks})
	le.OnMouse = func(int) { notches++ }

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
	if notches != 1 {
		t.Errorf("notches = %d, want 1", notches)
	}
}

// The deadline is per read rather than for the whole hold, so a report
// arriving over several reads with a gap between each is still assembled.
// Four gaps each longer than the timeout would exceed a deadline measured for
// the hold as a whole.
func TestAuditDeadlineIsPerReadNotForTheHold(t *testing.T) {
	r := &gappedReader{gap: prefixTimeout * 2, data: "\x1b[<64;10;20Mok\r"}

	var notches int
	le := NewLineEditor(r)
	le.OnMouse = func(int) { notches++ }

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
	if notches != 1 {
		t.Errorf("notches = %d, want 1", notches)
	}
}

// gappedReader returns its data one byte per read, pausing longer than the
// prefix timeout between reads. A file is not required, since a reader that
// blocks reports the end of its input rather than timing out.
type gappedReader struct {
	data string
	gap  time.Duration
	at   int
}

func (r *gappedReader) Read(p []byte) (int, error) {
	if r.at >= len(r.data) {
		return 0, io.EOF
	}
	time.Sleep(r.gap)
	p[0] = r.data[r.at]
	r.at++
	return 1, nil
}

// A prefix too short to recognise is still held, since a two-byte buffer
// cannot yet be told from a lone escape followed by something.
func TestAuditTwoBytePrefixIsHeld(t *testing.T) {
	if !mousePrefix([]byte("\x1b[")) {
		t.Error("a two-byte prefix was not held, so a report arriving a byte at a time is at risk")
	}
	if !mousePrefix([]byte("\x1b")) {
		t.Error("a lone escape was not held")
	}
}

// The six-byte form is not held, since it is a fixed width and is taken whole
// or not at all. An arrow must never be held either.
func TestAuditFixedWidthAndArrowFormsAreNotHeld(t *testing.T) {
	if mousePrefix([]byte("\x1b[M\x60\x20")) {
		t.Error("the six-byte form was held, so a partial report would be held and then cut")
	}
	for _, seq := range []string{"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D", "\x1b[H", "\x1b[F", "\x1b[3~"} {
		if mousePrefix([]byte(seq)) {
			t.Errorf("%q was held", seq)
		}
	}
}

// A prefix that never completes is handed back as keys, so a lone escape
// interrupts and an arrow reaches the key handler rather than either being
// reported as the end of the input.
func TestAuditIncompletePrefixIsHandedBackAsKeys(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[<64;10;20"))
	if _, err := le.ReadLine(); err == nil {
		t.Error("an incomplete report was not reported at all")
	}

	le = NewLineEditor(strings.NewReader("\x1b["))
	_, err := le.ReadLine()
	if err != ErrInterrupt {
		t.Errorf("err = %v, want the incomplete prefix handed back as an escape", err)
	}
}

// A wheel notch moves three lines, and scrolling down stops at the bottom
// rather than running past it.
func TestAuditScrollArithmeticAtTheEnds(t *testing.T) {
	if got := scrolledBy(0, mouseUp); got != scrollStep {
		t.Errorf("one notch up from the bottom = %d, want %d", got, scrollStep)
	}
	if got := scrolledBy(scrollStep, mouseDown); got != 0 {
		t.Errorf("scrolling down from one notch = %d, want 0", got)
	}
	if got := scrolledBy(1, mouseDown); got != 0 {
		t.Errorf("scrolling down from less than a notch = %d, want 0 rather than a negative", got)
	}
	if got := scrolledBy(7, mouseNone); got != 7 {
		t.Errorf("an event that is not the wheel moved the offset to %d", got)
	}
}

// Only the wheel is acted on: a press and a drag carry no direction, so they
// leave the view alone rather than scrolling on a gesture that means nothing
// here.
//
// The release form, which ends in a lower case m, is deliberately absent. A
// terminal in button tracking mode reports the wheel as a press and does not
// report a release for it, so there is no notch to double count, and the two
// forms are taken as one sequence so that neither is read as keys.
func TestAuditOnlyTheWheelIsActedOn(t *testing.T) {
	for _, seq := range []string{
		"\x1b[<0;10;20M",  // press
		"\x1b[<32;10;20M", // drag with the button held
	} {
		if got := parseMouse([]byte(seq)); got != mouseNone {
			t.Errorf("parseMouse(%q) = %d, want no direction", seq, got)
		}
	}

	for _, seq := range []string{"\x1b[<64;10;20M", "\x1b[<65;10;20M"} {
		if got := parseMouse([]byte(seq)); got == mouseNone {
			t.Errorf("parseMouse(%q) = %d, want a wheel direction", seq, got)
		}
	}
}
