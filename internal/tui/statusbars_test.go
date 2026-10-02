package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// The status is split in two. What changes on the fly, which is the provider,
// the model, the state and the approval, is in a bar above the input box. The
// figures that move slowly are in the bar at the top. These tests find each bar
// by what it says and never by the row it is on, since the header yields and
// the division comes and goes with the height of the terminal.

// fullFrame is a frame with every field of both bars filled in.
func fullFrame() Frame {
	return Frame{
		Title: "openrouter-cli",
		Status: Status{
			Provider:  "openrouter.ai",
			Model:     "test/model",
			State:     "idle",
			Approval:  "ask",
			Credits:   "0.42/5",
			Cost:      "$0.0123",
			Context:   "12%",
			TokensIn:  "1.2k",
			TokensOut: "5.7k",
			Host:      "hostname.example",
		},
		Reply: []string{"> a question", "", "an answer"},
		Input: "typing",
	}
}

// rowWith returns the index of the one row carrying text, or fails when there is
// not exactly one.
func rowWith(t *testing.T, rows []string, text string) int {
	t.Helper()
	at := -1
	for i, row := range rows {
		if strings.Contains(row, text) {
			if at >= 0 {
				t.Fatalf("more than one row carries %q:\n%s", text, strings.Join(rows, "\n"))
			}
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no row carries %q:\n%s", text, strings.Join(rows, "\n"))
	}
	return at
}

// marked is a frame whose pane is full of rows that can be counted.
func marked(n int) Frame {
	f := fullFrame()
	f.Reply = nil
	for i := 0; i < n; i++ {
		f.Reply = append(f.Reply, fmt.Sprintf("pane%03d", i))
	}
	return f
}

// paneRowCount is how many rows of a frame hold the marked pane.
func paneRowCount(rows []string) int {
	n := 0
	for _, row := range rows {
		if strings.HasPrefix(row, "pane") {
			n++
		}
	}
	return n
}

// The rows of the frame, from the top, are the header, the pane, a blank, a
// rule, a blank, the bar, a blank, a rule, a blank, and the input box. Each rule
// has a blank on both sides of it, so neither rule touches the pane or the
// prompt.
func TestTheFrameHasTheSplitRowSequence(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		for _, height := range []int{24, 30, 40, 60} {
			name := fmt.Sprintf("%dx%d", height, width)
			rows := Render(fullFrame(), height, width)

			top := rowWith(t, rows, "Context:")
			bar := rowWith(t, rows, "Provider:")
			prompt := promptRow(rows)

			// The header: a rule, the title, a blank, a rule, a blank, the
			// bar, a blank, a rule, and a blank.
			if !isRuleRow(rows[0]) || !strings.Contains(rows[1], "openrouter-cli") ||
				rows[2] != "" || !isRuleRow(rows[3]) || rows[4] != "" || top != 5 ||
				rows[6] != "" || !isRuleRow(rows[7]) || rows[8] != "" {
				t.Errorf("%s: the header is not rule, title, blank, rule, blank, bar, blank, rule, blank:\n%s",
					name, strings.Join(rows[:10], "\n"))
			}

			// The division, found from the bar.
			if !isRuleRow(rows[bar-2]) {
				t.Errorf("%s: the row two above the bar is %q, want a rule", name, rows[bar-2])
			}
			if rows[bar-1] != "" || rows[bar+1] != "" {
				t.Errorf("%s: the bar is not between two blanks: %q and %q", name, rows[bar-1], rows[bar+1])
			}
			if !isRuleRow(rows[bar+2]) {
				t.Errorf("%s: the row two below the bar is %q, want a rule", name, rows[bar+2])
			}
			if rows[bar-3] != "" || rows[bar+3] != "" {
				t.Errorf("%s: a rule touches the pane or the prompt: %q and %q", name, rows[bar-3], rows[bar+3])
			}
			if prompt != bar+4 {
				t.Errorf("%s: the prompt is on row %d, want it one blank under the rule at row %d",
					name, prompt, bar+2)
			}
			// The pane runs from under the header to the blank above the first
			// rule of the division.
			if !strings.Contains(strings.Join(rows[top+4:bar-3], "\n"), "an answer") {
				t.Errorf("%s: the pane does not hold the reply:\n%s", name, strings.Join(rows, "\n"))
			}

			// The foot: a blank, the closing rule, and the row held beneath
			// it, which is blank.
			if rows[prompt+1] != "" || !isRuleRow(rows[prompt+2]) {
				t.Errorf("%s: the foot is not a blank and a rule", name)
			}
			for _, row := range rows[prompt+3:] {
				if row != "" {
					t.Errorf("%s: %q is below the closing rule", name, row)
				}
			}
			if len(rows) != height {
				t.Errorf("%s: %d rows", name, len(rows))
			}
		}
	}
}

