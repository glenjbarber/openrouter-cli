package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// csi matches the sequences color adds, so that they can be stripped and the
// text compared.
var csi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// frameCorpus is a set of frames that between them reach every chrome row.
func frameCorpus() []Frame {
	long := strings.Repeat("a long reply line that folds ", 30)
	return []Frame{
		{Input: "hi"},
		{Title: "a session", Reply: []string{"one", "two"}, Input: "typing"},
		{Title: "t", Reply: []string{long}, Input: "x", Scroll: 3},
		{Title: "t", Hints: []string{"Esc stops", "Tab completes"}, Input: "x"},
		{Title: "t", Notice: "nothing matches", Input: "/zz"},
		{Title: "t", Confirm: "run go build ./... here?", ConfirmChoice: "yes", Input: ""},
		{Title: "t", Confirm: "run it?", Notice: "n", Hints: []string{"Esc"}, Pasted: []string{"p1", "p2"}, Queued: []string{"q"}},
		{Title: "t", Status: Status{Provider: "p", Model: "m"}, Partial: "streaming", Spinner: spinnerFrames[0], Elapsed: "3s"},
		{Title: "t", Reply: []string{"a\x1b[31mred\x1b[0m", "tab\there"}, Input: "x"},
		fullFrame(),
	}
}

// frameSizes are the sizes the corpus is drawn at, including ones too small to
// hold the whole frame.
var frameSizes = [][2]int{{24, 80}, {24, 40}, {12, 40}, {6, 30}, {3, 20}, {30, 8}, {1, 1}}

// oldFrame is the frame as it was drawn before color existed, written out as
// the bytes it comes to. It is what a color-off frame must still be.
func oldFrame(rows []string, height int) string {
	var b strings.Builder
	b.WriteString(seqHome)
	for i, row := range rows {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(seqResetAttr + seqClearLine + row)
	}
	for i := len(rows); i < height; i++ {
		b.WriteString("\r\n" + seqClearLine)
	}
	b.WriteString(seqHome)
	return b.String()
}

// Spans line up with the rows: one list for each row, every span inside its row
// and on a character boundary, and none in the prompt row.
func TestSpansLineUpWithTheRows(t *testing.T) {
	for fi, f := range frameCorpus() {
		for _, size := range frameSizes {
			height, width := size[0], size[1]
			rows, spans, _, _ := renderStyled(f, height, width)
			name := fmt.Sprintf("frame %d at %dx%d", fi, height, width)
			if len(spans) != len(rows) {
				t.Fatalf("%s: %d span lists for %d rows", name, len(spans), len(rows))
			}
			for i, list := range spans {
				pos := 0
				for _, sp := range list {
					if sp.start < pos || sp.start >= sp.end || sp.end > len(rows[i]) {
						t.Errorf("%s row %d: bad span %+v in a row of %d bytes", name, i, sp, len(rows[i]))
						continue
					}
					if sp.role < 0 || sp.role >= roleCount {
						t.Errorf("%s row %d: span with role %d", name, i, sp.role)
					}
					if !validBoundary(rows[i], sp.start) || !validBoundary(rows[i], sp.end) {
						t.Errorf("%s row %d: span %+v splits a character of %q", name, i, sp, rows[i])
					}
					pos = sp.end
				}
				if strings.HasPrefix(rows[i], promptMark) && len(list) != 0 {
					t.Errorf("%s row %d: the prompt row carries spans %+v", name, i, list)
				}
			}
		}
	}
}

// validBoundary reports whether an offset falls between two characters.
func validBoundary(s string, at int) bool {
	return at == 0 || at == len(s) || (s[at]&0xC0) != 0x80
}

// Render and render keep the text they always had, whatever the spans say.
func TestRenderStyledRowsEqualRenderRows(t *testing.T) {
	for fi, f := range frameCorpus() {
		for _, size := range frameSizes {
			want := Render(f, size[0], size[1])
			got, _, _, _ := renderStyled(f, size[0], size[1])
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("frame %d at %v: renderStyled rows differ from Render", fi, size)
			}
			for _, row := range got {
				if strings.Contains(row, "\x1b") {
					t.Errorf("frame %d at %v: an escape is in a row: %q", fi, size, row)
				}
			}
		}
	}
}

