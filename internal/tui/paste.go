package tui

import (
	"strings"
)

// Bracketed paste markers.
//
// A terminal in bracketed paste mode wraps pasted text in an opening and a
// closing sequence. Without the markers a paste is indistinguishable from
// someone typing very fast, so a newline inside one would submit the line
// halfway through and send half a paste as a message. The markers are what
// make a multi-line paste a single input rather than several messages.
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// stripPaste removes the bracketed paste markers from text.
//
// Text that carries no markers is returned unchanged, so a paste arriving from
// a terminal without the mode enabled still works rather than being mangled.
func stripPaste(s string) string {
	s = strings.ReplaceAll(s, pasteStart, "")
	return strings.ReplaceAll(s, pasteEnd, "")
}

// hasPasteStart reports whether a block opens a paste.
func hasPasteStart(s string) bool { return strings.Contains(s, pasteStart) }

// takePaste extracts a complete pasted block from the buffer.
//
// A block that is not yet complete is left in place, since the closing marker
// may arrive in the next read. The remainder is returned alongside so that the
// bytes after the closing marker are not lost.
func takePaste(buf []byte) (text string, rest []byte, ok bool) {
	start := strings.Index(string(buf), pasteStart)
	if start < 0 {
		return "", nil, false
	}
	end := strings.Index(string(buf), pasteEnd)
	if end < 0 {
		return "", nil, false
	}
	return stripPaste(string(buf[start : start+end])), buf[start+end+len(pasteEnd):], true
}

// normalisePaste turns a pasted block into input lines.
//
// A carriage return and a newline together are one break, since a terminal
// sends the pair for a single newline. A trailing break is dropped, since it is
// the end of the paste rather than a request for an empty line, and leaving it
// would submit the message the moment the paste landed.
func normalisePaste(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
