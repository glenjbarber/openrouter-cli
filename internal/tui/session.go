package tui

import (
	"fmt"
	"os"
	"strings"
)

// Session is a running interface.
type Session struct {
	screen *Screen
	editor *LineEditor
	frame  Frame
}

// Start opens the interface on the given files.
//
// A caller that redirects the output receives ErrNotTerminal and is expected to
// fall back to ordinary line-oriented output rather than having a frame
// written into the capture.
func Start(out, in *os.File, title string) (*Session, error) {
	screen, err := NewScreen(out, in)
	if err != nil {
		return nil, err
	}
	return &Session{
		screen: screen,
		editor: NewLineEditor(in),
		frame:  Frame{Title: title},
	}, nil
}

// Close restores the terminal.
func (s *Session) Close() { s.screen.Close() }

// ErrQuit reports that the user asked to leave the session.
var ErrQuit = errorString("quit")

// Run reads lines until the user leaves.
//
// A line beginning with a slash is treated as a command rather than as a
// prompt, so that the interface keeps its own vocabulary separate from the
// model input.
func (s *Session) Run() error {
	s.draw()
	for {
		line, err := s.editor.ReadLine()
		if err == ErrEndOfInput {
			return nil
		}
		if err == ErrInterrupt {
			if s.frame.Input == "" {
				return ErrQuit
			}
			// An interrupt with text in hand abandons the line rather than
			// the session, which is what a shell does.
			s.frame.Input = ""
			s.draw()
			continue
		}
		if err != nil {
			return err
		}

		switch {
		case strings.TrimSpace(line) == "":
			continue
		case strings.HasPrefix(line, "/quit"), strings.HasPrefix(line, "/exit"):
			return ErrQuit
		case strings.HasPrefix(line, "/"):
			s.frame.Reply = append(s.frame.Reply, s.command(line))
		default:
			// The model is not yet connected, so the echo is marked rather
			// than presented as an answer.
			s.frame.Reply = append(s.frame.Reply,
				"> "+line, "(no model connection yet)")
		}
		s.draw()
	}
}

// command handles a slash command.
func (s *Session) command(line string) string {
	switch line {
	case "/help":
		return "/help, /quit, /exit, /clear"
	case "/clear":
		s.frame.Reply = nil
		return ""
	default:
		return "unknown command: " + line
	}
}

// draw repaints the frame.
func (s *Session) draw() {
	height, width := s.screen.Size()
	s.screen.Draw(Render(s.frame, height, width))
}

// Note adds a line to the reply pane, for a message the client generates such
// as a bootstrap confirmation. The line is shown inside the frame rather than
// written before it, so that it is not cleared by the first repaint.
func (s *Session) Note(format string, args ...any) {
	s.frame.Reply = append(s.frame.Reply, fmt.Sprintf(format, args...))
	s.draw()
}
