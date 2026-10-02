package tui

import (
	"os"
	"strings"
	"testing"
)

// mouseScreen builds a screen writing into a file, so the sequences sent to the
// terminal can be read back.
func mouseScreen(t *testing.T) (*Screen, func() string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "mouse")
	if err != nil {
		t.Fatalf("creating the capture: %v", err)
	}
	return &Screen{out: out, in: out, height: 24, width: 80}, func() string {
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatalf("reading the capture: %v", err)
		}
		return string(data)
	}
}

// A terminal is asked for the SGR encoding as well as the mode. Without it a
// terminal sends the older form, where a coordinate is one byte, and a pane
// past 223 in either direction cannot be scrolled at all.
func TestReportingAsksForTheSGREncoding(t *testing.T) {
	sc, read := mouseScreen(t)

	sc.SetMouse(true)
	got := read()

	if !strings.Contains(got, seqMouseOn) {
		t.Errorf("the mode was not turned on: %q", got)
	}
	if !strings.Contains(got, seqMouseSGROn) {
		t.Errorf("the SGR encoding was not asked for: %q", got)
	}
	// The encoding is asked for before the mode, so a terminal is never
	// reporting in a form this reader cannot parse.
	if strings.Index(got, seqMouseSGROn) > strings.Index(got, seqMouseOn) {
		t.Errorf("the mode was turned on before the encoding: %q", got)
	}
}

func TestTurningReportingOffResetsBoth(t *testing.T) {
	sc, read := mouseScreen(t)

	sc.SetMouse(true)
	sc.SetMouse(false)
	got := read()

	if !strings.Contains(got, seqMouseOff) {
		t.Errorf("the mode was not turned off: %q", got)
	}
	if !strings.Contains(got, seqMouseSGROff) {
		t.Errorf("the encoding was left on: %q", got)
	}
	// The mode goes first, so no report is sent in an encoding that is being
	// switched out from under the terminal.
	if strings.Index(got, seqMouseOff) > strings.Index(got, seqMouseSGROff) {
		t.Errorf("the encoding was reset before the mode: %q", got)
	}
	if sc.Mouse() {
		t.Error("the screen still believes reporting is on")
	}
}

func TestTurningReportingOffTwiceWritesNothingMore(t *testing.T) {
	sc, read := mouseScreen(t)

	sc.SetMouse(true)
	sc.SetMouse(false)
	before := read()
	sc.SetMouse(false)

	if read() != before {
		t.Error("reporting was turned off again when it was already off")
	}
}

// The terminal is put back on the way out, encoding included: a terminal left
// in the SGR form reports in it to whatever runs next.
func TestClosingResetsTheEncoding(t *testing.T) {
	sc, read := mouseScreen(t)
	sc.SetMouse(true)

	sc.Close()
	got := read()

	if !strings.Contains(got, seqMouseSGROff) {
		t.Errorf("closing left the SGR encoding on: %q", got)
	}
	if !strings.Contains(got, seqMouseOff) {
		t.Errorf("closing left the mode on: %q", got)
	}
}

// A copy must work with reporting off, since a reader who turned reporting off
// did so to be able to select text, and copying is the same gesture.
func TestACopyWorksWithReportingOff(t *testing.T) {
	sc, read := mouseScreen(t)
	sc.SetMouse(true)
	sc.SetMouse(false)

	sc.copyToClipboard("the text")

	if !strings.Contains(read(), seqCopyPrefix) {
		t.Error("the copy was not written with reporting off")
	}
}

// A copy while reporting is on must also work, since the sequence travels the
// same path as the drawing.
func TestACopyWorksWithReportingOn(t *testing.T) {
	sc, read := mouseScreen(t)
	sc.SetMouse(true)

	sc.copyToClipboard("the text")

	if !strings.Contains(read(), seqCopyPrefix) {
		t.Error("the copy was not written with reporting on")
	}
}

// The wheel the parser reads is the SGR form, which is why the encoding is
// asked for. A report in that form is acted on.
func TestAnSGRWheelReportScrolls(t *testing.T) {
	// The wheel up report in the SGR form, as a terminal sends it once asked.
	up := []byte("\x1b[<64;10;5M")
	down := []byte("\x1b[<65;10;5M")

	if got := parseMouse(up); got != mouseUp {
		t.Errorf("an SGR wheel up read as %d, want %d", got, mouseUp)
	}
	if got := parseMouse(down); got != mouseDown {
		t.Errorf("an SGR wheel down read as %d, want %d", got, mouseDown)
	}
}