// The frame costs twenty rows with nothing above the prompt: nine for the
// header, seven for the division, one for the prompt, two for the foot, and the
// one row the budget holds beneath the foot. The pane is given what is left.
func TestTheFrameCostsTwentyRows(t *testing.T) {
	const fixed = 9 + 7 + 1 + 2 + 1
	if fixed != 20 {
		t.Fatalf("the rows apart from the pane add up to %d, want 20", fixed)
	}
	for _, height := range []int{21, 22, 24, 30, 40, 60} {
		for _, width := range []int{20, 80} {
			rows := Render(marked(200), height, width)
			if got := paneRowCount(rows); got != height-fixed {
				t.Errorf("%dx%d: the pane holds %d rows, want %d", height, width, got, height-fixed)
			}
		}
	}
	if got := paneRows(40); got != 40-fixed {
		t.Errorf("paneRows(40) = %d, want %d", got, 40-fixed)
	}
}

// The pane the search and the pager work to is the pane the renderer draws, at
// every height. They had each kept a count of their own, and the counts had
// drifted.
func TestThePaneRowsAreWhatTheRendererDraws(t *testing.T) {
	for height := 2; height <= 70; height++ {
		rows := Render(marked(300), height, 60)
		if got, want := paneRowCount(rows), paneRows(height); got != want {
			t.Errorf("height %d: the renderer draws %d pane rows, paneRows says %d", height, got, want)
		}
	}
}

// A page is a little under the pane that is on screen, so that the row a reader
// was on is still in sight afterwards.
func TestAPageIsOneRowUnderThePaneThatIsDrawn(t *testing.T) {
	for _, height := range []int{12, 24, 40} {
		s := pagedSession(t, height)
		s.mu.Lock()
		frame := s.frame
		s.mu.Unlock()
		drawn := 0
		for _, row := range Render(frame, height, 80) {
			if row == "a line of history" {
				drawn++
			}
		}
		s.page(-1)
		// A pane of one row still pages by one, since a page that moved
		// nothing would not be a page.
		if got, want := s.pagingOffset(), max(1, drawn-1); got != want {
			t.Errorf("height %d: a page moved %d rows, want %d with %d drawn", height, got, want, drawn)
		}
	}
}

// Scrolled to the top, the pane is filled with the oldest lines, whatever the
// height, so the clamp agrees with the pane the frame now has.
func TestScrollingToTheTopFillsThePaneAtEveryHeight(t *testing.T) {
	for _, height := range []int{12, 20, 30, 50} {
		f := marked(300)
		f.Scroll = 1 << 20
		rows := Render(f, height, 60)
		if want := paneRows(height); paneRowCount(rows) != want {
			t.Errorf("height %d: %d pane rows, want the pane of %d full", height, paneRowCount(rows), want)
		}
		if first := rowWith(t, rows, "pane000"); first < 0 {
			t.Errorf("height %d: the oldest line is not shown", height)
		}
	}
}

// Every rule in a full frame has a blank row on both sides of it. The two
// exceptions are the rule on the first row, which has no row above it and sits
// against the title, and the closing rule, which has the held blank row below it
// or the end of the frame.
func TestEveryRuleHasABlankOnBothSides(t *testing.T) {
	for _, width := range []int{20, 40, 76, 120} {
		for _, height := range []int{21, 22, 24, 30, 60} {
			rows := Render(fullFrame(), height, width)
			name := fmt.Sprintf("%dx%d", height, width)
			rules := 0
			for i, row := range rows {
				if !isRuleRow(row) {
					continue
				}
				rules++
				if i == 0 {
					continue
				}
				if rows[i-1] != "" {
					t.Errorf("%s: the row above the rule at %d is %q, want a blank", name, i, rows[i-1])
				}
				if i+1 < len(rows) && rows[i+1] != "" {
					t.Errorf("%s: the row below the rule at %d is %q, want a blank", name, i, rows[i+1])
				}
			}
			// The first row, the one under the title, the one under the top bar,
			// the two around the bar, and the closing rule.
			if rules != 6 {
				t.Errorf("%s: %d rules, want 6:\n%s", name, rules, strings.Join(rows, "\n"))
			}
		}
	}
}

