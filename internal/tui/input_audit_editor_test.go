package tui

import (
	"fmt"
	"strings"
	"testing"
)

// The history is bounded, and remembering a line at the bound keeps the
// newest rather than the oldest.
func TestAuditHistoryKeepsTheNewestAtTheBound(t *testing.T) {
	le := NewLineEditor(strings.NewReader(""))
	for i := 0; i < maxHistory+10; i++ {
		le.remember(fmt.Sprintf("line%d", i))
	}
	if len(le.history) != maxHistory {
		t.Errorf("history = %d entries, want %d", len(le.history), maxHistory)
	}
	if le.history[len(le.history)-1] != fmt.Sprintf("line%d", maxHistory+9) {
		t.Errorf("newest = %q, want the last line remembered", le.history[len(le.history)-1])
	}
}

// A callback that may be nil must not be dereferenced on any path: a
// non-interactive reader passes none. Every path through the editor is walked
// with every callback unset.
func TestAuditNilCallbacksOnEveryPath(t *testing.T) {
	le := NewLineEditor(strings.NewReader(
		pasteStart + "pasted" + pasteEnd + // a paste
			"\x1b[<64;1;1M" + // a report
			"\x1b[A\x1b[B" + // both arrows
			"abc\x7f\x7f\x15\x17" + // backspace, clear, word
			"\t" + // a tab
			"d\xC3\xA9\x7f" + // a multibyte rune and backspace
			"\r"))
	le.OnPaste = nil
	le.OnChange = nil
	le.OnMouse = nil
	le.OnKey = nil
	le.OnTab = nil

	if _, err := le.ReadLine(); err != nil {
		t.Errorf("ReadLine: %v", err)
	}
}

// The history recall must not require a report callback either.
func TestAuditNilCallbacksWithHistoryRecall(t *testing.T) {
	le := NewLineEditor(strings.NewReader("a line\r\x1b[A\r"))
	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if _, err := le.ReadLine(); err != nil {
		t.Fatalf("ReadLine on the recalled line: %v", err)
	}
}

// A rune arriving one byte at a time is reported only once it is whole, so a
// character is never drawn half formed.
func TestAuditMultibyteSplitAcrossReadsIsReportedWhole(t *testing.T) {
	le := NewLineEditor(&splitReader{chunks: []string{"a\xC3", "\xA9b\r"}})

	var seen []string
	le.OnChange = func(s string) { seen = append(seen, s) }

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "a\U000000E9b" {
		t.Errorf("got %q, want the rune whole", got)
	}
	for _, s := range seen {
		if strings.ContainsRune(s, 0xFFFD) {
			t.Errorf("reported %q, want no replacement character", s)
		}
	}
}
