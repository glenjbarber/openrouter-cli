package tui

import (
	"strings"
	"testing"
)

// A shifted enter is the CSI u report a terminal sends for the key where it
// reports it at all. Most terminals send nothing, which is why the byte Ctrl-J
// carries is the one that opens the multi-line mode on every terminal.
func TestTakeShiftedEnter(t *testing.T) {
	tests := []struct {
		name string
		buf  string
		ok   bool
		rest string
	}{
		{name: "the form the reader acts on", buf: "\x1b[13;2u", ok: true, rest: ""},
		{name: "with text after it", buf: "\x1b[13;2uxyz", ok: true, rest: "xyz"},
		{name: "shift with another key", buf: "\x1b[13;1u", ok: false},
		{name: "shift with alt", buf: "\x1b[13;3u", ok: false},
		{name: "the key without shift", buf: "\x1b[13u", ok: false},
		{name: "shift with a newline", buf: "\x1b[10;2u", ok: false},
		{name: "an up arrow", buf: "\x1b[A", ok: false},
		{name: "a mouse report", buf: "\x1b[<64;1;1M", ok: false},
		{name: "the x10 mouse form", buf: "\x1b[M\x60\x20\x20", ok: false},
		{name: "a lone escape", buf: "\x1b", ok: false},
		{name: "not a sequence at all", buf: "text", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rest, ok := takeShiftedEnter([]byte(tt.buf))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v for %q", ok, tt.ok, tt.buf)
			}
			if ok && string(rest) != tt.rest {
				t.Errorf("rest = %q, want %q", rest, tt.rest)
			}
			if !ok && string(rest) != tt.buf {
				t.Errorf("rest = %q, want the buffer left whole", rest)
			}
		})
	}
}

// A report arriving in pieces is held rather than taken, since the escape at
// its head would otherwise be read as a key and end the session.
func TestTakeShiftedEnterHoldsAPartialReport(t *testing.T) {
	for _, buf := range []string{"\x1b", "\x1b[", "\x1b[1", "\x1b[13", "\x1b[13;", "\x1b[13;2"} {
		rest, ok := takeShiftedEnter([]byte(buf))
		if ok {
			t.Errorf("%q was taken as a shifted enter", buf)
		}
		if string(rest) != buf {
			t.Errorf("%q: rest = %q, want the buffer left for the next read", buf, rest)
		}
	}
}

// Shift with enter ends a line inside the multi-line mode and outside it
// alike, since it means a break either way. It is not the key that sends, so a
// reader holding it keeps typing without sending the block by accident.
//
// The block goes when Ctrl-J is pressed, which is the same key that opened the
// mode, so the report lands where the send is rather than where the break is.
func TestShiftEnterBreaksTheLineWhereItWasPressed(t *testing.T) {
	// one, Ctrl-J opens the mode, two, shift+enter ends the line, three,
	// Ctrl-J sends the block.
	le := NewLineEditor(strings.NewReader("one\ntwo\x1b[13;2uthree\n"))

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "one\ntwo\nthree" {
		t.Errorf("got %q, want three lines joined into one message", got)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("got %q, want no sequence bytes left in the message", got)
	}
}
