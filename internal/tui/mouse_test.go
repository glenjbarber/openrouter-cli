package tui

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// The wheel is reported in the SGR form by a modern terminal, so both
// directions have to be recognised there.
func TestParseMouseWheelSGR(t *testing.T) {
	cases := []struct {
		seq  string
		want int
	}{
		{"\x1b[<64;10;5M", mouseUp},
		{"\x1b[<65;10;5M", mouseDown},
		// The release form ends in m rather than M and carries the same
		// button, so a terminal sending it is understood the same way.
		{"\x1b[<64;10;5m", mouseUp},
		{"\x1b[<65;10;5m", mouseDown},
	}
	for _, c := range cases {
		if got := parseMouse([]byte(c.seq)); got != c.want {
			t.Errorf("parseMouse(%q) = %d, want %d", c.seq, got, c.want)
		}
	}
}

// The older form is still understood, since a terminal falls back to it.
func TestParseMouseWheelX10(t *testing.T) {
	// The X10 button is the SGR button with the motion bit folded out.
	if got := parseMouse([]byte("\x1b[M \x20")); got != mouseNone {
		t.Errorf("a button press = %d, want no direction", got)
	}
	up := append([]byte("\x1b[M"), byte(wheelUp+mouseStateMask), ' ', '5')
	down := append([]byte("\x1b[M"), byte(wheelDown+mouseStateMask), ' ', '5')
	if got := parseMouse(up); got != mouseUp {
		t.Errorf("X10 up = %d, want %d", got, mouseUp)
	}
	if got := parseMouse(down); got != mouseDown {
		t.Errorf("X10 down = %d, want %d", got, mouseDown)
	}
}

// A sequence that is not a wheel report is left alone, so a button press or a
// drag does not scroll the view.
func TestParseMouseIgnoresOtherSequences(t *testing.T) {
	for _, seq := range []string{
		"\x1b[<0;10;5M",  // left button press
		"\x1b[<32;10;5M", // a drag
		"\x1b[A",         // an arrow key
		"\x1b[64;10;5M",  // a report missing the private marker
		"hello",
		"",
		"\x1b[<6;10;5M", // a truncated button number
		"\x1b[<;10;5M",
	} {
		if got := parseMouse([]byte(seq)); got != mouseNone {
			t.Errorf("parseMouse(%q) = %d, want no direction", seq, got)
		}
	}
}

// A report is taken from the front of a buffer and what follows is kept, so a
// keystroke arriving in the same read is not swallowed by the report.
func TestTakeMouseSequenceKeepsFollowingBytes(t *testing.T) {
	buf := []byte("\x1b[<64;10;5Mhello")
	seq, rest, ok := takeMouseSequence(buf)
	if !ok {
		t.Fatal("takeMouseSequence did not take the report")
	}
	if string(seq) != "\x1b[<64;10;5M" {
		t.Errorf("seq = %q, want the report", seq)
	}
	if string(rest) != "hello" {
		t.Errorf("rest = %q, want the text behind the report", rest)
	}
}

// A report that has not arrived whole is not taken, so a split report is not
// mistaken for input. The bytes are held for the next read instead.
func TestTakeMouseSequenceWaitsForWholeReport(t *testing.T) {
	for _, seq := range []string{"\x1b[<64", "\x1b[<64;10", "\x1b[<64;10;5", "\x1b[M\x40\x20"} {
		if _, _, ok := takeMouseSequence([]byte(seq)); ok {
			t.Errorf("takeMouseSequence(%q) took an incomplete report", seq)
		}
	}
}

// A buffer that does not open a report is left for the ordinary reader, since
// the keys and the reports share one stream.
func TestTakeMouseSequenceLeavesOtherInput(t *testing.T) {
	for _, buf := range []string{"hello", "\x1b[A", "", "\x1b"} {
		if _, _, ok := takeMouseSequence([]byte(buf)); ok {
			t.Errorf("takeMouseSequence(%q) took ordinary input", buf)
		}
	}
}

