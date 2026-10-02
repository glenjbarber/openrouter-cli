package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// The ramp was written before anything called it, so it carried no test of its
// own. These cover the maths and the wiring together: that a step produces a
// sequence, that the frame carries one, and that the screen writes it around
// the twiddle and no further.

// screenCapture returns a screen writing into a file, so a test can read the
// bytes without a terminal.
func screenCapture(t *testing.T) (*Screen, func() string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "screen")
	if err != nil {
		t.Fatalf("creating the capture: %v", err)
	}
	sc := &Screen{out: out, in: out, height: 24, width: 80}
	return sc, func() string {
		data, err := os.ReadFile(out.Name())
		if err != nil {
			t.Fatalf("reading the capture: %v", err)
		}
		return string(data)
	}
}

func TestATwiddleStepProducesASequence(t *testing.T) {
	seq := twiddleTint(0)
	if !strings.HasPrefix(seq, "\x1b[38;5;") {
		t.Errorf("step 0 produced %q, want a cube foreground", seq)
	}
	if !strings.HasSuffix(seq, "m") {
		t.Errorf("step 0 produced %q, want it terminated", seq)
	}
}

func TestTheRampIsNotOneColor(t *testing.T) {
	// Every step must not draw the same color, or the ramp is a shade rather
	// than a scroll and the figure is not worth coloring.
	seen := map[string]bool{}
	for step := 0; step < twiddleSteps; step++ {
		seen[twiddleTint(step)] = true
	}
	if len(seen) < len(primaryColors) {
		t.Errorf("the ramp produced %d distinct colors over %d steps, want at least one per primary",
			len(seen), twiddleSteps)
	}
}

func TestTheRampRepeatsExactly(t *testing.T) {
	// A turn long enough to go round the circuit many times must repeat it
	// rather than run off the end of the table.
	for step := 0; step < twiddleSteps; step++ {
		if got, want := twiddleTint(step+twiddleSteps), twiddleTint(step); got != want {
			t.Errorf("step %d gave %q and step %d gave %q, want the same",
				step, want, step+twiddleSteps, got)
		}
	}
}

func TestTheRampIsDefinedBeforeItsStart(t *testing.T) {
	// A step indexes a table, and a table read out of range would panic rather
	// than draw. The step before the first is the last.
	if twiddleTint(-1) != twiddleTint(twiddleSteps-1) {
		t.Error("a negative step did not wrap onto the end of the ramp")
	}
}

func TestTheRampTurnsAtTheRateTheTwiddleDoes(t *testing.T) {
	// The ramp is divided by the repaint interval, so a circuit is a circuit
	// rather than however long the division happens to make it.
	if want := int(twiddleCircuit / spinnerInterval); twiddleSteps != want {
		t.Errorf("the ramp is %d steps for a %s circuit at a %s interval, want %d",
			twiddleSteps, twiddleCircuit, spinnerInterval, want)
	}
	if twiddleSteps < 2 {
		t.Errorf("the ramp has %d steps, which is not a scroll", twiddleSteps)
	}
}

func TestTheRampIsNeverTheBlackOfTheCube(t *testing.T) {
	// A component reaching zero is floored, so a primary on its way down does
	// not become the black that reads as nothing on a dark terminal.
	for step := 0; step < twiddleSteps; step++ {
		if twiddleTint(step) == "\x1b[38;5;0m" {
			t.Fatalf("step %d drew the black of the cube", step)
		}
	}
}

// The frame leaves the renderer as plain text, so a copy out of the pane cannot
// carry a sequence. This is what makes the color a screen concern.
func TestTheTwiddleRowCarriesNoSequenceOutOfTheRenderer(t *testing.T) {
	rows := Render(Frame{Spinner: spinnerFrames[0], Partial: "a"}, 24, 80)

	found := false
	for _, row := range rows {
		if strings.Contains(row, "thinking") {
			found = true
			if strings.Contains(row, "\x1b") {
				t.Errorf("the twiddle row carries an escape: %q", row)
			}
		}
	}
	if !found {
		t.Error("no row held the twiddle, so nothing was tested")
	}
}

func TestTheRendererReportsTheTwiddleRow(t *testing.T) {
	frame := Frame{Spinner: spinnerFrames[0], Partial: "a"}
	rows, _, row := render(frame, 24, 80)

	if row < 0 || row >= len(rows) {
		t.Fatalf("the reported row is %d, outside a frame of %d rows", row, len(rows))
	}
	if !strings.Contains(rows[row], "thinking") {
		t.Errorf("row %d is %q, which is not the twiddle", row, rows[row])
	}
}

func TestAFrameWithNoTwiddleReportsNoRow(t *testing.T) {
	rows, _, row := render(Frame{Partial: "a"}, 24, 80)

	if row != -1 {
		t.Errorf("a frame with no twiddle reported row %d of %d", row, len(rows))
	}
}

