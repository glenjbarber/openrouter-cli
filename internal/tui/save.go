package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// cmdSave writes the conversation to a file of its own.
//
// A save is refused while in-cognito mode is on. The mode promises that
// nothing is recorded, on disk as well as in memory, and a file holding the
// conversation would break that promise rather than record that it was broken.
// A thread is refused the same way, since a thread records nothing and so has
// nothing to write.
func (s *Session) cmdSave(args []string) bool {
	if s.cognito {
		s.addReply("(/save is refused while cognito is on, since it records nothing; " +
			"turn it off with /cognito first)")
		return false
	}
	if s.thread != nil {
		s.addReply("(/save is refused in a thread, since a thread records nothing; " +
			"leave it with /main first)")
		return false
	}
	name := defaultSaveName()
	if len(args) > 0 {
		name = strings.Join(args, " ")
	}
	path, err := saved.Path(name)
	if err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}

	// The overwrite is settled before anything is written rather than after a
	// failure, so that a reader who declined is not shown an error where the
	// save actually succeeded under the name they were offered.
	overwrite := false
	if _, err := os.Stat(path); err == nil {
		if s.confirmOverwrite(path) {
			overwrite = true
		} else if path, err = saved.Path(saved.Suffixed(name, time.Now().Unix())); err != nil {
			s.addReply("(" + err.Error() + ")")
			return false
		}
	}

	sess := s.snapshot(name)
	if err := saved.Write(path, sess, overwrite); err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}
	s.Note("saved %s, %d turns, to %s", sess.Name, sess.TurnCount(), path)
	return false
}

// cmdLoad puts a saved conversation back in front of the model.
//
// The turns are shown as well as restored. A load that replaced the
// conversation silently would leave the reader unable to see what they had
// resumed, and unable to tell a loaded conversation from one that had merely
// been going on behind them.
func (s *Session) cmdLoad(args []string) bool {
	if len(args) == 0 {
		s.addReply("(/load needs the name a session was saved under)")
		return false
	}
	path, err := saved.Path(strings.Join(args, " "))
	if err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}
	sess, err := saved.Read(path)
	if err != nil {
		s.addReply("(" + err.Error() + ")")
		return false
	}

	s.conv.Load(sess)
	s.clearReply()
	s.resetScroll()
	s.addReply(conversationLines(sess.Messages)...)
	s.Note("loaded %s, %d turns, saved %s", sess.Name, sess.TurnCount(),
		sess.SavedAt.Local().Format("2006-01-02 15:04"))
	return false
}

// snapshot copies the conversation out from under the lock, since the counters
// and the turns are written by the request goroutine and a save that read them
// while a turn was recording one would read a half-written exchange.
func (s *Session) snapshot(name string) saved.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return saved.Session{
		Name:      strings.TrimSuffix(name, ".db"),
		Model:     s.conv.Model(),
		TokensIn:  s.conv.TokensIn(),
		TokensOut: s.conv.TokensOut(),
		Usage:     s.conv.Usage(),
		Messages:  s.conv.Messages(),
	}
}

// confirmOverwrite asks whether a file that is already there should be
// replaced.
//
// The answer has to be yes to replace it and anything else declines, since the
// reader was asked whether a conversation they may not have seen again should
// be destroyed, and a stray keypress is not consent to that.
func (s *Session) confirmOverwrite(path string) bool {
	s.addReply(path + " is already saved. overwrite it? y or n, Escape leaves it")
	s.draw()

	line, err := s.editor.ReadLine()
	// The editor reports the line to the frame after every keystroke, so what
	// was typed for the answer is still held there once the line is read.
	s.mu.Lock()
	s.frame.Input = ""
	s.frame.Pasted = nil
	s.mu.Unlock()

	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// defaultSaveName is the name a save with no name given is filed under.
//
// It carries the second rather than the minute, since a session saved twice in
// one minute is ordinary and a save that quietly overwrote the last one would
// not be.
func defaultSaveName() string {
	return "session-" + time.Now().Format("20060102-150405")
}

// conversationLines renders saved turns for the pane, one entry per turn.
//
// Each turn is a single entry rather than one per line, so that a reply
// carrying fenced code keeps its own line breaks and the renderer folds it as
// a block. A turn split here would leave each fence marker on its own and the
// renderer would fold code that must not be folded.
func conversationLines(msgs []openrouter.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, fmt.Sprintf("%s: %s", m.Role, m.Content))
	}
	return out
}

// loadSaved is the startup path for a bootstrap document that is a saved
// session rather than prose.
//
// The model is adopted only where the session has not chosen one, so that an
// explicit choice still wins over the file, which is the same rule the
// configuration file's model is held to.
func (s *Session) loadSaved(sess *saved.Session) {
	if sess == nil {
		return
	}
	s.conv.Load(sess)
	s.Note("bootstrap: %s (%d turns, saved %s)", sess.Name, sess.TurnCount(),
		sess.SavedAt.Local().Format("2006-01-02 15:04"))
}
