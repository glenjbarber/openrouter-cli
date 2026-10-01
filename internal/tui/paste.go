package tui

import (
	"bytes"
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

// The markers as bytes, so that a search for one does not copy the buffer
// it is searching. A pasted block is searched for on every read while it
// arrives, and a large one is searched for many times over, so the copy
// would dominate the cost of assembling it.
var (
	pasteStartBytes = []byte(pasteStart)
	pasteEndBytes   = []byte(pasteEnd)
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
//
// Both markers are located by index into the whole buffer, and the two are
// offset from each other rather than one from the other. Measuring the end
// from the start of the block would take the text up to a marker found before
// the block began, and would leave the remainder pointing past the end of the
// buffer.
func takePaste(buf []byte) (text string, rest []byte, ok bool) {
	start := bytes.Index(buf, pasteStartBytes)
	if start < 0 {
		return "", nil, false
	}
	// The body is what follows the opening marker, so the closing marker is
	// sought within it. A stray closing marker ahead of the opening one
	// belongs to a block that has already been taken.
	body := buf[start+len(pasteStart):]
	end := bytes.Index(body, pasteEndBytes)
	if end < 0 {
		return "", nil, false
	}
	return stripPaste(string(body[:end])), body[end+len(pasteEnd):], true
}

// pastePrefix reports whether the bytes so far could still grow into a
// complete pasted block, and so must be held rather than read as keys.
//
// The opening marker is itself the shape of a four-byte key sequence, and a
// block larger than one read has not arrived whole when it is first looked at.
// Reading the marker as a sequence and the body as typing would submit the
// line at the first newline inside the paste, which is the failure the markers
// exist to prevent. The marker is therefore held, in whole or in part, until
// the closing marker arrives or the hold times out.
func pastePrefix(buf []byte) bool {
	if len(buf) == 0 {
		return false
	}
	if len(buf) < len(pasteStart) {
		return string(buf) == pasteStart[:len(buf)]
	}
	if !bytes.HasPrefix(buf, pasteStartBytes) {
		return false
	}
	// A complete opening marker is still a prefix until the closing one
	// arrives, since takePaste is tried before this and has not taken it.
	return bytes.Index(buf, pasteEndBytes) < 0
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