// The pane is trimmed to what fits, so the row the twiddle is appended at is
// not the row it is finally drawn on. The reported row is the final one.
func TestTheReportedRowFollowsThePane(t *testing.T) {
	var reply []string
	for i := 0; i < 300; i++ {
		reply = append(reply, "a line of history")
	}
	frame := Frame{Reply: reply, Spinner: spinnerFrames[0], Partial: "working"}
	rows, _, row := render(frame, 24, 80)

	if row < 0 || row >= len(rows) {
		t.Fatalf("the reported row is %d, outside a frame of %d rows", row, len(rows))
	}
	if !strings.Contains(rows[row], "thinking") {
		t.Errorf("row %d is %q, which is not the twiddle", row, rows[row])
	}
}

// A scrolled pane has no twiddle on it. The figure is the newest line of the
// pane, so any offset takes it off the screen, and no row may be reported. A
// tint naming a row that has taken its place would color a line of history a
// reader is reading.
func TestAScrolledPaneReportsNoRow(t *testing.T) {
	var reply []string
	for i := 0; i < 40; i++ {
		reply = append(reply, "a line of history")
	}
	for _, scroll := range []int{1, 5, 20} {
		frame := Frame{Reply: reply, Spinner: spinnerFrames[0], Scroll: scroll}
		rows, _, row := render(frame, 24, 80)

		if strings.Contains(strings.Join(rows, "\n"), "thinking") {
			t.Errorf("at scroll %d the twiddle is drawn, so a row was expected", scroll)
		}
		if row != -1 {
			t.Errorf("at scroll %d a frame with no twiddle reported row %d", scroll, row)
		}
	}
}

// A short conversation cannot be scrolled far enough to lose the twiddle, so
// the figure is still on it and the row is still reported. The offset is
// clamped against the pane height, which is what makes this the case.
func TestAShortPaneKeepsTheTwiddleThroughAnyScroll(t *testing.T) {
	frame := Frame{
		Reply:   []string{"one", "two"},
		Spinner: spinnerFrames[0],
		Scroll:  100,
	}
	rows, _, row := render(frame, 24, 80)

	if row < 0 || row >= len(rows) {
		t.Fatalf("the reported row is %d, outside a frame of %d rows", row, len(rows))
	}
	if !strings.Contains(rows[row], "thinking") {
		t.Errorf("row %d is %q, which is not the twiddle", row, rows[row])
	}
}

// The color covers the whole row, the twiddle and the word beside it, and is
// reset before the next row so that it does not run on into whatever follows.
func TestTheSequenceCoversTheWholeRow(t *testing.T) {
	sc, read := screenCapture(t)

	rows := []string{"before", "⠋ thinking", "after"}
	sc.DrawTinted(rows, tint{row: 1, sequence: twiddleTint(0), figure: "⠋"})
	got := read()

	marker := strings.Index(got, "\x1b[38;5;")
	if marker < 0 {
		t.Fatalf("no color was written at all: %q", got)
	}
	rest := got[marker:]

	// The word is inside the color, since the point of the row is that the
	// whole indicator is drawn as one thing.
	figureAt := strings.Index(rest, "⠋")
	wordAt := strings.Index(rest, "thinking")
	if figureAt < 0 || wordAt < 0 {
		t.Fatalf("the row is missing from what was written: %q", got)
	}
	if wordAt < figureAt {
		t.Errorf("the word was written before the figure: %q", got)
	}

	resetAt := strings.Index(rest, seqResetAttr)
	if resetAt < 0 {
		t.Fatalf("the color was never reset: %q", got)
	}
	if resetAt < wordAt {
		t.Errorf("the color was reset before the word beside the twiddle: %q", got)
	}

	// The reset has to come before the next row is drawn, or the color runs
	// on into whatever the frame drew after it. The line break and the next
	// row's own reset sit between them, so what is checked is that the
	// sequence ends at the reset rather than that nothing follows it.
	tail := rest[resetAt:]
	next := strings.Index(tail, "after")
	if next < 0 {
		t.Fatalf("the row after the twiddle was not drawn: %q", got)
	}
	if strings.Contains(tail[:next], "\x1b[38;5;") {
		t.Errorf("the color ran on into the next row: %q", tail[:next])
	}
}

// The row is built as plain text whatever the color is doing, so a selection
// out of the pane carries the characters and not the sequence. This is what
// makes coloring the word beside the twiddle safe.
func TestTheTwiddleRowIsStillPlainTextForASelection(t *testing.T) {
	rows := Render(Frame{Spinner: spinnerFrames[0], Partial: "a"}, 24, 80)

	for _, row := range rows {
		if strings.Contains(row, "thinking") && strings.Contains(row, "\x1b") {
			t.Errorf("the twiddle row carries an escape: %q", row)
		}
	}
}

// The braille figures are several bytes each, so a count of columns rather than
// of bytes would cut one in half and the terminal would draw the tail of it as
// text. The figure travels as text for that reason, and the whole row is written
// rather than a slice of it.
func TestTheFigureIsNotCutInHalf(t *testing.T) {
	sc, read := screenCapture(t)

	rows := []string{"⠋ thinking"}
	sc.DrawTinted(rows, tint{row: 0, sequence: "\x1b[38;5;1m", figure: spinnerFrames[0]})
	got := read()

	marker := strings.Index(got, "\x1b[38;5;1m")
	if marker < 0 {
		t.Fatalf("no color was written: %q", got)
	}
	after := got[marker+len("\x1b[38;5;1m"):]
	if !strings.HasPrefix(after, spinnerFrames[0]) {
		t.Errorf("the figure was cut: it begins %q", after)
	}
}

