package tui

import (
	"encoding/base64"
	"slices"
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

// cmdCopy writes the last reply to the clipboard.
//
// The last reply rather than the whole conversation. A reader reaching for a
// copy is usually carrying one answer somewhere: into a report, into a commit
// message, into a shell. The whole conversation is what they would select with
// the mouse when they meant all of it, and a copy that brings the questions
// along with the answer is a paste that has to be edited before it is used.
//
// The pane is not what is copied either. It is folded to the width of the
// terminal, so what a reader selected out of it is not the reply as the model
// wrote it.
func (s *Session) cmdCopy([]string) bool {
	text := s.lastReply()
	if strings.TrimSpace(text) == "" {
		s.addReply("(there is no reply to copy yet)")
		return false
	}
	if len(text) > clipboardLimit {
		s.addReply("(the reply is " + itoa(len(text)) + " bytes, over the " +
			"limit of " + itoa(clipboardLimit) + " for a copy)")
		return false
	}

	s.screen.copyToClipboard(text)
	s.appendLines("copied the last reply, " + itoa(len(text)) +
		" bytes, to the clipboard" + copyCaveat())
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

// lastReply is the most recent thing the model said, as plain text.
//
// A reply is a whole exchange rather than one message. A turn that called a
// tool is several: the call, the result, and then what the model said about
// it. Those are one answer, and a reader carrying it elsewhere wants the build
// and the error with the answer rather than the answer with the evidence
// missing. The walk therefore goes back to the question rather than to the
// last assistant turn alone.
//
// The question itself is left out, since it is what the reader asked and has.
func (s *Session) lastReply() string {
	conv := s.conv
	if conv == nil {
		return ""
	}
	s.mu.Lock()
	messages := conv.Messages()
	s.mu.Unlock()

	// The exchange is collected backwards and then reversed, since the
	// conversation is in the order it was had and a reply reads forwards.
	var b strings.Builder
	for i := len(messages) - 1; i >= 0; i-- {
		switch messages[i].Role {
		case openrouter.RoleUser:
			// The question marks where the reply began.
			if b.Len() > 0 {
				return reverseLines(b.String())
			}
			return ""
		case openrouter.RoleTool:
			b.WriteString("[")
			b.WriteString(singleLine(messages[i].Content))
			b.WriteString("]\n")
		case openrouter.RoleAssistant:
			for _, call := range messages[i].ToolCalls {
				b.WriteString("[called ")
				b.WriteString(call.Function.Name)
				b.WriteString(" ")
				b.WriteString(singleLine(call.Function.Arguments))
				b.WriteString("]\n")
			}
			if text := strings.TrimRight(messages[i].Content, "\n"); text != "" {
				b.WriteString(text)
				b.WriteByte('\n')
			}
		}
	}
	if b.Len() > 0 {
		return reverseLines(b.String())
	}
	return ""
}

// reverseLines puts a reply built backwards into the order it was said in.
func reverseLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	slices.Reverse(lines)
	return strings.Join(lines, "\n") + "\n"
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
