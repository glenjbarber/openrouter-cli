package tui

import (
	"io"
	"strings"
	"testing"
)

// A lone escape followed by ordinary input must still interrupt, since the
// prefix was held and then timed out.
func TestLoneEscapeAfterHold(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1bx"))
	_, err := le.ReadLine()
	if err != ErrInterrupt {
		t.Errorf("err = %v, want ErrInterrupt", err)
	}
}

// A report split across reads is held and consumed, and the text after it
// survives. This is the ssh case: a terminal writes a report in one piece, but
// a slow link delivers it in pieces, and the leading escape must not be read as
// a keypress or the session ends.
func TestSplitReportThenText(t *testing.T) {
	le := NewLineEditor(&splitReader{chunks: []string{"\x1b", "[<64;1;", "1Mok\r"}})

	var notches int
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

// splitReader hands back one chunk per Read, which is what a link delivering a
// report in pieces looks like to the reader.
type splitReader struct {
	chunks []string
}

func (r *splitReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[0])
	r.chunks = r.chunks[1:]
	return n, nil
}

// An arrow key must reach the key handler rather than being held as a report
// prefix and then reported as the end of the input. The up arrow is the
// shortest sequence that begins like a report and is not one.
func TestArrowIsNotAHeldPrefix(t *testing.T) {
	for _, seq := range []string{"\x1b[A", "\x1b[B", "\x1b[C", "\x1b[D"} {
		if mousePrefix([]byte(seq)) {
			t.Errorf("%q was held as a report prefix", seq)
		}
	}
}

// A report split across reads must be assembled rather than cut, since the
// link delivers it in pieces.
func TestSplitReportIsAssembled(t *testing.T) {
	le := NewLineEditor(&splitReader{chunks: []string{
		"\x1b[", "<64;", "1;1Mok\r",
	}})

	var notches int
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
