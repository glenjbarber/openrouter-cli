package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

const savedSecret = "sk-or-SECRET-KEY-VALUE"

// colorSavedSession is a session whose saver writes the file at path.
func colorSavedSession(t *testing.T, path string) (*Session, func() string) {
	t.Helper()
	s, capture := auditSession(t, "")
	s.SetColorSaver(func(on bool) error { return config.WriteColor(path, on) })
	return s, capture
}

func savedFile(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openrouter-cli.json")
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileBody(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// /color and its alias both persist the new state, whatever they were typed as.
func TestColorCommandPersistsTheNewState(t *testing.T) {
	for _, typed := range []string{"/color on", "/colour on"} {
		path := savedFile(t, `{"OPENROUTER_API_KEY": "`+savedSecret+`"}`, 0o600)
		s, capture := colorSavedSession(t, path)
		s.command(typed)
		if got := fileBody(t, path); !strings.Contains(got, `"color": true`) {
			t.Errorf("%s did not persist:\n%s", typed, got)
		}
		frame := paintedFrame(t, s, capture)
		if !strings.Contains(frame, "color on") || strings.Contains(frame, "not saved") {
			t.Errorf("%s: unexpected status:\n%q", typed, frame)
		}
		s.command("/colour off")
		if got := fileBody(t, path); !strings.Contains(got, `"color": false`) {
			t.Errorf("/colour off did not persist:\n%s", got)
		}
		s.command("/color")
		if got := fileBody(t, path); !strings.Contains(got, `"color": true`) {
			t.Errorf("the toggle did not persist the new state:\n%s", got)
		}
	}
}

// The state after /colour on is the state after /color on, and the file holds
// the same bytes.
func TestColourAliasPersistsTheSameAsColor(t *testing.T) {
	body := `{"OPENROUTER_API_KEY": "` + savedSecret + `"}`
	pa, pb := savedFile(t, body, 0o600), savedFile(t, body, 0o600)
	a, _ := colorSavedSession(t, pa)
	b, _ := colorSavedSession(t, pb)
	a.command("/color on")
	b.command("/colour on")
	if fileBody(t, pa) != fileBody(t, pb) || a.colorOn != b.colorOn {
		t.Error("/colour on and /color on differ")
	}
}

// A failed write leaves the session changed and shows a one line reason with
// nothing from the file in it.
func TestColorWriteFailureIsNoticedWithoutSecrets(t *testing.T) {
	body := `{"OPENROUTER_API_KEY": "` + savedSecret + `"}`
	path := savedFile(t, body, 0o644)
	s, capture := colorSavedSession(t, path)
	s.command("/color on")
	if !s.colorOn {
		t.Error("the session did not change")
	}
	frame := paintedFrame(t, s, capture)
	if !strings.Contains(frame, "not saved") {
		t.Errorf("no notice:\n%q", frame)
	}
	if strings.Contains(frame, savedSecret) {
		t.Errorf("the notice carries file content:\n%q", frame)
	}
	if fileBody(t, path) != body {
		t.Error("the file changed")
	}
}

// A session with no way to save is session-only and says so.
func TestColorWithNoConfigFileSaysNotSaved(t *testing.T) {
	s, capture := auditSession(t, "")
	s.command("/color on")
	if !s.colorOn {
		t.Error("the session did not change")
	}
	if frame := paintedFrame(t, s, capture); !strings.Contains(frame, "color on (not saved: no configuration file)") {
		t.Errorf("unexpected notice:\n%q", frame)
	}
}

// The color_theme key is left as it is by the command.
func TestColorCommandLeavesTheTheme(t *testing.T) {
	theme := `{ "foreground":"red" }`
	path := savedFile(t, `{"color_theme": `+theme+`}`, 0o600)
	s, _ := colorSavedSession(t, path)
	s.command("/color on")
	if got := fileBody(t, path); !strings.Contains(got, theme) {
		t.Errorf("the theme changed:\n%s", got)
	}
}
