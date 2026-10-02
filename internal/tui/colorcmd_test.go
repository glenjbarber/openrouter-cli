package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// paintedFrame paints the session once and returns the bytes of that frame.
func paintedFrame(t *testing.T, s *Session, capture func() string) string {
	t.Helper()
	before := len(capture())
	s.paintNow()
	return capture()[before:]
}

// chromeColor is the sequence the rules of the frame are drawn in.
var chromeColor = roleSequences[roleChrome]

// /color on makes the next paint colored, and /color off makes it plain again.
func TestColorCommandColorsTheNextPaint(t *testing.T) {
	s, capture := auditSession(t, "")

	plain := paintedFrame(t, s, capture)
	if strings.Contains(plain, chromeColor) {
		t.Fatalf("a fresh session paints color:\n%q", plain)
	}

	s.command("/color on")
	on := paintedFrame(t, s, capture)
	if !strings.Contains(on, chromeColor) {
		t.Errorf("the paint after /color on carries no color:\n%q", on)
	}

	s.command("/color off")
	off := paintedFrame(t, s, capture)
	for _, seq := range csi.FindAllString(off, -1) {
		if seq != seqResetAttr {
			t.Errorf("the paint after /color off carries %q", seq)
		}
	}
}

// /colour is the same command.
func TestColorSpellingPaintsTheSame(t *testing.T) {
	a, capA := auditSession(t, "")
	b, capB := auditSession(t, "")

	a.command("/color on")
	b.command("/colour on")
	if got, want := paintedFrame(t, b, capB), paintedFrame(t, a, capA); got != want {
		t.Errorf("/colour on painted differently from /color on:\n%q\n%q", got, want)
	}
	a.command("/color off")
	b.command("/colour off")
	if got, want := paintedFrame(t, b, capB), paintedFrame(t, a, capA); got != want {
		t.Errorf("/colour off painted differently from /color off:\n%q\n%q", got, want)
	}
}

// A toggle with no argument turns color on and then off again.
func TestColorCommandToggles(t *testing.T) {
	s, capture := auditSession(t, "")
	s.command("/color")
	if !strings.Contains(paintedFrame(t, s, capture), chromeColor) {
		t.Error("the first toggle did not turn color on")
	}
	s.command("/color")
	if strings.Contains(paintedFrame(t, s, capture), chromeColor) {
		t.Error("the second toggle did not turn color off")
	}
}

// The command and the configuration key set the same two values, so the frames
// they produce are the same bytes, with and without a theme.
func TestCommandAndConfigKeyPaintTheSame(t *testing.T) {
	themes := []config.Theme{
		{},
		{Foreground: "white", Background: "#101010"},
		{Background: "0"},
	}
	for _, theme := range themes {
		viaKey, capKey := auditSession(t, "")
		viaKey.SetColor(true)
		viaKey.SetColorTheme(theme)

		viaCmd, capCmd := auditSession(t, "")
		viaCmd.SetColorTheme(theme)
		viaCmd.command("/color on")

		// The command prints its one line status into the pane, which the key
		// does not, so the key side runs the command too before the two are
		// compared, and both frames hold the same line.
		viaKey.command("/color on")
		want := paintedFrame(t, viaKey, capKey)
		got := paintedFrame(t, viaCmd, capCmd)
		if got != want {
			t.Errorf("theme %+v: the command and the key paint differently:\n%q\n%q", theme, got, want)
		}
	}
}

// The command prints one line saying what it did.
func TestColorCommandPrintsItsStatus(t *testing.T) {
	s, _ := auditSession(t, "")
	s.command("/color on")
	s.command("/color off")
	s.mu.Lock()
	reply := strings.Join(s.frame.Reply, "\n")
	s.mu.Unlock()
	for _, want := range []string{"color on", "color off"} {
		if !strings.Contains(reply, want) {
			t.Errorf("the pane does not say %q:\n%s", want, reply)
		}
	}
}

// A theme brings the base into every reset, and the row text is unchanged.
func TestAThemeIsAppliedOnPaint(t *testing.T) {
	s, capture := auditSession(t, "")
	s.SetColorTheme(config.Theme{Foreground: "white", Background: "#101010"})
	s.command("/color on")
	got := paintedFrame(t, s, capture)

	pal := newPalette(s.colorTheme)
	if pal.base() == "" {
		t.Fatal("the theme resolved to no base")
	}
	if !strings.Contains(got, sgrReset+pal.base()+seqClearLine) {
		t.Errorf("a row is not begun with the base:\n%q", got)
	}
	if strings.Contains(got, "\x1b[38;2") || strings.Contains(got, "\x1b[48;2") {
		t.Error("a 24 bit sequence was written")
	}
}

// A theme the loader could not use falls back to the terminal theme: no base is
// written, color still works, and the one line note is shown.
func TestAnInvalidThemeFallsBackAndShowsTheNote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	file := filepath.Join(home, ".openrouter-cli.json")
	body := `{"OPENROUTER_API_KEY":"k","color":true,` +
		`"color_theme":{"foreground":"chartreuse","background":"#12"}}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("loading the file: %v", err)
	}
	if cfg.ColorNote == "" {
		t.Fatal("the loader produced no note for an invalid theme")
	}

	// This is what the start of a session does with what the loader returned.
	s, capture := auditSession(t, "")
	s.SetColor(cfg.Color)
	s.SetColorTheme(cfg.ColorTheme)
	s.Note("%s", cfg.ColorNote)

	got := capture()
	if !strings.Contains(got, "color_theme") {
		t.Errorf("the note is not on the screen:\n%q", got)
	}
	frame := paintedFrame(t, s, capture)
	if !strings.Contains(frame, chromeColor) {
		t.Errorf("color did not stay on with a dropped theme:\n%q", frame)
	}
	if pal := s.framePalette(); pal == nil || pal.base() != "" {
		t.Errorf("an invalid theme left a base: %+v", pal)
	}
	if strings.Contains(frame, "\x1b[38;5;") || strings.Contains(frame, "\x1b[48;5;") {
		t.Errorf("a base sequence was written for an invalid theme:\n%q", frame)
	}
}

// The text of the pane never holds a sequence, whatever color is doing.
func TestColorNeverEntersTheStoredText(t *testing.T) {
	s, capture := auditSession(t, "")
	s.command("/color on")
	paintedFrame(t, s, capture)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range s.frame.Reply {
		if strings.Contains(line, "\x1b") {
			t.Errorf("a stored line holds an escape: %q", line)
		}
	}
}
