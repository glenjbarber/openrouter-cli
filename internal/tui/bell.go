package tui

import (
	"os"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// bellFile is the control character that rings the terminal bell.
//
// The bell is a byte in the output stream rather than a terminal feature, so it
// needs no support beyond the output itself.
const bellFile = "\a"

// ringBell writes the bell character to the given output.
//
// It is written only when the output is a terminal, since a redirected run has
// no terminal to ring and the byte would be noise in the capture.
func ringBell(out *os.File) {
	if out == nil || !IsTerminal(out) {
		return
	}
	out.WriteString(bellFile)
}

// ringBell rings the terminal bell if it is wanted.
//
// The bell is a preference rather than a mode. Nothing else in the client
// changes because of it, and a user who did not ask for one never hears one.
func (s *Session) ringBell() {
	if s.bellWanted {
		ringBell(s.out)
	}
}

// toggleBell turns the bell on or off, and records the new state in the
// configuration file, on the same terms as cmdColor: the session changes
// first and whatever happens to the file does not undo it, and a reader is
// told in one line when the file could not take the change.
//
// It is changed at runtime rather than only in the configuration file, since a
// user reaching for a bell mid-session wants it now and would not want to
// restart to get it.
func (s *Session) toggleBell() {
	s.bellWanted = !s.bellWanted
	on := s.bellWanted
	state := "off"
	if on {
		state = "on"
	}
	s.mu.Lock()
	save := s.bellSaver
	s.mu.Unlock()
	err := config.ErrNoConfigFile
	if save != nil {
		err = save(on)
	}
	if err != nil {
		state += " (not saved: " + strings.Join(strings.Fields(err.Error()), " ") + ")"
	}
	s.appendLines("bell " + state +
		": the terminal bell is rung when a reply arrives")
}

// SetBell sets whether the bell is rung when a reply arrives.
//
// The value comes from the configuration file, so a user who wants the bell
// writes it once rather than typing a command at every session.
func (s *Session) SetBell(on bool) { s.bellWanted = on }

// SetBellSaver installs the function /bell calls to record the new state. A
// session with none changes for the session only and says so, on the same
// terms as SetColorSaver.
func (s *Session) SetBellSaver(save func(on bool) error) {
	s.mu.Lock()
	s.bellSaver = save
	s.mu.Unlock()
}