// A byte that plainRow drops moves the offsets after it, and the spans follow.
func TestSpansFollowARowPlainRowChanges(t *testing.T) {
	row := "ab\x1bcd\ref"
	got := remapSpans(row, []span{{start: 0, end: 2, role: roleChrome}, {start: 3, end: len(row), role: roleDim}})
	want := []span{{0, 2, roleChrome}, {2, 6, roleDim}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("remapSpans = %v, want %v", got, want)
	}
	if plain := plainRow(row); plain != "abcdef" || plain[2:6] != "cdef" {
		t.Errorf("plainRow = %q", plain)
	}
}

// A reply carrying an escape in a box question leaves the border span on the
// border after the escape is dropped.
func TestBoxSpansSurviveADroppedEscape(t *testing.T) {
	rows, spans, _, _ := renderStyled(Frame{Title: "t", Confirm: "run \x1b[31mgo\x1b[0m?", Input: "x"}, 24, 40)
	found := false
	for i, row := range rows {
		if !strings.HasPrefix(row, boxVertical) {
			continue
		}
		found = true
		if len(spans[i]) != 2 {
			t.Fatalf("a side row has %d spans: %v", len(spans[i]), spans[i])
		}
		if spans[i][1].end != len(row) || row[spans[i][1].start:] != boxVertical {
			t.Errorf("the right edge span is off the border: %v in %q", spans[i], row)
		}
	}
	if !found {
		t.Fatal("no side row in the frame")
	}
}

// With color off the bytes are the bytes of a frame with no color in it, for
// every frame of the corpus, with or without a box.
func TestColorOffDrawsTheOldBytes(t *testing.T) {
	for fi, f := range frameCorpus() {
		for _, size := range frameSizes {
			height, width := size[0], size[1]
			sc, read := screenCapture(t)
			sc.height, sc.width = height, width
			rows, spans, _, _ := renderStyled(f, height, width)
			sc.DrawFrame(rows, framePaint{box: rows, spans: spans, twiddle: -1})
			got := read()

			sc2, read2 := screenCapture(t)
			sc2.height, sc2.width = height, width
			sc2.DrawFrame(rows, framePaint{twiddle: -1})
			if got != read2() {
				t.Errorf("frame %d at %v: spans changed a color-off frame", fi, size)
			}
			for _, seq := range csi.FindAllString(got, -1) {
				if seq != seqResetAttr {
					t.Errorf("frame %d at %v: color-off output carries %q", fi, size, seq)
				}
			}
			// The rows and the clears, with only the caret placement left out.
			if want := oldFrame(rows, height); !strings.HasPrefix(got, want) {
				t.Errorf("frame %d at %v: not the old bytes\n got %q\nwant %q...", fi, size, got, want)
			}
		}
	}
}

// With color on, the sequences are around chrome rows only, and the text with
// them stripped is the color-off text.
func TestColorOnColorsChromeRowsOnly(t *testing.T) {
	pal := newPalette(config.Theme{})
	for fi, f := range frameCorpus() {
		// The twiddle frame is left to its own path, which has its own tests.
		if f.Spinner != "" {
			continue
		}
		height, width := 24, 60
		rows, spans, _, _ := renderStyled(f, height, width)

		sc, read := screenCapture(t)
		sc.height, sc.width = height, width
		sc.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
		got := read()

		sc2, read2 := screenCapture(t)
		sc2.height, sc2.width = height, width
		sc2.DrawFrame(rows, framePaint{twiddle: -1})
		off := read2()

		if stripped := csi.ReplaceAllString(got, ""); stripped != csi.ReplaceAllString(off, "") {
			t.Errorf("frame %d: the text with color stripped is not the color-off text\n got %q\nwant %q", fi, stripped, off)
		}

		// Each row of the output, with the row start removed, carries a role
		// sequence only where the frame has a span for that row.
		body := strings.TrimPrefix(got, seqHome)
		lines := strings.Split(body, "\r\n")
		for i, row := range rows {
			if i >= len(lines) {
				t.Fatalf("frame %d: row %d is missing from the output", fi, i)
			}
			// The last row is followed by the home and the caret placement.
			line, _, _ := strings.Cut(lines[i], seqHome)
			rest := strings.TrimPrefix(line, pal.reset()+seqClearLine)
			colored := rest != row
			if colored != (len(spans[i]) > 0) {
				t.Errorf("frame %d row %d: colored = %v with %d spans: %q", fi, i, colored, len(spans[i]), lines[i])
			}
			if strings.HasPrefix(row, promptMark) && colored {
				t.Errorf("frame %d: the prompt row is colored: %q", fi, lines[i])
			}
		}
	}
}