// At every height that holds the whole division, the rules around the bar have
// a blank on both sides, whatever the header has had to give up. Below that the
// rules are not drawn against the bar at all.
func TestTheRulesAroundTheBarAreNeverDrawnWithoutBlanks(t *testing.T) {
	for height := minHeightForBar; height <= 40; height++ {
		rows := Render(fullFrame(), height, 80)
		bar := rowWith(t, rows, "Provider:")
		if height < minHeightForDivision {
			if isRuleRow(rows[bar-1]) || (bar+1 < len(rows) && isRuleRow(rows[bar+1])) {
				t.Errorf("height %d: a rule is drawn against the bar alone", height)
			}
			continue
		}
		for _, at := range []int{bar - 2, bar + 2} {
			if !isRuleRow(rows[at]) || rows[at-1] != "" || rows[at+1] != "" {
				t.Errorf("height %d: the rule at row %d has no blank on both sides:\n%s",
					height, at, strings.Join(rows, "\n"))
			}
		}
	}
}

// The values that change on the fly are in the bar above the input box and not
// in the top bar, and the top bar keeps the rest.
func TestTheMovedFieldsAreInTheInputBarOnly(t *testing.T) {
	rows := Render(fullFrame(), 30, 200)
	top := rows[rowWith(t, rows, "Context:")]
	input := rows[rowWith(t, rows, "Provider:")]

	moved := []string{"Provider: openrouter.ai", "Model: test/model", "Status: idle", "Approval: ask"}
	kept := []string{"Credits: 0.42/5", "Cost: $0.0123", "Context: 12%", "In: 1.2k", "Out: 5.7k", "hostname.example"}
	for _, want := range moved {
		if !strings.Contains(input, want) {
			t.Errorf("the bar above the input box = %q, want %q", input, want)
		}
		if label := strings.SplitN(want, ":", 2)[0] + ":"; strings.Contains(top, label) {
			t.Errorf("the top bar = %q, still carries %s", top, label)
		}
	}
	for _, want := range kept {
		if !strings.Contains(top, want) {
			t.Errorf("the top bar = %q, want %q", top, want)
		}
		if strings.Contains(input, want) {
			t.Errorf("the bar above the input box = %q, carries %q", input, want)
		}
	}
}

// A bar is found by content, so the same two bars are found at every height
// from the shortest that holds both, and the top bar is above the pane and the
// other below it.
func TestEachBarIsFoundByContentAtEveryHeight(t *testing.T) {
	for height := minHeightForBar; height <= 60; height++ {
		rows := Render(marked(100), height, 100)
		top := rowWith(t, rows, "Context:")
		bar := rowWith(t, rows, "Provider:")
		for i, row := range rows {
			if strings.HasPrefix(row, "pane") && (i < top || i > bar) {
				t.Errorf("height %d: pane row %d is outside the bars at %d and %d", height, i, top, bar)
			}
		}
		if bar < top {
			t.Errorf("height %d: the bar above the input box (%d) is above the top bar (%d)", height, bar, top)
		}
	}
}

// On a short terminal the division gives up its rules and its blanks and keeps
// the bar, and below that the bar goes too. The prompt is kept throughout and
// the frame is never taller than the terminal.
func TestTheDivisionGivesWayOnAShortTerminal(t *testing.T) {
	if minHeightForDivision != 12 || minHeightForBar != 6 {
		t.Fatalf("the thresholds are %d and %d, want 12 and 6", minHeightForDivision, minHeightForBar)
	}
	for height := 1; height <= 26; height++ {
		rows := Render(fullFrame(), height, 80)
		name := fmt.Sprintf("height %d", height)
		if len(rows) != height {
			t.Errorf("%s: %d rows", name, len(rows))
		}
		prompt := promptRow(rows)
		if prompt < 0 {
			t.Errorf("%s: no prompt:\n%s", name, strings.Join(rows, "\n"))
			continue
		}
		count := 0
		for _, row := range rows {
			if strings.Contains(row, "Provider:") {
				count++
			}
		}
		switch {
		case height >= minHeightForDivision:
			if count != 1 || rows[prompt-1] != "" || !isRuleRow(rows[prompt-2]) || rows[prompt-3] != "" ||
				!strings.Contains(rows[prompt-4], "Provider:") || rows[prompt-5] != "" ||
				!isRuleRow(rows[prompt-6]) || rows[prompt-7] != "" {
				t.Errorf("%s: the whole division is not above the prompt:\n%s", name, strings.Join(rows, "\n"))
			}
		case height >= minHeightForBar:
			if count != 1 || !strings.Contains(rows[prompt-1], "Provider:") {
				t.Errorf("%s: the bar alone is not directly above the prompt:\n%s", name, strings.Join(rows, "\n"))
			}
			// No rule is drawn without its blanks, so none borders the bar.
			if at := prompt - 1; isRuleRow(rows[at-1]) {
				t.Errorf("%s: a rule is drawn against the bar alone:\n%s", name, strings.Join(rows, "\n"))
			}
		default:
			if count != 0 {
				t.Errorf("%s: the bar is drawn on a terminal too short for it:\n%s", name, strings.Join(rows, "\n"))
			}
		}
		// The top bar is the last of the header to go, so it is there on any
		// terminal with a row for it.
		if height >= 5 && !strings.Contains(strings.Join(rows, "\n"), "Context:") {
			t.Errorf("%s: no top bar:\n%s", name, strings.Join(rows, "\n"))
		}
	}
}