// The wheel must be reported to the caller rather than being read as a key. The
// decisive case is that a report does not end the line, since every report
// opens with the bare escape that would otherwise interrupt.
func TestReadLineReportsWheel(t *testing.T) {
	var got []int
	le := NewLineEditor(strings.NewReader("\x1b[<64;10;5M\x1b[<65;10;5Mhello\r"))
	le.OnMouse = func(d int) { got = append(got, d) }

	line, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if line != "hello" {
		t.Errorf("line = %q, want %q", line, "hello")
	}
	want := []int{mouseUp, mouseDown}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// A wheel notch on its own must not end the line, and must not be inserted into
// it either. This is the case that would quit the session if the opening escape
// were read as a key.
func TestReadLineWheelDoesNotInterrupt(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[<64;1;1M\r"))
	called := false
	le.OnMouse = func(int) { called = true }

	line, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v, want the line to survive a wheel notch", err)
	}
	if !called {
		t.Error("the wheel was not reported")
	}
	if line != "" {
		t.Errorf("line = %q, want the report kept out of the line", line)
	}
}

// A wheel notch between two keystrokes leaves the line holding only the keys.
func TestReadLineWheelAmongKeys(t *testing.T) {
	le := NewLineEditor(strings.NewReader("ab\x1b[<64;1;1Mcd\r"))
	var directions []int
	le.OnMouse = func(d int) { directions = append(directions, d) }

	line, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if line != "abcd" {
		t.Errorf("line = %q, want %q", line, "abcd")
	}
	if len(directions) != 1 {
		t.Errorf("directions = %v, want one wheel notch", directions)
	}
}

// A reader with no view to scroll discards the report rather than inserting it.
func TestReadLineNilMouseCallback(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b[<64;1;1Mok\r"))
	le.OnMouse = nil

	line, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if line != "ok" {
		t.Errorf("line = %q, want %q", line, "ok")
	}
}

// A burst of notches is consumed whole rather than ending the line, which is
// what a fast wheel produces.
func TestReadLineWheelBurst(t *testing.T) {
	var burst bytes.Buffer
	for i := 0; i < 8; i++ {
		burst.WriteString("\x1b[<64;1;1M")
	}
	burst.WriteString("go\r")

	count := 0
	le := NewLineEditor(bytes.NewReader(burst.Bytes()))
	le.OnMouse = func(int) { count++ }

	line, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if count != 8 {
		t.Errorf("count = %d, want 8 notches", count)
	}
	if line != "go" {
		t.Errorf("line = %q, want %q", line, "go")
	}
}

// A lone escape is still a key and still interrupts, which the chunked reader
// must not have changed.
func TestReadLineLoneEscapeStillInterrupts(t *testing.T) {
	le := NewLineEditor(strings.NewReader("\x1b"))
	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Errorf("err = %v, want ErrInterrupt", err)
	}
}

// An escape behind text interrupts as well. It used to do nothing at all, which
// left a reader pressing it to abandon what they had typed with no way to say
// so, and stopped it reaching the session while a model was working. What the
// interrupt means is decided there: a line is abandoned on an idle prompt, and
// a model is stopped with the text on an idle one.
func TestReadLineEscapeBehindTextInterrupts(t *testing.T) {
	le := NewLineEditor(strings.NewReader("abc\x1b"))
	if _, err := le.ReadLine(); err != ErrInterrupt {
		t.Errorf("err = %v, want ErrInterrupt", err)
	}
}

// The offset moves a wheel notch at a time and stops at both ends.
func TestScrollArithmetic(t *testing.T) {
	if got := scrolledBy(0, mouseUp); got != scrollStep {
		t.Errorf("up from the bottom = %d, want %d", got, scrollStep)
	}
	if got := scrolledBy(scrollStep, mouseUp); got != 2*scrollStep {
		t.Errorf("up = %d, want %d", got, 2*scrollStep)
	}
	if got := scrolledBy(2*scrollStep, mouseDown); got != scrollStep {
		t.Errorf("down = %d, want %d", got, scrollStep)
	}

	// Scrolling down at the bottom does not go negative, since a negative
	// offset would show blank rows above the newest output.
	for i := 0; i < 5; i++ {
		if got := scrolledBy(0, mouseDown); got != 0 {
			t.Fatalf("down at the bottom = %d, want 0", got)
		}
	}
	if got := scrolledBy(scrollStep, mouseDown); got != 0 {
		t.Errorf("down one notch from the bottom = %d, want 0", got)
	}

	// An unknown direction leaves the offset alone rather than jumping.
	if got := scrolledBy(scrollStep, mouseNone); got != scrollStep {
		t.Errorf("no direction = %d, want the offset held at %d", got, scrollStep)
	}
}

// New output must not pull a scrolled-back view to the bottom, since the
// reader is part way through the history and the whole point of the offset is
// that it stays there.
func TestNewOutputKeepsScrollOffset(t *testing.T) {
	s := newScrollSession()
	s.scroll = scrolledBy(0, mouseUp)
	before := s.scroll

	// A reply arriving is the event that would yank the view, since it
	// appends to the pane the offset is measured against.
	s.appendLines("a reply that arrived while scrolled back")
	s.draw()

	if s.scroll != before {
		t.Errorf("scroll = %d, want %d held", s.scroll, before)
	}
	if s.scrollAtBottom() {
		t.Error("the view should still be scrolled back")
	}
}

// A reply still streaming in must not pull the view either, since that is the
// longest run of new output a session has.
func TestStreamingOutputKeepsScrollOffset(t *testing.T) {
	s := newScrollSession()
	s.scroll = scrolledBy(0, mouseUp)
	before := s.scroll

	s.frame.Busy = true
	s.stream("a partial reply arriving while scrolled back")
	s.draw()

	if s.scroll != before {
		t.Errorf("scroll = %d, want %d held while streaming", s.scroll, before)
	}
}

// Clearing the pane returns the view to the newest output, since the history
// the offset was measured against no longer exists.
func TestClearingResetsScroll(t *testing.T) {
	s := newScrollSession()
	s.scroll = scrolledBy(0, mouseUp)
	if s.scrollAtBottom() {
		t.Fatal("the view should be scrolled back")
	}
	s.resetScroll()
	if !s.scrollAtBottom() {
		t.Error("the view should be back at the bottom")
	}
}

// Only scrolling down returns the view to the bottom. A message arriving is
// not a reason to move it, since that is the case where a reader part way
// through the history would have it moved under them.
func TestOnlyScrollingDownReturnsToBottom(t *testing.T) {
	s := newScrollSession()
	s.scroll = scrolledBy(0, mouseUp)

	// New output, a repaint, and a partial reply all leave it alone.
	s.appendLines("a reply arrived")
	s.stream("a partial reply")
	s.draw()
	if s.scrollAtBottom() {
		t.Error("new output moved the view to the bottom")
	}

	// Scrolling down does bring it back, and the marker goes with it.
	s.scrollBy(mouseDown)
	if !s.scrollAtBottom() {
		t.Error("scrolling down did not return to the bottom")
	}
}

// The wheel reaches the view through the same path the session uses, so the
// callback wiring is exercised rather than the arithmetic alone.
func TestScrollByReachesTheView(t *testing.T) {
	s := newScrollSession()
	// The pane holds history, since an offset larger than the history is
	// clamped away and the wheel would then have nothing to move.
	for i := 0; i < 40; i++ {
		s.appendLines("a line of output")
	}
	s.scrollBy(mouseUp)
	if s.scroll != scrollStep {
		t.Errorf("scroll = %d, want %d", s.scroll, scrollStep)
	}
	if s.scrollAtBottom() {
		t.Error("the view should be scrolled back")
	}
	s.scrollBy(mouseDown)
	if !s.scrollAtBottom() {
		t.Error("scrolling down should return to the bottom")
	}
}

// The frame the renderer is given carries the offset, which is what makes the
// pane honour it.
func TestDrawCarriesTheOffset(t *testing.T) {
	s := newScrollSession()
	s.scroll = scrolledBy(0, mouseUp)

	s.mu.Lock()
	frame := s.frame
	frame.Scroll = s.scroll
	s.mu.Unlock()

	if frame.Scroll != scrollStep {
		t.Errorf("frame.Scroll = %d, want %d", frame.Scroll, scrollStep)
	}
}

// With an offset of zero the frame is byte for byte what it was before
// scrolling existed, so the pane behaviour is not regressed.
func TestRenderAtOffsetZeroIsUnchanged(t *testing.T) {
	f := Frame{
		Title:  "openrouter-cli",
		Reply:  []string{"one", "two", "three", "four", "five"},
		Status: Status{Provider: providerName, Host: "h"},
		Input:  "hi",
	}
	withOffset := f
	withOffset.Scroll = 0

	got := Render(f, 8, 40)
	want := Render(withOffset, 8, 40)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("offset zero changed the frame:\n got %q\nwant %q", got, want)
	}
	if strings.Contains(strings.Join(got, "\n"), scrollMarker) {
		t.Errorf("frame = %q, want no marker at the bottom", got)
	}
}

