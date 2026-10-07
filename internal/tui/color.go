package tui

import (
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// cmdColor turns color on or off, and records the new state in the
// configuration file.
//
// An argument of on or off sets the flag, and no argument toggles it. The
// session changes first and whatever happens to the file does not undo it: a
// reader who asked for color gets color, and is told in one line when the file
// could not take the change. The next paint reads the flag, and the input loop
// repaints after every command, so the screen changes as soon as the command
// has run.
//
// The flag is written under the session lock, since the paint path reads it
// from the spinner goroutine as well as from the input one. The file is written
// after the lock is released, since it is slow and the paint path waits on the
// lock.
func (s *Session) cmdColor(args []string) bool {
	s.mu.Lock()
	switch {
	case len(args) == 0:
		s.colorOn = !s.colorOn
	case len(args) == 1 && strings.EqualFold(args[0], "on"):
		s.colorOn = true
	case len(args) == 1 && strings.EqualFold(args[0], "off"):
		s.colorOn = false
	default:
		s.mu.Unlock()
		s.addReply("usage: /color [on|off]")
		return false
	}
	on := s.colorOn
	save := s.colorSaver
	s.mu.Unlock()
	state := "off"
	if on {
		state = "on"
	}
	err := config.ErrNoConfigFile
	if save != nil {
		err = save(on)
	}
	if err != nil {
		// The reason is one line and is made by the writer without any file
		// content in it.
		state += " (not saved: " + strings.Join(strings.Fields(err.Error()), " ") + ")"
	}
	s.appendLines("color " + state)
	return false
}

// SetColorSaver installs the function /color calls to record the new state. A
// session with none changes for the session only and says so.
func (s *Session) SetColorSaver(save func(on bool) error) {
	s.mu.Lock()
	s.colorSaver = save
	s.mu.Unlock()
}

// framePalette resolves the palette a frame is drawn in, or none when color is
// off.
//
// It is resolved at paint time from the flag and the theme, so the command and
// the configuration key, which both only set those two values, cannot produce
// different results. A theme that holds nothing resolves to a palette with no
// base, which is the terminal theme showing through.
func (s *Session) framePalette() *palette {
	s.mu.Lock()
	on, theme, ground := s.colorOn, s.colorTheme, s.ground
	s.mu.Unlock()
	if !on {
		return nil
	}
	p := newPaletteOn(theme, ground)
	return &p
}

// SetColor sets whether color is drawn on the screen.
//
// The value comes from the configuration file, so a user who wants color
// writes it once rather than typing a command at every session.
func (s *Session) SetColor(on bool) {
	s.mu.Lock()
	s.colorOn = on
	s.mu.Unlock()
}

// SetColorTheme sets the base colors the file asks for.
//
// An empty side follows the terminal theme. The values were checked when the
// file was read, so a value that is set is one the palette can resolve.
func (s *Session) SetColorTheme(theme config.Theme) {
	s.mu.Lock()
	s.colorTheme = theme
	s.mu.Unlock()
}