// The hint row, a notice and a paste are part of the input box, so they sit
// between the rule that closes the bar and the prompt, and the bar and its
// rules stay where they were.
func TestTheInputBlockSitsUnderTheDivision(t *testing.T) {
	f := fullFrame()
	f.Hints = []string{"Esc stops"}
	f.Notice = "nothing matches"
	f.Pasted = []string{"pasted one", "pasted two"}
	rows := Render(f, 40, 80)

	bar := rowWith(t, rows, "Provider:")
	if !isRuleRow(rows[bar+2]) || rows[bar+3] != "" {
		t.Fatalf("no rule and blank close the bar:\n%s", strings.Join(rows, "\n"))
	}
	want := []string{"  pasted one", "  pasted two", "nothing matches", "Esc stops", "> typing"}
	for i, w := range want {
		if !strings.HasPrefix(rows[bar+4+i], w) {
			t.Errorf("row %d of the input box = %q, want %q", i, rows[bar+4+i], w)
		}
	}
}

// Neither bar is ever wider than the terminal, and the frame is never taller
// than it, at any size, with every field filled in.
func TestTheBarsFitEverySize(t *testing.T) {
	for width := 1; width <= 100; width++ {
		for _, height := range []int{5, 8, 10, 12, 24} {
			rows := Render(fullFrame(), height, width)
			if len(rows) > height {
				t.Fatalf("%dx%d: %d rows", height, width, len(rows))
			}
			for i, row := range rows {
				if n := displayWidth(row); n > width {
					t.Fatalf("%dx%d: row %d is %d columns: %q", height, width, i, n, row)
				}
			}
		}
	}
}

// The delegate pane and the worker pane are drawn with the same frame, so both
// bars stay on them and the pane is given what is left.
func TestTheBarsStayWhileAPaneIsShown(t *testing.T) {
	for _, title := range []string{delegateTitle, workerTitle} {
		f := marked(100)
		f.Title = title
		f.Delegate = "an answer that is still arriving"
		rows := Render(f, 30, 80)
		rowWith(t, rows, "Context:")
		rowWith(t, rows, "Provider:")
		if !strings.Contains(strings.Join(rows, "\n"), title) {
			t.Errorf("%s: the title is not drawn", title)
		}
		if !strings.Contains(strings.Join(rows, "\n"), "still arriving") {
			t.Errorf("%s: the partial answer is not drawn", title)
		}
	}
}

// dropOrderOf returns the labels of a bar in the order they disappear as the
// width shrinks, one for each label that does disappear.
func dropOrderOf(line func(width int) string, labels []string) []string {
	gone := map[string]int{}
	for width := 300; width >= 1; width-- {
		got := line(width)
		for _, label := range labels {
			if _, seen := gone[label]; !seen && !strings.Contains(got, label) {
				gone[label] = width
			}
		}
	}
	var order []string
	for len(order) < len(gone) {
		best, bestWidth := "", -1
		for _, label := range labels {
			w, ok := gone[label]
			if ok && w > bestWidth && !contains(order, label) {
				best, bestWidth = label, w
			}
		}
		order = append(order, best)
	}
	return order
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// A narrow top bar gives up the host, then the token counters, then the
// allowance, then the cost, and the context share is the last figure left and
// is cut rather than dropped.
func TestTheTopBarDegradesInOrder(t *testing.T) {
	st := fullFrame().Status
	order := dropOrderOf(func(w int) string { return TopLine(st, w) },
		[]string{"hostname.example", "In: ", "Out: ", "Credits: ", "Cost: ", "Context: "})
	want := []string{"hostname.example", "In: ", "Out: ", "Credits: ", "Cost: ", "Context: "}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("the top bar gives up %q, want %q", order, want)
	}
	if got := TopLine(st, 12); got != "Context: 12%" {
		t.Errorf("TopLine at 12 columns = %q, want the context share alone", got)
	}
	if got := TopLine(st, 8); displayWidth(got) > 8 || got == "" {
		t.Errorf("TopLine at 8 columns = %q, want the context share cut to fit", got)
	}
}

