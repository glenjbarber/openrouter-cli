package tui

import (
	"strings"
	"testing"
)

// A paste must be taken whole wherever it begins in the buffer, not only at
// the front of it. Measuring the end of the block from the front rather than
// from the start of the block both cut the text short and pointed the
// remainder past the end of the buffer, which is a panic rather than a
// misread.
func TestAuditPasteAfterTypedTextIsTakenWhole(t *testing.T) {
	body := pasteStart + "one\ntwo" + pasteEnd

	text, rest, ok := takePaste([]byte("ab" + body + "cd"))
	if !ok {
		t.Fatal("takePaste did not find a complete paste after typed text")
	}
	if text != "one\ntwo" {
		t.Errorf("text = %q, want %q", text, "one\ntwo")
	}
	if string(rest) != "cd" {
		t.Errorf("rest = %q, want %q", string(rest), "cd")
	}
}

// A stray closing marker ahead of the opening one belongs to a block that has
// already been taken, and must not be taken as the end of this one. The two
// offsets are measured from different places for that reason.
func TestAuditStrayPasteEndIsNotTakenAsTheEnd(t *testing.T) {
	buf := []byte(pasteEnd + "tail" + pasteStart + "body" + pasteEnd + "rest")

	text, rest, ok := takePaste(buf)
	if !ok {
		t.Fatal("takePaste did not find the complete block")
	}
	if text != "body" {
		t.Errorf("text = %q, want %q", text, "body")
	}
	if string(rest) != "rest" {
		t.Errorf("rest = %q, want %q", string(rest), "rest")
	}
}

// A paste larger than one read block has not arrived whole when it is first
// looked at. Its opening marker is the shape of a four-byte key sequence, so
// reading it as one and the body as typing submits the line at the first
// newline inside the paste, which is what the markers exist to prevent.
func TestAuditLargePasteArrivingAcrossReadsIsOneMessage(t *testing.T) {
	body := strings.Repeat("x", readChunk*4) + "\nsecond line\n"

	le := NewLineEditor(strings.NewReader(pasteStart + body + pasteEnd + "\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	want := strings.TrimRight(body, "\n")
	if got != want {
		t.Errorf("got %d bytes, want %d, so a large paste was not taken whole", len(got), len(want))
	}
}

// A paste arriving a byte at a time, which is what a slow link delivers, must
// be assembled rather than read as keys. The leading escape of the opening
// marker is the byte that ends the session when it is read as a key.
func TestAuditPasteSplitAtEveryBoundaryIsOneMessage(t *testing.T) {
	full := pasteStart + "one\ntwo" + pasteEnd + "ok\r"

	for cut := 1; cut < len(full)-1; cut++ {
		le := NewLineEditor(&splitReader{chunks: []string{full[:cut], full[cut:]}})
		got, err := le.ReadLine()
		if err != nil {
			t.Errorf("cut=%d: ReadLine: %v", cut, err)
			continue
		}
		if got != "one\ntwo\nok" {
			t.Errorf("cut=%d: got %q, want the paste and the typed text joined", cut, got)
		}
	}
}

// A prefix of the opening marker is held too, since the marker may not have
// arrived whole. Each prefix is a candidate on its own account.
func TestAuditPasteMarkerPrefixesAreHeld(t *testing.T) {
	for i := 1; i < len(pasteStart); i++ {
		if !pastePrefix([]byte(pasteStart[:i])) {
			t.Errorf("prefix %q was not held", pasteStart[:i])
		}
	}
	if pastePrefix([]byte(pasteStart + "body" + pasteEnd)) {
		t.Error("a complete block was held as a prefix")
	}
	if pastePrefix([]byte("\x1b[A")) {
		t.Error("an arrow was held as a paste prefix")
	}
	if pastePrefix(nil) {
		t.Error("an empty buffer was held as a prefix")
	}
}

// A paste of any size must be bounded, since an unbounded block is a buffer
// that grows with whatever arrives. The paste is stored as lines, and the
// renderer cuts the rows it shows, so the bound that matters here is on what
// the editor holds.
func TestAuditHugePasteDoesNotPanicOrHang(t *testing.T) {
	body := strings.Repeat("line\n", 20000)

	le := NewLineEditor(strings.NewReader(pasteStart + body + pasteEnd + "\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if strings.Count(got, "\n") != 19999 {
		t.Errorf("got %d lines, want 19999", strings.Count(got, "\n"))
	}
}

// A paste joined with what is typed after it is one message, with the paste
// first, since the paste was in the buffer before the typing began.
func TestAuditPasteComesBeforeWhatIsTypedAfter(t *testing.T) {
	le := NewLineEditor(strings.NewReader(pasteStart + "first\nsecond" + pasteEnd + " typed\r"))
	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "first\nsecond\n typed" {
		t.Errorf("got %q, want the paste first and the typed text after it", got)
	}
}
