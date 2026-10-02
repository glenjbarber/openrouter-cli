package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/saved"
)

// errAutosaveRefused reports an autosave that would break a promise the mode
// makes.
var errAutosaveRefused = errors.New("autosave is refused while cognito is on, and in " +
	"a thread, since both record nothing and a file holding the conversation " +
	"would record that they had")

// Autosave writes the conversation to a file of its own without being asked.
//
// The interval is settled by what an autosave is for rather than by how often
// it is cheap. A session that is closed by a terminal dying is the case it
// exists for, and the conversation worth recovering is the whole of it rather
// than the last exchange, so an autosave after every turn is the figure that
// loses nothing. The timer is a backstop for a session sitting idle for hours,
// where nothing has been said and there is nothing new to lose.
//
// It is a timer inside the client rather than a cron entry or a shell loop,
// since a scheduled job writing a session it has not read would keep writing
// the same conversation for ever, and a reader would come back to a directory
// of files that all say the same thing.

// AutosaveInterval is how long an idle session waits before its conversation is
// written without being asked.
//
// It is long enough that a reader pausing to think does not fill the directory
// with copies of one conversation, and short enough that a session left open
// over a break has been written before the machine goes down.
const AutosaveInterval = 5 * time.Minute

// cmdAutosave reports or sets whether the conversation is written on its own.
func (s *Session) cmdAutosave(args []string) bool {
	if len(args) == 0 {
		s.appendLines(strings.Join(s.autosaveListing(), "\n"))
		return false
	}

	switch strings.ToLower(strings.Join(args, " ")) {
	case "on", "start", "every":
		s.setAutosave(true)
		s.appendLines("autosave is on: the conversation is written " +
			"after every turn, and on its own if it sits idle for " +
			AutosaveInterval.String() + ".")
	case "off", "stop":
		s.setAutosave(false)
		s.appendLines("autosave is off. /save still writes when you ask it to.")
	case "now", "once":
		path, err := s.writeAutosave()
		if err != nil {
			s.addReply("(" + err.Error() + ")")
			return false
		}
		s.Note("autosaved to %s", path)
	default:
		s.addReply("(/autosave takes on, off or now; not " +
			strings.ToLower(strings.Join(args, " ")) + ")")
	}
	return false
}

// setAutosave starts or stops the timer and says which it did.
func (s *Session) setAutosave(on bool) {
	s.mu.Lock()
	if on == (s.autosaveStop != nil) {
		// Already in the state asked for. The goroutine is not replaced, so a
		// reader turning it on and off does not leave one behind.
		s.mu.Unlock()
		return
	}
	if !on {
		s.autosaveStop <- struct{}{}
		s.autosaveStop = nil
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.autosaveStop = stop
	s.mu.Unlock()

	go s.autosaveTimer(stop)
}

// autosaveTimer writes the conversation every interval until it is stopped.
//
// It checks the clock rather than sleeping the whole interval, so that a
// session closed while the timer is waiting does not leave a goroutine holding
// a session nothing refers to. The close path stops the timer first, so this is
// belt and braces rather than the only guard.
func (s *Session) autosaveTimer(stop chan struct{}) {
	ticker := time.NewTicker(AutosaveInterval)
	defer ticker.Stop()

	// A session without a context is one assembled by a test rather than by
	// Start, and its end cannot be watched. The stop channel is enough for it.
	var done <-chan struct{}
	if s.ctx != nil {
		done = s.ctx.Done()
	}
	for {
		select {
		case <-stop:
			return
		case <-done:
			return
		case <-ticker.C:
			if _, err := s.writeAutosave(); err != nil {
				// A failure is not reported into the pane. The reader is not
				// watching, since nothing was said, and a line appearing with
				// no message after it is a line they cannot explain.
				continue
			}
		}
	}
}

// autosaveListing says what the autosave is doing.
func (s *Session) autosaveListing() []string {
	s.mu.Lock()
	running := s.autosaveStop != nil
	s.mu.Unlock()

	lines := []string{
		"autosave: " + onOff(running) + ", writing after every turn and on its " +
			"own every " + AutosaveInterval.String(),
	}
	if dir, err := saved.AutoDir(); err == nil {
		lines = append(lines, "  to "+dir)
	}
	lines = append(lines, "  named for you, the directory and the moment, with a "+
		"link to the newest beside them")
	return lines
}

// onOff renders a boolean as the two words a reader reads.
func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// writeAutosave writes the conversation and points the link at it.
//
// It returns the path written, since the reader is told where their work went
// when they asked for it by name.
func (s *Session) writeAutosave() (string, error) {
	if s.cognito || s.thread != nil {
		// The same promise /save refuses to break. A mode that says nothing is
		// recorded cannot then write the conversation to a disk.
		return "", errAutosaveRefused
	}

	conv := s.conv
	if conv == nil {
		return "", errAutosaveRefused
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	at := time.Now()
	name := saved.AutoName(dir, at)
	sess := s.snapshot(name)
	path, err := saved.Path(name)
	if err != nil {
		return "", err
	}
	if err := saved.Write(path, sess, true); err != nil {
		return "", err
	}
	if err := s.linkAutosave(dir, path); err != nil {
		// The save itself succeeded, so a link that could not be moved is not
		// a reason to report the save as failed.
		return path, nil
	}
	return path, nil
}

// linkAutosave points the per directory link at the file just written.
//
// The link is replaced rather than followed, since a symlink written over
// another one becomes a link to the link on some systems, which is a chain
// that grows for every save.
func (s *Session) linkAutosave(dir, path string) error {
	base, err := saved.AutoDir()
	if err != nil {
		return err
	}
	link := filepath.Join(base, saved.AutoLinkName(dir)+".db")

	// A stale link is removed first. os.Symlink refuses to write over one, and
	// leaving a link pointing at a file that is about to be replaced would
	// mean /load resolved through it to nothing.
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(path, link)
}

// autosaveAfterTurn writes the conversation if the reader asked for it to be.
//
// It is a no-op where the timer is not running, so a session with autosave off
// does no work on the path at all rather than checking a flag and returning.
func (s *Session) autosaveAfterTurn() {
	s.mu.Lock()
	running := s.autosaveStop != nil
	s.mu.Unlock()
	if !running {
		return
	}
	_, _ = s.writeAutosave()
}