func TestAFrameWithNoTintIsDrawnAsItAlwaysWas(t *testing.T) {
	rows := []string{"one", "two"}

	plain, readPlain := screenCapture(t)
	plain.Draw(rows)
	before := readPlain()

	tinted, readTinted := screenCapture(t)
	tinted.DrawTinted(rows, tint{})
	after := readTinted()

	if before != after {
		t.Errorf("an empty tint changed the output:\n%q\n%q", before, after)
	}
}

func TestATintNamingNoRowIsNotApplied(t *testing.T) {
	sc, read := screenCapture(t)

	sc.DrawTinted([]string{"one", "two"}, tint{
		row: 9, sequence: "\x1b[38;5;1m", figure: "one",
	})

	if strings.Contains(read(), "\x1b[38;5;1m") {
		t.Error("a tint naming no row was applied")
	}
}

// A row whose text is not the twiddle is left alone, since the figure is what
// decides whether the color applies and not the index alone.
func TestATintNotMatchingTheRowIsNotApplied(t *testing.T) {
	sc, read := screenCapture(t)

	sc.DrawTinted([]string{"one", "two"}, tint{
		row: 1, sequence: "\x1b[38;5;1m", figure: "⠋",
	})

	if strings.Contains(read(), "\x1b[38;5;1m") {
		t.Error("a tint whose figure is not on the row was applied")
	}
}

// The twiddle color is governed by the color setting like every other color.
// These cover the gate in DrawFrame and the same gate reached through the
// session, so that /color and the key cannot disagree about it.

// twiddleFrame is a frame with a twiddle on it, and the rows it draws as.
func twiddleFrame(t *testing.T) ([]string, [][]span, int) {
	t.Helper()
	f := Frame{Title: "t", Partial: "a", Spinner: spinnerFrames[0], Elapsed: "1s", Input: "x"}
	rows, spans, _, twiddle := renderStyled(f, 24, 60)
	if twiddle < 0 {
		t.Fatal("no twiddle row")
	}
	return rows, spans, twiddle
}

// With color off a frame carrying a tint is the frame with no tint at all.
func TestATintIsIgnoredWithColorOff(t *testing.T) {
	rows, spans, twiddle := twiddleFrame(t)
	tintSeq := twiddleTint(2)

	tinted, readTinted := screenCapture(t)
	tinted.height, tinted.width = 24, 60
	tinted.DrawFrame(rows, framePaint{spans: spans, twiddle: twiddle, sequence: tintSeq, figure: spinnerFrames[0]})

	bare, readBare := screenCapture(t)
	bare.height, bare.width = 24, 60
	bare.DrawFrame(rows, framePaint{spans: spans, twiddle: -1})

	if readTinted() != readBare() {
		t.Errorf("a tint changed a color-off frame:\n%q\n%q", readTinted(), readBare())
	}
	if strings.Contains(readTinted(), tintSeq) {
		t.Error("the twiddle sequence was written with color off")
	}
}

// With color on the sequence is written and reset around the row, as before.
func TestATintIsAppliedWithColorOn(t *testing.T) {
	rows, spans, twiddle := twiddleFrame(t)
	pal := newPalette(config.Theme{})
	tintSeq := twiddleTint(2)
	sc, read := screenCapture(t)
	sc.height, sc.width = 24, 60
	sc.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: twiddle, sequence: tintSeq, figure: spinnerFrames[0]})
	if !strings.Contains(read(), tintSeq+rows[twiddle]+seqResetAttr) {
		t.Errorf("the twiddle row is not colored with color on:\n%q", read())
	}
}

// /color on and off toggle the twiddle color on the next paint, through the
// real command path.
func TestColorCommandTogglesTheTwiddleColor(t *testing.T) {
	s, capture := auditSession(t, "")
	tintSeq := twiddleTint(2)
	s.mu.Lock()
	s.frame.Busy = true
	s.frame.Spinner = spinnerFrames[0]
	s.frame.Elapsed = "1s"
	s.frame.Tint = tintSeq
	s.mu.Unlock()

	if got := paintedFrame(t, s, capture); strings.Contains(got, tintSeq) {
		t.Errorf("a fresh session paints the twiddle color:\n%q", got)
	}
	s.command("/color on")
	if got := paintedFrame(t, s, capture); !strings.Contains(got, tintSeq+spinnerFrames[0]) {
		t.Errorf("the paint after /color on has no twiddle color:\n%q", got)
	}
	s.command("/color off")
	if got := paintedFrame(t, s, capture); strings.Contains(got, tintSeq) {
		t.Errorf("the paint after /color off still has the twiddle color:\n%q", got)
	}
}