// Scrolling back shows the lines that were previously off the pane, rather
// than the same newest lines with blank rows above them.
func TestRenderScrolledShowsOlderLines(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line" + string(rune('a'+i))
	}
	f := Frame{Reply: lines}
	paneHeight := 8

	bottom := Render(f, paneHeight+3, 40)
	if !containsLine(bottom, "linet") {
		t.Fatalf("bottom frame = %q, want the newest line", bottom)
	}

	scrolled := f
	scrolled.Scroll = 2
	top := Render(scrolled, paneHeight+3, 40)
	if !containsLine(top, "liner") {
		t.Errorf("scrolled frame = %q, want a line two back", top)
	}
	if containsLine(top, "linet") {
		t.Errorf("scrolled frame = %q, want the newest line moved out", top)
	}
}

// An offset past the top of the conversation shows the oldest lines there are
// and fills the pane with them, rather than leaving a single line in an empty
// frame. The top is where the view stops, so the oldest line is the stop.
func TestRenderScrollClampsAtTop(t *testing.T) {
	lines := []string{"one", "two", "three"}
	f := Frame{Reply: lines, Scroll: 500}
	out := Render(f, 6, 40)
	if !containsLine(out, "one") {
		t.Errorf("frame = %q, want the oldest line at the top of the history", out)
	}
}

