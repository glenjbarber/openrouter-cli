package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// Colour must not be on unless asked for.
func TestColorOffByDefault(t *testing.T) {
	s := &Session{}
	if s.colorOn {
		t.Error("colorOn = true on a fresh session")
	}
}

func TestCmdColorSetsAndToggles(t *testing.T) {
	cases := []struct {
		name  string
		start bool
		args  []string
		want  bool
	}{
		{"no argument turns on", false, nil, true},
		{"no argument turns off", true, nil, false},
		{"on from off", false, []string{"on"}, true},
		{"on stays on", true, []string{"on"}, true},
		{"off from on", true, []string{"off"}, false},
		{"off stays off", false, []string{"off"}, false},
		{"case is ignored", false, []string{"ON"}, true},
		{"unknown word leaves it", true, []string{"maybe"}, true},
		{"extra words leave it", false, []string{"on", "now"}, false},
	}
	for _, c := range cases {
		s := &Session{colorOn: c.start}
		if s.cmdColor(c.args) {
			t.Errorf("%s: cmdColor asked to end the session", c.name)
		}
		if s.colorOn != c.want {
			t.Errorf("%s: colorOn = %v, want %v", c.name, s.colorOn, c.want)
		}
	}
}

func TestSetColor(t *testing.T) {
	s := &Session{}
	s.SetColor(true)
	if !s.colorOn {
		t.Error("SetColor(true) did not turn colour on")
	}
	s.SetColor(false)
	if s.colorOn {
		t.Error("SetColor(false) did not turn colour off")
	}
}

// The hidden spelling resolves to the very same entry as the listed one.
func TestColourIsAHiddenAliasOfColor(t *testing.T) {
	a, b := lookupCommand("/color"), lookupCommand("/colour")
	if a == nil || b == nil {
		t.Fatalf("lookupCommand: /color = %v, /colour = %v", a, b)
	}
	if a != b {
		t.Error("/colour does not resolve to the /color entry")
	}
}

// The hidden spelling is accepted when typed and is not shown anywhere.
func TestColourIsNeverListedOrOffered(t *testing.T) {
	if strings.Contains(helpText(), "/colour") {
		t.Error("the help lists /colour")
	}
	if !strings.Contains(helpText(), "/color") {
		t.Error("the help does not list /color")
	}
	for _, c := range candidates() {
		if c.Name == "/colour" {
			t.Error("the completer offers /colour")
		}
	}
	for _, c := range commands {
		if strings.Contains(c.usageLine(), "/colour") {
			t.Errorf("usageLine of %v names /colour", c.names)
		}
	}
}

// Typing the hidden spelling runs the command.
func TestColourRunsTheCommand(t *testing.T) {
	s := &Session{}
	s.command("/colour on")
	if !s.colorOn {
		t.Error("/colour on did not turn colour on")
	}
	s.command("/color off")
	if s.colorOn {
		t.Error("/color off did not turn colour off")
	}
}

func TestSetColorTheme(t *testing.T) {
	s := &Session{}
	if s.colorTheme != (config.Theme{}) {
		t.Errorf("a fresh session has a theme: %+v", s.colorTheme)
	}
	want := config.Theme{Foreground: "red", Background: "#101010"}
	s.SetColorTheme(want)
	if s.colorTheme != want {
		t.Errorf("colorTheme = %+v, want %+v", s.colorTheme, want)
	}
}
