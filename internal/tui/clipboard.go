package tui

import (
	"encoding/base64"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// Copying to the clipboard is done by asking the terminal rather than by
// running a program.
//
// The two ways are OSC 52, which writes the selection through the terminal, and
// an external command such as pbcopy or xclip. The command is a subprocess,
// which on this client means going through the approval path and being asked
// about every copy, and it does not work at all over a link where the
// clipboard belongs to the machine the reader is sitting at. OSC 52 is written
// by the client itself and travels the same path as everything else on screen.
//
// The cost is that a terminal may refuse it. Some refuse anything past a small
// selection, since a terminal cannot tell the difference between a copy the
// reader asked for and one a program asked for on its own. That is reported
// rather than hidden, since a copy that silently did nothing is worse than one
// that says it did not.

// clipboardLimit is the largest selection this client offers to copy.
//
// It is well above any reply a reader would want to carry elsewhere, and below
// what a terminal is likely to refuse. A reply of two gigabytes would be carried
// as a base64 sequence through the same pipe as the drawing, and every frame
// would wait for it.
const clipboardLimit = 1 << 20

// OSC 52 is the sequence that sets the selection. The 52 is the selector for
// the clipboard rather than for a named selection, and the c is the clipboard
// rather than the primary selection, which on a Unix-like system is the one a
// middle click pastes.
const (
	seqCopyPrefix = "\x1b]52;c;"
	seqCopySuffix = "\x07"
)

// cmdCopy writes the conversation to the clipboard.
//
// The conversation is copied whole rather than the visible pane. The pane is
// folded to the width of the terminal, so what a reader selected out of it is
// not the reply as the model wrote it, and a reader carrying a reply elsewhere
// wants the reply rather than a rendering of it.
func (s *Session) cmdCopy([]string) bool {
	text := s.copyText()
	if strings.TrimSpace(text) == "" {
		s.addReply("(there is nothing to copy yet)")
		return false
	}
	if len(text) > clipboardLimit {
		s.addReply("(the conversation is " + itoa(len(text)) + " bytes, over the " +
			"limit of " + itoa(clipboardLimit) + " for a copy)")
		return false
	}

	s.screen.copyToClipboard(text)
	s.appendLines("copied " + itoa(len(text)) + " bytes to the clipboard" +
		copyCaveat())
	// The frame is repainted after the copy rather than before it: the sequence
	// leaves the terminal wherever the copy ended, and a frame drawn from there
	// is a frame drawn in the wrong place. Repainting after is also what puts
	// the confirmation on screen at all, since the copy leaves nothing behind
	// it of its own.
	s.draw()
	return false
}

// copyCaveat says what a terminal may do with the request, once, rather than
// on every copy.
//
// The client cannot tell whether the terminal honoured it: OSC 52 is written
// and nothing comes back. Saying so once here is the only place a reader learns
// it, and a reader whose paste came out empty will look here first.
func copyCaveat() string {
	return ". If nothing pasted, this terminal does not take a copy from the " +
		"application; select the text and copy it with the terminal instead."
}

// copyText is the conversation as plain text.
//
// The turns are taken in the order they were had, and a turn a model made with
// tools carries the calls and their results, since those are part of what the
// reader asked for and what came back. Nothing is written that is not in the
// conversation, so what arrives elsewhere is what was said here.
func (s *Session) copyText() string {
	conv := s.conv
	if conv == nil {
		return ""
	}
	s.mu.Lock()
	messages := conv.Messages()
	s.mu.Unlock()

	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case openrouter.RoleUser:
			b.WriteString("> ")
			b.WriteString(m.Content)
			b.WriteByte('\n')
		case openrouter.RoleAssistant:
			// An assistant turn carrying tool calls has no content of its own,
			// and writing a marker for it keeps the exchange readable rather
			// than leaving a gap where a reply should be.
			if strings.TrimSpace(m.Content) != "" {
				b.WriteString(m.Content)
				b.WriteByte('\n')
			}
			for _, call := range m.ToolCalls {
				b.WriteString("[called ")
				b.WriteString(call.Function.Name)
				b.WriteString(" ")
				b.WriteString(singleLine(call.Function.Arguments))
				b.WriteString("]\n")
			}
		case openrouter.RoleTool:
			b.WriteString("[")
			b.WriteString(singleLine(m.Content))
			b.WriteString("]\n")
		}
	}
	return b.String()
}

// singleLine folds a value onto one line, so that a multi-line argument or a
// multi-line result does not read as separate exchanges.
func singleLine(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// copyToClipboard writes the text to the terminal's clipboard.
//
// The sequence is written and then the screen is repainted, since writing an
// OSC leaves the terminal wherever the sequence ended and the frame below it
// would be drawn from the wrong place. The repaint is the whole frame rather
// than a cursor move, since a terminal that honoured the copy may also have
// moved the cursor somewhere else entirely.
func (s *Screen) copyToClipboard(text string) {
	s.write(seqCopyPrefix)
	s.write(base64.StdEncoding.EncodeToString([]byte(text)))
	s.write(seqCopySuffix)
	s.write(seqHome)
}
