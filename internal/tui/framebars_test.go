package tui

import (
	"strings"
	"testing"
)

// The frame is divided by rules at the top, under the title, under the status
// bar, above the prompt, and at the very bottom. These check the shape, since
// it is what a reader reads the window by.

func isRuleRow(row string) bool {
	return row != "" && strings.Trim(row, ruleRune) == ""
}

func TestTheFrameIsDividedAtTheTopAndTheBottom(t *testing.T) {
	rows := Render(Frame{
		Title:   "openrouter-cli",
		Status:  Status{Provider: "openrouter.ai"},
		Reply:   []string{"a reply"},
		Partial: "partial",
	}, 24, 50)

	if !isRuleRow(rows[0]) {
		t.Errorf("the first row is %q, want a rule", rows[0])
	}
	// The rule closing the frame is the last row with anything on it. The rows
	// below it are the blank the caret sits on, since a caret drawn against a
	// rule is a caret on a border.
	closing := -1
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			closing = i
			break
		}
	}
	if !isRuleRow(rows[closing]) {
		t.Errorf("the last row carrying anything is %q, want a rule", rows[closing])
	}
	for _, row := range rows[closing+1:] {
		if strings.TrimSpace(row) != "" {
			t.Errorf("the frame has content below its closing rule: %q", row)
		}
	}
}

func TestTheTitleSitsDirectlyUnderTheTopRule(t *testing.T) {
	rows := Render(Frame{Title: "openrouter-cli"}, 24, 50)

	// No blank between them, so the rule reads as the edge of the frame
	// rather than as an underline of the title.
	if !strings.Contains(rows[1], "openrouter-cli") {
		t.Errorf("row 1 is %q, want the title", rows[1])
	}
}

func TestTheStatusBarIsBetweenTwoRulesAndABlank(t *testing.T) {
	rows := Render(Frame{
		Title:  "openrouter-cli",
		Status: Status{Provider: "openrouter.ai"},
	}, 24, 50)

	barAt := -1
	for i, row := range rows {
		if strings.Contains(row, "Provider") {
			barAt = i
			break
		}
	}
	if barAt < 0 {
		t.Fatalf("no status bar in the frame:\\n%s", strings.Join(rows, "\\n"))
	}
	if !isRuleRow(rows[barAt-2]) {
		t.Errorf("the row two above the bar is %q, want a rule", rows[barAt-2])
	}
	if strings.TrimSpace(rows[barAt-1]) != "" {
		t.Errorf("the row above the bar is %q, want a blank", rows[barAt-1])
	}
	// The rule closing the header, then the blank, then the pane.
	// The rule closing the header, with a blank on either side of it.
	if strings.TrimSpace(rows[barAt+1]) != "" {
		t.Errorf("the row below the bar is %q, want a blank", rows[barAt+1])
	}
	if !isRuleRow(rows[barAt+2]) {
		t.Errorf("the row two below the bar is %q, want the rule closing the header",
			rows[barAt+2])
	}
}

func TestTheCaretRowIsAboveTheBlankAndTheBottomRule(t *testing.T) {
	rows := Render(Frame{Input: "hello"}, 24, 50)

	at := promptRow(rows)
	if at < 0 {
		t.Fatalf("no prompt in the frame:\\n%s", strings.Join(rows, "\\n"))
	}
	// A blank below the prompt, then the rule closing the frame, so the caret
	// is not drawn against a rule.
	if strings.TrimSpace(rows[at+1]) != "" {
		t.Errorf("the row below the prompt is %q, want a blank", rows[at+1])
	}
	if !isRuleRow(rows[at+2]) {
		t.Errorf("the row two below the prompt is %q, want a rule", rows[at+2])
	}
}

func TestTheFrameFillsTheTerminalAtEveryHeight(t *testing.T) {
	// A frame shorter than the terminal would leave whatever the terminal had
	// drawn there, which after a resize is not blank.
	for height := 1; height <= 40; height++ {
		rows := Render(Frame{Title: "t", Status: Status{Provider: "p"}, Reply: []string{"r"}},
			height, 40)
		if len(rows) != height {
			t.Errorf("height=%d: the frame is %d rows", height, len(rows))
		}
	}
}

// The header is nine rows, so a short terminal gives them up from the top. The
// rules go first, then the blanks, then the title, and the status bar is held.
func TestTheHeaderGivesUpRulesBeforeText(t *testing.T) {
	for height := 4; height <= 14; height++ {
		rows := Render(Frame{
			Title:  "TITLEROW",
			Status: Status{Provider: "PROVIDER"},
			Input:  "hi",
		}, height, 40)
		joined := strings.Join(rows, "\n")

		hasTitle := strings.Contains(joined, "TITLEROW")
		hasBar := strings.Contains(joined, "PROVIDER")

		if hasTitle && !hasBar {
			t.Errorf("height=%d: the title was kept and the status bar dropped:\\n%s",
				height, joined)
		}
	}
}

// A rule the header keeps while the figures are dropped divides nothing, which
// is the one header row worth refusing.
func TestTheHeaderDropsItsLastRuleWithTheStatusBar(t *testing.T) {
	for height := 4; height <= 12; height++ {
		rows := Render(Frame{
			Title:  "TITLEROW",
			Status: Status{Provider: "PROVIDER"},
			Input:  "hi",
		}, height, 40)
		if strings.Contains(strings.Join(rows, "\n"), "PROVIDER") {
			continue
		}
		// The frame is a prompt, a blank and the closing rule, with nothing
		// above the prompt that belongs to the header.
		at := promptRow(rows)
		if at <= 0 {
			continue
		}
		for _, row := range rows[:at] {
			if isRuleRow(row) {
				t.Errorf("height=%d: a header rule survived with no status bar:\\n%s",
					height, strings.Join(rows, "\n"))
				break
			}
		}
	}
}
