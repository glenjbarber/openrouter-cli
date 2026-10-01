package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderFrameShape(t *testing.T) {
	f := Frame{
		Title:  "openrouter-cli",
		Reply:  []string{"hello"},
		Status: Status{Host: "host.example"},
		Input:  "hi",
	}
	lines := Render(f, 12, 60)
	if len(lines) != 12 {
		t.Errorf("len(lines) = %d, want 12", len(lines))
	}
	if !strings.HasPrefix(lines[len(lines)-1], "> ") {
		t.Errorf("last line = %q, want the input prompt", lines[len(lines)-1])
	}

	// The status bar is found by content rather than by position, since the
	// division now sits between it and the prompt.
	var bar string
	for _, l := range lines {
		if strings.Contains(l, "Provider") {
			bar = l
		}
	}
	if bar == "" {
		t.Error("no status bar in the frame")
	}
}

// A field that is not yet known is shown as a dash rather than hidden, so the
// layout does not shift as values arrive.
func TestStatusLinePlaceholder(t *testing.T) {
	line := StatusLine(Status{Host: "h"}, 200)
	if !strings.Contains(line, "Provider: -") {
		t.Errorf("line = %q, want a placeholder", line)
	}
	if !strings.HasSuffix(line, "h") {
		t.Errorf("line = %q, want the host at the end", line)
	}
}

func TestStatusLineFitsWidth(t *testing.T) {
	line := StatusLine(Status{Host: "h"}, 30)
	if len(line) > 30 {
		t.Errorf("len = %d, want at most 30: %q", len(line), line)
	}
}

// The host is dropped before the fields are, since it is the least useful when
// the bar is narrow.
func TestStatusLineDropsHostWhenNarrow(t *testing.T) {
	line := StatusLine(Status{Host: "host.example"}, 30)
	if strings.Contains(line, "host.example") {
		t.Errorf("line = %q, want the host dropped", line)
	}
}

func TestStatusLineOrderIsFixed(t *testing.T) {
	line := StatusLine(Status{}, 400)
	// Branch, Reasoning, and Approval were removed: they have no source in
	// this client, so a dash would be permanent rather than temporary.
	order := []string{"Provider", "Model", "Status", "Credits", "In:", "Out:"}
	at := -1
	for _, label := range order {
		i := strings.Index(line, label)
		if i < 0 {
			t.Fatalf("label %q missing from %q", label, line)
		}
		if i < at {
			t.Errorf("label %q out of order in %q", label, line)
		}
		at = i
	}
}

// A long reply is trimmed to the newest lines, since those are being read.
func TestRenderKeepsNewestReply(t *testing.T) {
	f := Frame{Reply: []string{"one", "two", "three"}}
	lines := Render(f, 5, 40)
	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "three") {
		t.Errorf("frame = %q, want the newest line", body)
	}
	if strings.Contains(body, "one") {
		t.Errorf("frame = %q, want the oldest line dropped", body)
	}
}

func TestRenderTruncatesLongInput(t *testing.T) {
	f := Frame{Input: strings.Repeat("x", 200)}
	lines := Render(f, 12, 20)
	for i, l := range lines {
		// Runes rather than bytes, since the rule is a multibyte character and
		// a byte count would report it as three times too wide.
		if n := utf8.RuneCountInString(l); n > 20 {
			t.Errorf("line %d is %d columns, want at most 20: %q", i, n, l)
		}
	}
}

// The rule must divide the screen at the full width, since a rule that stops
// short reads as a broken border rather than a division.
func TestRuleSpansWidth(t *testing.T) {
	if got := rule(20); utf8.RuneCountInString(got) != 20 {
		t.Errorf("rule(20) is %d columns, want 20", utf8.RuneCountInString(got))
	}
	if rule(0) != "" {
		t.Error("rule(0) is not empty")
	}
}

// The prompt must be separated from the conversation, so that a reply ending
// above it is not read as part of it.
func TestRenderSeparatesInputFromOutput(t *testing.T) {
	lines := Render(Frame{Reply: []string{"a reply"}, Input: "typing"}, 14, 30)

	ruleAt, promptAt := -1, -1
	for i, l := range lines {
		if l == strings.Repeat(ruleRune, 30) && ruleAt < 0 {
			ruleAt = i
		}
		if strings.HasPrefix(l, "> typing") {
			promptAt = i
		}
	}
	if ruleAt < 0 {
		t.Fatal("no rule in the frame")
	}
	if promptAt < 0 {
		t.Fatal("no prompt in the frame")
	}

	// A blank row above the rule, and one below it before the prompt.
	if lines[ruleAt-1] != "" {
		t.Errorf("the row above the rule is %q, want it blank", lines[ruleAt-1])
	}
	if lines[ruleAt+1] != "" {
		t.Errorf("the row below the rule is %q, want it blank", lines[ruleAt+1])
	}
	if lines[promptAt-1] != "" {
		t.Errorf("the row above the prompt is %q, want it blank", lines[promptAt-1])
	}
}

// The frame must never exceed the height it was given, or the status bar is
// pushed off the screen.
func TestRenderFitsHeight(t *testing.T) {
	for h := 6; h <= 30; h++ {
		lines := Render(Frame{Reply: []string{"a"}, Input: "b"}, h, 40)
		if len(lines) > h {
			t.Errorf("height %d produced %d rows", h, len(lines))
		}
	}
}

// A very small terminal must not panic or produce an empty frame.
func TestRenderTinyTerminal(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 5}, {3, 10}, {0, 0}} {
		lines := Render(Frame{Input: "hi"}, size[0], size[1])
		if len(lines) < 3 {
			t.Errorf("size %v: len(lines) = %d, want at least 3", size, len(lines))
		}
	}
}

func TestTruncateMarksTheCut(t *testing.T) {
	if got := truncate("abcdefghij", 5); got != "ab..." {
		t.Errorf("truncate = %q, want %q", got, "ab...")
	}
	if got := truncate("abc", 5); got != "abc" {
		t.Errorf("truncate = %q, want it unchanged", got)
	}
}

// The provider is a constant rather than a derived value, and must appear.
func TestStatusLineShowsProvider(t *testing.T) {
	line := StatusLine(Status{Provider: providerName}, 200)
	if !strings.Contains(line, providerName) {
		t.Errorf("line = %q, want the provider", line)
	}
}

// The state reads Working while a request is in flight, so a slow model is
// visible as working rather than as idle.
func TestStatusLineShowsWorking(t *testing.T) {
	line := StatusLine(Status{State: stateWorking}, 200)
	if !strings.Contains(line, stateWorking) {
		t.Errorf("line = %q, want the working state", line)
	}
}

// The removed fields must not reappear as permanent dashes.
func TestStatusLineHasNoDeadFields(t *testing.T) {
	line := StatusLine(Status{Provider: providerName}, 400)
	for _, gone := range []string{"Reasoning", "Branch", "Approval"} {
		if strings.Contains(line, gone) {
			t.Errorf("line = %q, want %q removed", line, gone)
		}
	}
}
