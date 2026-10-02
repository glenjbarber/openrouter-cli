package tui

import "strings"

// cmdColor turns colour on or off for the session.
//
// An argument of on or off sets the flag, and no argument toggles it. The
// change lasts for the session only, on the same terms as /bell and /mouse,
// since the configuration file is written only when it is absent or during
// setup. Nothing is drawn from the flag yet.
func (s *Session) cmdColor(args []string) bool {
	switch {
	case len(args) == 0:
		s.colorOn = !s.colorOn
	case len(args) == 1 && strings.EqualFold(args[0], "on"):
		s.colorOn = true
	case len(args) == 1 && strings.EqualFold(args[0], "off"):
		s.colorOn = false
	default:
		s.addReply("usage: /color [on|off]")
		return false
	}
	state := "off"
	if s.colorOn {
		state = "on"
	}
	s.appendLines("colour " + state)
	return false
}

// SetColor sets whether colour is drawn on the screen.
//
// The value comes from the configuration file, so a user who wants colour
// writes it once rather than typing a command at every session.
func (s *Session) SetColor(on bool) { s.colorOn = on }
