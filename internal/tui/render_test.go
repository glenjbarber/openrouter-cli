package tui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// lastContent returns the last row of the frame that is not blank.
func lastContent(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}

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
	// The prompt is the last row carrying content, since a blank row sits
	// below it so that the prompt is not flush against the foot of the screen.
	prompt := lastContent(lines)
	if !strings.HasPrefix(prompt, "> ") {
		t.Errorf("prompt row = %q, want the input prompt", prompt)
	}

	// The status bar is found by content rather than by position, since it sits
	// above the conversation rather than below it.
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

	// The rule above the prompt is separated from it by a blank row, so that
	// the rule reads as a division of the screen rather than a border of the
	// prompt.
	if lines[promptAt-1] != "" {
		t.Errorf("the row above the prompt is %q, want it blank", lines[promptAt-1])
	}
	// A reply must not sit directly against the prompt.
	if lines[promptAt-2] == "" || strings.HasPrefix(lines[promptAt-2], ">") {
		t.Errorf("the row before the blank is %q, want the conversation",
			lines[promptAt-2])
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

// A very small terminal must not panic, and must give back no more rows than
// it has.
//
// The frame is cut to the height rather than padded up to some minimum. A
// frame of rows the terminal does not have is written off the bottom, and a
// writer that then placed its caret against the last of those rows would put
// the caret somewhere the reader cannot see it.
func TestRenderTinyTerminal(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {2, 5}, {3, 10}, {0, 0}} {
		lines := Render(Frame{Input: "hi"}, size[0], size[1])
		if len(lines) > size[0] {
			t.Errorf("size %v: len(lines) = %d, want at most %d",
				size, len(lines), size[0])
		}
	}
}

// A row is cut in characters rather than in bytes. A reply carries multibyte
// text, and a cut at a byte boundary would leave half a character on the row
// and count it as wider than it is.
func TestTruncateCountsColumns(t *testing.T) {
	got := truncate("héllo wörld ünter", 8)
	if got != "héllo..." {
		t.Errorf("truncate = %q, want the cut taken in characters", got)
	}
	if strings.ContainsRune(got, 0xFFFD) {
		t.Errorf("truncate = %q, want no replacement character", got)
	}
}

func TestTruncateAsciiUnchanged(t *testing.T) {
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

// The context field shows the share of the window, which is what a reader
// watches to know when a compaction is coming.
func TestContextFieldRenders(t *testing.T) {
	line := StatusLine(Status{Context: "42%"}, 200)
	if !strings.Contains(line, "Context: 42%") {
		t.Errorf("line = %q, want the context share", line)
	}
}

// Context and Credits are different measures and must not be confused. Credits
// is the billing allowance; context is the window the conversation occupies.
func TestContextIsSeparateFromCredits(t *testing.T) {
	line := StatusLine(Status{Credits: "0.42/5", Context: "42%"}, 200)
	if !strings.Contains(line, "Credits: 0.42/5") {
		t.Errorf("line = %q, want the credits figure", line)
	}
	if !strings.Contains(line, "Context: 42%") {
		t.Errorf("line = %q, want the context share", line)
	}
}

// The context share is kept in preference to the allowance when the bar is
// narrow. The share is what says when a compaction is coming, and the allowance
// is the less urgent of the two.
func TestContextKeptOverCredits(t *testing.T) {
	s := Status{Credits: "0.42/5", Context: "42%", Model: "m"}
	for w := 20; w < 80; w++ {
		line := StatusLine(s, w)
		if strings.Contains(line, "Credits") && !strings.Contains(line, "Context") {
			t.Errorf("width %d: %q, want the context share kept", w, line)
		}
	}
}

// An unknown window must not report a share, since a figure against nothing is
// worse than no figure.
func TestContextOmittedWithoutAWindow(t *testing.T) {
	s := &Session{conv: NewConversation(), mainConv: NewConversation()}
	s.mainConv = s.conv
	s.updateStatus()

	if got := s.frame.Status.Context; got != "" && got != "0%" {
		t.Errorf("Context = %q, want it empty without a window", got)
	}
}