// The chrome rows get their roles: the rules and the status row the chrome
// color, the title its own, the hint dim and the notice its own.
func TestChromeRowsTakeTheirRoles(t *testing.T) {
	f := Frame{Title: "t", Hints: []string{"Esc stops"}, Notice: "nothing matches", Input: "x"}
	rows, spans, _, _ := renderStyled(f, 24, 60)
	seen := map[role]bool{}
	for i := range rows {
		for _, sp := range spans[i] {
			seen[sp.role] = true
		}
	}
	for _, r := range []role{roleChrome, roleTitle, roleDim, roleNotice} {
		if !seen[r] {
			t.Errorf("no row carries role %d", r)
		}
	}
	if seen[roleApproval] {
		t.Error("a frame with no question carries the approval role")
	}
}

// With a base set, every reset re-applies it before the row is cleared, so the
// fill takes the background, and so do the rows past the frame.
func TestTheBaseIsReappliedAfterEveryReset(t *testing.T) {
	pal := newPalette(config.Theme{Foreground: "white", Background: "#101010"})
	base := pal.base()
	if base == "" {
		t.Fatal("the theme produced no base")
	}
	sc, read := screenCapture(t)
	sc.height, sc.width = 10, 40
	rows, spans, _, _ := renderStyled(Frame{Title: "t", Input: "x"}, 6, 40)
	sc.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
	got := read()

	if n := strings.Count(got, sgrReset+seqClearLine); n != 0 {
		t.Errorf("%d clears follow a bare reset", n)
	}
	if n := strings.Count(got, sgrReset+base+seqClearLine); n != len(rows) {
		t.Errorf("%d clears follow a reset with the base, want %d", n, len(rows))
	}
	if n := strings.Count(got, base+seqClearLine); n != len(rows)+4 {
		t.Errorf("%d clears carry the base, want %d with the 4 rows below the frame", n, len(rows)+4)
	}
	// Every reset that ends a span carries the base too.
	for _, part := range strings.Split(got, sgrReset)[1:] {
		if !strings.HasPrefix(part, base) {
			t.Errorf("a reset is not followed by the base: %q", part[:min(len(part), 20)])
		}
	}
}

// With no theme there is no base, and no sequence of one is written.
func TestNoThemeWritesNoBaseInTheFrame(t *testing.T) {
	pal := newPalette(config.Theme{})
	sc, read := screenCapture(t)
	sc.height, sc.width = 10, 40
	rows, spans, _, _ := renderStyled(Frame{Title: "t", Input: "x"}, 6, 40)
	sc.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
	if strings.Contains(read(), "\x1b[38;5;") || strings.Contains(read(), "\x1b[48;5;") {
		t.Error("a base was written with no theme")
	}
	if sc.base {
		t.Error("the screen believes a base is set")
	}
}

// The base is cleared on the way out, so the shell is not drawn on the theme
// background. Closing a screen that never set one writes nothing extra.
func TestRestoreClearsTheBase(t *testing.T) {
	sc, read := screenCapture(t)
	sc.base = true
	sc.restore()
	if want := seqPasteOff + seqResetAttr + seqShowCur + seqExitAlt; read() != want {
		t.Errorf("restore wrote %q, want %q", read(), want)
	}

	plain, readPlain := screenCapture(t)
	plain.restore()
	if want := seqPasteOff + seqShowCur + seqExitAlt; readPlain() != want {
		t.Errorf("restore with no base wrote %q, want %q", readPlain(), want)
	}
}

// The twiddle row is drawn by its own path even when color is on.
func TestTheTwiddleWinsItsRowWithColorOn(t *testing.T) {
	pal := newPalette(config.Theme{})
	f := Frame{Title: "t", Partial: "a", Spinner: spinnerFrames[0], Elapsed: "1s", Input: "x"}
	rows, spans, _, twiddle := renderStyled(f, 24, 60)
	if twiddle < 0 {
		t.Fatal("no twiddle row")
	}
	sc, read := screenCapture(t)
	sc.height, sc.width = 24, 60
	tintSeq := twiddleTint(2)
	sc.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: twiddle, sequence: tintSeq, figure: spinnerFrames[0]})
	if !strings.Contains(read(), tintSeq+rows[twiddle]+seqResetAttr) {
		t.Errorf("the twiddle row is not drawn by the twiddle path:\n%q", read())
	}
}