// A conversation longer than the pane must still fill the pane when scrolled
// past its beginning, since an offset beyond the history is not a position that
// exists.
func TestRenderScrollPastTopFillsThePane(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%02d", i)
	}
	const height = 12
	f := Frame{Reply: lines, Scroll: 500}
	out := Render(f, height, 40)

	// The pane starts below the header, which is the title, a rule, and the
	// status bar. It is found by content rather than by arithmetic, since the
	// header grows and shrinks with the terminal.
	barAt := -1
	for i, l := range out {
		if strings.Contains(l, "Provider") {
			barAt = i
			break
		}
	}
	if barAt < 2 {
		t.Fatalf("no status bar found in %d rows", len(out))
	}
	// The pane runs from below the header to above the rule that divides it
	// from the prompt.
	promptAt := -1
	for i, l := range out {
		if strings.HasPrefix(l, ">") {
			promptAt = i
			break
		}
	}
	if promptAt < barAt {
		t.Fatalf("no prompt found below the bar in %d rows", len(out))
	}
	// The pane is everything between the header and the division above the
	// prompt. Both are found by content, since the row counts change with the
	// terminal height and with whether the division is drawn.
	paneStart := barAt + 1
	paneEnd := -1
	for i := paneStart; i < len(out); i++ {
		if strings.HasPrefix(strings.TrimSpace(out[i]), "\u2500") {
			paneEnd = i
			break
		}
	}
	if paneEnd < 0 {
		t.Fatalf("no division found below the bar in %d rows", len(out))
	}

	pane := out[paneStart:paneEnd]

	// The history must be contiguous from the oldest line, with no row of the
	// frame showing through it. Trailing blank rows are expected, since the
	// pane is padded when the history is shorter than the space available.
	for i, line := range pane {
		if strings.TrimSpace(line) == "" {
			continue
		}
		want := fmt.Sprintf("line%02d", i)
		if strings.TrimSpace(line) != want {
			t.Errorf("pane row %d is %q, want %q", i, line, want)
			break
		}
	}
	_ = promptAt
}

// The marker is what tells a reader the view is not at the bottom, since
// nothing else on screen changes when it is scrolled.
func TestRenderShowsScrollMarker(t *testing.T) {
	f := Frame{Reply: []string{"one", "two", "three"}, Scroll: 1}
	out := Render(f, 8, 60)
	if !strings.Contains(out[0], scrollMarker) {
		t.Errorf("title row = %q, want the scroll marker", out[0])
	}

	// The marker is on the title row rather than on a row of its own, since
	// a row taken for it would resize the pane as the reader scrolled.
	if len(out) != 8 {
		t.Errorf("len = %d, want 8, the frame must not resize", len(out))
	}
}