// A narrow bar above the input box gives up the approval, then the state, then
// the model, and the provider is the last field left and is cut rather than
// dropped.
func TestTheInputBarDegradesInOrder(t *testing.T) {
	st := fullFrame().Status
	order := dropOrderOf(func(w int) string { return InputLine(st, w) },
		[]string{"Approval: ", "Status: ", "Model: ", "Provider: "})
	want := []string{"Approval: ", "Status: ", "Model: ", "Provider: "}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Errorf("the bar above the input box gives up %q, want %q", order, want)
	}
	if got := InputLine(st, 24); got != "Provider: openrouter.ai" {
		t.Errorf("InputLine at 24 columns = %q, want the provider alone", got)
	}
	if got := InputLine(st, 8); displayWidth(got) > 8 || got == "" {
		t.Errorf("InputLine at 8 columns = %q, want the provider cut to fit", got)
	}
}

// With color on, the new bar and the rules around it take the roles the top bar
// and its rules take, one span over the whole row, and no role is added for it.
func TestTheNewBarIsStyledLikeTheTopBar(t *testing.T) {
	rows, spans, _, _ := renderStyled(fullFrame(), 30, 100)
	top := rowWith(t, rows, "Context:")
	bar := rowWith(t, rows, "Provider:")

	whole := func(i int) span { return span{start: 0, end: len(rows[i]), role: roleChrome} }
	for name, at := range map[string]int{"the top bar": top, "the new bar": bar} {
		if len(spans[at]) != 1 || spans[at][0] != whole(at) {
			t.Errorf("%s has spans %+v, want the chrome role over the whole row", name, spans[at])
		}
	}
	// Every rule is chrome, and every blank row has no span.
	for i, row := range rows {
		switch {
		case isRuleRow(row):
			if len(spans[i]) != 1 || spans[i][0] != whole(i) {
				t.Errorf("rule row %d has spans %+v, want the chrome role", i, spans[i])
			}
		case row == "":
			if len(spans[i]) != 0 {
				t.Errorf("blank row %d has spans %+v", i, spans[i])
			}
		}
	}
	// The rules either side of the new bar are the same rows the top bar has.
	if !isRuleRow(rows[bar-2]) || !isRuleRow(rows[bar+2]) {
		t.Fatalf("no rules around the new bar:\n%s", strings.Join(rows, "\n"))
	}
}

// With color on the new bar is drawn in the chrome color, and with color off
// the bytes are those of a frame that has no color in it.
func TestTheNewBarIsColoredOnlyWhenColorIsOn(t *testing.T) {
	const height, width = 30, 100
	rows, spans, _, _ := renderStyled(fullFrame(), height, width)
	bar := rows[rowWith(t, rows, "Provider:")]
	pal := newPalette(config.Theme{})

	on, readOn := screenCapture(t)
	on.height, on.width = height, width
	on.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
	if want := pal.role(roleChrome) + bar + pal.reset(); !strings.Contains(readOn(), want) {
		t.Errorf("the new bar is not in the chrome color with color on:\n%q", readOn())
	}

	off, readOff := screenCapture(t)
	off.height, off.width = height, width
	off.DrawFrame(rows, framePaint{spans: spans, twiddle: -1})
	got := readOff()
	if !strings.Contains(got, seqClearLine+bar) {
		t.Errorf("the new bar is not drawn as plain text with color off:\n%q", got)
	}
	for _, seq := range csi.FindAllString(got, -1) {
		if seq != seqResetAttr {
			t.Errorf("color off wrote the sequence %q", seq)
		}
	}
	for _, row := range rows {
		if strings.Contains(row, "\x1b") {
			t.Errorf("an escape is stored in a row: %q", row)
		}
	}
}
