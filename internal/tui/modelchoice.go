package tui

import (
	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// setModel is the model a request is sent to, and it is remembered.
//
// The choice is written beside the configuration rather than into it, since
// the configuration holds the credential and is read-only outside setup, and
// the approval rules are kept beside it for the same reason and by the same
// route. A model chosen once should not have to be chosen again on the next
// run, which is the whole reason the choice is written down.
//
// The provider is written alongside it and left as it stands, since choosing a
// model is not choosing a provider. A reader who has not named one keeps the
// one the client was built against.
//
// A failure to write is reported rather than swallowed. The model has been set
// either way, so a reader whose choice was not kept would otherwise find it
// gone at the next run with nothing said about it.
func (s *Session) setModel(name string) {
	s.conv.SetModel(name)

	chosen := s.chosen
	chosen.Model = s.conv.Model()
	if err := config.WriteChosen(chosen); err != nil {
		s.appendLines("(" + err.Error() + ")")
	}
	s.updateStatus()
}

// adoptConfiguredModel puts the model in force for a session, in the order the
// two sources decide between them.
//
// A model chosen at the keyboard settles over one written into the
// configuration by hand, since a choice made later is a choice made about
// this. The configuration is applied first and only where the session has
// none, so that a session assembled without a configuration falls back to what
// the reader chose rather than to nothing.
//
// The remembered choice is applied whatever the configuration says, so that a
// reader who has chosen a model keeps it across a run in which the file was
// also edited. A file edited by hand and a choice made at the keyboard are both
// ways of saying which model to use, and the later one is the one a reader
// means.
func (s *Session) adoptConfiguredModel(fromFile string) {
	if fromFile != "" && s.conv.Model() == "" {
		s.conv.SetModel(fromFile)
	}
	if s.chosen.Model != "" {
		s.conv.SetModel(s.chosen.Model)
	}
}

// providerLabel is the name the status bar shows for the service.
//
// The remembered provider is a label and nothing more. Where requests are
// actually sent is settled by OPENROUTER_URL_BASE, which the loader has always
// read, so a name chosen for the screen does not repoint a single request. The
// two are kept apart deliberately: merging them would make a name written for
// the status bar silently send the reader's conversations somewhere else.
//
// A reader who has named no provider sees the one the client speaks to, which
// is the constant rather than an empty field, since a bar with a gap in it is
// a bar a reader has to interpret.
func (s *Session) providerLabel() string {
	if s.chosen.Provider != "" {
		return s.chosen.Provider
	}
	return providerName
}