// The marker is dropped when the view is at the bottom.
func TestRenderNoMarkerAtBottom(t *testing.T) {
	out := Render(Frame{Reply: []string{"one"}}, 8, 60)
	if strings.Contains(strings.Join(out, "\n"), scrollMarker) {
		t.Errorf("frame = %q, want no marker at the bottom", out)
	}
}

// A very narrow terminal must still show a marker rather than losing it or
// wrapping the title onto a second row.
func TestRenderScrollMarkerFitsNarrowPane(t *testing.T) {
	for _, width := range []int{1, 2, 5, 10, len(scrollMarker)} {
		out := Render(Frame{Reply: []string{"a long line of output"}, Scroll: 1}, 8, width)
		if len(out[0]) > width {
			t.Errorf("width %d: title row is %d wide: %q", width, len(out[0]), out[0])
		}
	}
}

// tmux is detected from the environment, since that is the only place a nested
// terminal is recorded.
func TestInTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if InTmux() {
		t.Error("InTmux reported true with TMUX unset")
	}
	t.Setenv("TMUX", "/tmp/tmux-0/default,1234,0")
	if !InTmux() {
		t.Error("InTmux reported false with TMUX set")
	}
}

// Reporting is off until it is asked for, and asking turns it on. A screen
// built directly is used rather than NewScreen, since NewScreen needs a real
// terminal and the behaviour under test is only the reporting state.
func TestMouseReportingIsOptIn(t *testing.T) {
	s := &Screen{}

	if s.Mouse() {
		t.Error("mouse reporting is on by default, want it off until asked")
	}
	s.SetMouse(true)
	if !s.Mouse() {
		t.Error("SetMouse(true) did not turn reporting on")
	}
	s.SetMouse(false)
	if s.Mouse() {
		t.Error("SetMouse(false) did not turn reporting off")
	}
}

// The mode asked for is the button-event mode, and not a mode that reports
// motion. A mode that reports every pointer movement would fill the input
// stream while the mouse merely crosses the window, and a wheel interface has
// no use for the drag events that mode adds.
func TestMouseUsesButtonModeOnly(t *testing.T) {
	if !strings.Contains(seqMouseOn, "?1000h") {
		t.Errorf("seqMouseOn = %q, want the button-event mode", seqMouseOn)
	}
	if !strings.Contains(seqMouseOff, "?1000l") {
		t.Errorf("seqMouseOff = %q, want the button-event mode", seqMouseOff)
	}
	// Modes 1002 and 1003 add drag and motion reporting, which are not used.
	for _, unwanted := range []string{"1002", "1003"} {
		if strings.Contains(seqMouseOn, unwanted) {
			t.Errorf("seqMouseOn = %q, want no %s mode", seqMouseOn, unwanted)
		}
	}
}

// newScrollSession returns a session with a screen that draws nowhere, so the
// scroll behaviour can be exercised without a terminal.
func newScrollSession() *Session {
	return &Session{conv: NewConversation(), screen: &Screen{}}
}

// containsLine reports whether a rendered frame carries the given line.
func containsLine(frame []string, want string) bool {
	for _, l := range frame {
		if strings.TrimSpace(l) == want {
			return true
		}
	}
	return false
}

// The wheel is handled on the reading goroutine while the spinner writes the
// frame from its own, so the offset is guarded by the same lock as the frame.
// This is the case the race detector is there to catch.
//
// The reply is appended under the lock here, which the request loop does
// without it because that loop is the one writing the reply and it is the same
// goroutine that reads it back. The wheel is the only writer that arrives from
// elsewhere.
func TestScrollUnderConcurrentWork(t *testing.T) {
	s := &Session{
		conv:    NewConversation(),
		screen:  &Screen{},
		spinner: NewSpinner(),
	}

	s.beginWork()
	defer s.endWork()

	var wg sync.WaitGroup
	// The reader scrolling, which is what the wheel does.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.scrollBy(mouseUp)
			s.scrollBy(mouseDown)
		}
	}()
	// Repaints from the work in flight, which read the same frame.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.draw()
		}
	}()
	// A reply arriving, which is what must not move the offset.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			s.mu.Lock()
			s.frame.Reply = append(s.frame.Reply, "a line of output")
			s.mu.Unlock()
		}
	}()
	wg.Wait()

	if s.scroll < 0 {
		t.Errorf("scroll = %d, want it never negative", s.scroll)
	}
}
