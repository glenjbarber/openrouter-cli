package tui

import (
	"strings"
	"testing"
	"time"
)

// The elapsed figure is what a reader watching a slow turn is looking at, so
// these check what it says and, more to the point, what it does not say.

func TestNoWorkMeansNoFigure(t *testing.T) {
	// A clock run from the epoch would read something enormous, which is
	// worse than reading nothing.
	if got := elapsedSince(time.Time{}); got != "" {
		t.Errorf("a turn that has not started reported %q", got)
	}
}

func TestAClockSetBackReadsZero(t *testing.T) {
	// A figure counting backwards is a thing that cannot have happened.
	if got := elapsedSince(time.Now().Add(time.Hour)); got == "" || strings.HasPrefix(got, "-") {
		t.Errorf("a clock set back reported %q", got)
	}
}

func TestTheFigureIsSecondsUntilAMinute(t *testing.T) {
	start := time.Now().Add(-3 * time.Second)
	got := elapsedSince(start)

	if got != "3s" {
		t.Errorf("three seconds reported %q", got)
	}
}

func TestTheFigureGrowsToMinutes(t *testing.T) {
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{59 * time.Second, "59s"},
		{61 * time.Second, "1m01s"},
		{90 * time.Second, "1m30s"},
		{125 * time.Second, "2m05s"},
	}
	for _, tc := range cases {
		got := elapsedSince(time.Now().Add(-tc.ago))
		if got != tc.want {
			t.Errorf("%s of work reported %q, want %q", tc.ago, got, tc.want)
		}
	}
}

// The seconds within a minute are not padded, so the figure does change width
// as it grows. That is only safe because the figure is at the end of the row,
// and this is what holds it there: the twiddle and the word are at a fixed
// offset from the left whatever the figure is.
func TestTheTwiddleDoesNotShiftAsTheFigureGrows(t *testing.T) {
	head := func(elapsed string) string {
		rows := Render(Frame{
			Spinner: spinnerFrames[0],
			Partial: "a",
			Elapsed: elapsed,
		}, 24, 80)
		for _, r := range rows {
			if strings.Contains(r, "thinking") {
				return r[:strings.Index(r, "thinking")+len("thinking")]
			}
		}
		return ""
	}

	short := head("9s")
	long := head("2m05s")

	if short == "" || long == "" {
		t.Fatalf("a row was not drawn: %q %q", short, long)
	}
	// The figure is appended, so the part before it is identical and the
	// twiddle sits at the same column in both.
	if short != long {
		t.Errorf("the row before the figure moved:\n%q\n%q", short, long)
	}
}

// The figure follows the word, so the left of the row still reads as the
// twiddle and the word before anything that measures them.
func TestTheFigureFollowsTheWord(t *testing.T) {
	rows := Render(Frame{
		Spinner: spinnerFrames[0],
		Partial: "a",
		Elapsed: "12s",
	}, 24, 80)

	var row string
	for _, r := range rows {
		if strings.Contains(r, "thinking") {
			row = r
		}
	}
	if row == "" {
		t.Fatal("no row held the twiddle")
	}
	figure := strings.Index(row, "12s")
	word := strings.Index(row, "thinking")
	if figure < 0 {
		t.Fatalf("the figure is not on the row: %q", row)
	}
	if word < 0 || figure < word {
		t.Errorf("the figure is not after the word: %q", row)
	}
}

func TestNoFigureMeansTheWordAlone(t *testing.T) {
	rows := Render(Frame{Spinner: spinnerFrames[0], Partial: "a"}, 24, 80)

	for _, r := range rows {
		if strings.Contains(r, "thinking") && strings.Contains(r, "s ") {
			t.Errorf("a row with no work behind it carries a figure: %q", r)
		}
	}
}

// The figure is written to the frame beside the twiddle and cleared with it.
func TestTheFigureIsClearedWithTheTwiddle(t *testing.T) {
	// The session needs a screen, since beginning work paints a frame.
	s := elapsedSession(t)

	s.beginWork()
	s.endWork()

	s.mu.Lock()
	elapsed, figure := s.frame.Elapsed, s.frame.Spinner
	s.mu.Unlock()

	if elapsed != "" {
		t.Errorf("the figure survived the twiddle: %q", elapsed)
	}
	if figure != "" {
		t.Errorf("the twiddle survived its own end: %q", figure)
	}
}

func TestTheFigureIsOnTheFrameWhileWorkRuns(t *testing.T) {
	s := elapsedSession(t)

	s.beginWork()
	t.Cleanup(s.endWork)

	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.frame.Elapsed != ""
	}, "the figure never reached the frame")
}

// elapsedSession builds a session that can paint, since beginning work paints
// a frame and a session with no screen would panic on the first repaint.
func elapsedSession(t *testing.T) *Session {
	t.Helper()
	sc, _ := screenCapture(t)
	conv := NewConversation()
	s := &Session{
		conv:      conv,
		mainConv:  conv,
		screen:    sc,
		spinner:   NewSpinner(),
		windows:   newContextLength(),
		approvals: newApprovalState(),
	}
	t.Cleanup(func() { s.spinner.Stop() })
	return s
}
